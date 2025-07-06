package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/vault"
	"github.com/testcontainers/testcontainers-go/wait"
)

type VaultClustersHelper struct {
	MainVaultCluster *VaultHelper
	ReplicasClusters []*VaultHelper
}

type VaultHelper struct {
	container *vault.VaultContainer
	Config    *VaultClusterConfig
}

type VaultClusterConfig struct {
	ClusterName string
	Address     string
	Port        string
	Token       string
}

func NewVaultClusters(t require.TestingT, ctx context.Context, numberOfReplicas int) (*VaultClustersHelper, error) {
	mainClusterC, err := NewVaultClusterContainer(t, ctx, "main-cluster")
	if err != nil {
		return nil, fmt.Errorf("failed to create main vault cluster: %w", err)
	}

	replicaClusters := make([]*VaultHelper, numberOfReplicas)
	for i := range replicaClusters {
		replicaName := fmt.Sprintf("replica-%d", i)
		replicaCluster, err := NewVaultClusterContainer(t, ctx, replicaName)
		if err != nil {
			return nil, fmt.Errorf("failed to create replica vault cluster %q: %w", replicaName, err)
		}
		replicaClusters[i] = replicaCluster
	}

	return &VaultClustersHelper{
		MainVaultCluster: mainClusterC,
		ReplicasClusters: replicaClusters,
	}, nil
}

func NewVaultClusterContainer(t require.TestingT, ctx context.Context, clusterName string) (*VaultHelper, error) {
	pm := getPortManager()
	randomPort, err := pm.reservePort()
	if err != nil {
		return nil, fmt.Errorf("failed to reserve port: %w", err)
	}

	return newVaultContainerWithFixedPort(ctx, clusterName, fmt.Sprintf("%d", randomPort))
}

func newVaultContainerWithFixedPort(ctx context.Context, clusterName string, hostPort string) (*VaultHelper, error) {
	root_token := "root-token"
	vaultContainer, err := vault.Run(ctx,
		"hashicorp/vault:1.13.0",
		vault.WithToken(root_token),
		vault.WithInitCommand("secrets enable transit", "write -f transit/keys/my-key"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithPort("8200/tcp").
				WithStartupTimeout(30*time.Second),
			wait.ForExposedPort().WithStartupTimeout(1*time.Minute)),
		testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
			hostConfig.PortBindings = nat.PortMap{nat.Port("8200/tcp"): []nat.PortBinding{{HostPort: hostPort}}}
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to start Vault container: %w", err)
	}

	host, err := vaultContainer.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	port, err := vaultContainer.MappedPort(ctx, "8200")
	if err != nil {
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	vaultConfig := &VaultClusterConfig{
		ClusterName: clusterName,
		Address:     fmt.Sprintf("http://%s:%s", host, port.Port()),
		Port:        port.Port(),
		Token:       root_token,
	}

	fmt.Println("Vault container started at", vaultConfig.Address)
	return &VaultHelper{
		container: vaultContainer,
		Config:    vaultConfig,
	}, nil
}

// Container Operations
func (v *VaultHelper) Terminate(ctx context.Context) error {
	if v.container != nil {
		return v.container.Terminate(ctx)
	}
	return nil
}

func (v *VaultHelper) Stop(ctx context.Context, timeout *time.Duration) error {
	if v.container != nil {
		if v.container.IsRunning() {
			return v.container.Stop(ctx, timeout)
		}
	}
	return nil
}

func (v *VaultHelper) Start(ctx context.Context) error {
	if v.container != nil {
		if !v.container.IsRunning() {
			return v.container.Start(ctx)
		}
	}
	return nil
}

// Vault Operations

// EnableAppRoleAuth enables the AppRole authentication method in Vault.
func (v *VaultHelper) EnableAppRoleAuth(ctx context.Context) error {
	if _, err := v.ExecuteVaultCommand(ctx, "vault auth enable approle"); err != nil {
		return fmt.Errorf("failed to enable AppRole auth: %w", err)
	}
	return nil
}

// GetAppRoleID retrieves the role ID for the specified AppRole.
func (v *VaultHelper) GetAppRoleID(ctx context.Context, approle string) (string, error) {
	cmd := fmt.Sprintf("vault read -field=role_id auth/approle/role/%s/role-id", approle)
	output, err := v.ExecuteVaultCommand(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read AppRole ID: %w", err)
	}
	return output, nil
}

// GetAppSecret retrieves the secret ID for the specified AppRole.
func (v *VaultHelper) GetAppSecret(ctx context.Context, approle string) (string, error) {
	cmd := fmt.Sprintf("vault write -force -field=secret_id auth/approle/role/%s/secret-id", approle)
	output, err := v.ExecuteVaultCommand(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read AppRole secret: %w", err)
	}
	return output, nil
}

// EnableKVv2Mounts enables KV version 2 mounts for the specified paths.
func (v *VaultHelper) EnableKVv2Mounts(ctx context.Context, mounts ...string) error {
	for _, mount := range mounts {
		cmd := fmt.Sprintf("vault secrets enable -path=%s -version=2 kv", mount)
		if _, err := v.ExecuteVaultCommand(ctx, cmd); err != nil {
			return err
		}
	}
	return nil
}

// CreateApproleWithReadPermissions creates an AppRole with read permissions for the specified mounts.
// It generates a policy that allows reading and listing secrets in the specified mounts.
// It returns the AppRole ID and secret.
// The policy also includes permissions to read the mounts themselves.
func (v *VaultHelper) CreateApproleWithReadPermissions(ctx context.Context, approle string, mounts ...string) (string, string, error) {
	for _, mount := range mounts {
		policyPaths := []string{
			`path "auth/approle/login" { capabilities = ["create"] }`,
			`path "sys/mounts" { capabilities = ["read", "list"] }`,
			`path "sys/mounts/*" { capabilities = ["read", "list"] }`,
		}
		for _, mount := range mounts {
			policyPaths = append(policyPaths,
				fmt.Sprintf(`path "%s/data/*" { capabilities = ["read", "list"] }`, mount),
				fmt.Sprintf(`path "%s/metadata/*" { capabilities = ["read", "list"] }`, mount),
			)
		}
		policy := strings.Join(policyPaths, "\n")
		if _, err := v.ExecuteVaultCommand(ctx, fmt.Sprintf("vault policy write readwrite-%s -<<EOF\n%s\nEOF", mount, policy)); err != nil {
			return "", "", err
		}
		if _, err := v.createAppRole(ctx, approle, []string{fmt.Sprintf("readwrite-%s", mount)}); err != nil {
			return "", "", err
		}
	}

	return v.getAppRoleIDAndSecret(ctx, approle)
}

// CreateApproleWithRWPermissions creates an AppRole with read and write permissions for the specified mounts.
// It generates a policy that allows creating, updating, reading, and listing secrets in the specified mounts.
// It returns the AppRole ID and secret.
// The policy also includes permissions to read the mounts themselves.
func (v *VaultHelper) CreateApproleWithRWPermissions(ctx context.Context, approle string, mounts ...string) (roleID string, roleSecret string, err error) {
	for _, mount := range mounts {
		policyPaths := []string{
			`path "auth/approle/login" { capabilities = ["create"] }`,
			`path "sys/mounts" { capabilities = ["read", "list"] }`,
			`path "sys/mounts/*" { capabilities = ["read", "list"] }`,
		}
		for _, mount := range mounts {
			policyPaths = append(policyPaths,
				fmt.Sprintf(`path "%s/data/*" { capabilities = ["create", "update", "read", "list", "delete"]  }`, mount),
				fmt.Sprintf(`path "%s/metadata/*" { capabilities = ["create", "update", "read", "list", "delete"]  }`, mount),
			)
		}
		policy := strings.Join(policyPaths, "\n")
		if _, err := v.ExecuteVaultCommand(ctx, fmt.Sprintf("vault policy write readwrite-%s -<<EOF\n%s\nEOF", mount, policy)); err != nil {
			return "", "", err
		}
		if _, err := v.createAppRole(ctx, approle, []string{fmt.Sprintf("readwrite-%s", mount)}); err != nil {
			return "", "", err
		}
	}
	return v.getAppRoleIDAndSecret(ctx, approle)
}

// WriteSecret writes a secret to the specified path in the KV store.
func (v *VaultHelper) WriteSecret(ctx context.Context, mount, path string, data map[string]string) (string, error) {
	cmd := fmt.Sprintf("vault kv put %s/%s %s", mount, path, formatDataForVault(data))
	return v.ExecuteVaultCommand(ctx, cmd)
}

// DeleteSecret deletes a secret at the specified path
func (v *VaultHelper) DeleteSecret(ctx context.Context, secretPath string) (string, error) {
	cmd := fmt.Sprintf("vault kv delete %s", secretPath)
	return v.ExecuteVaultCommand(ctx, cmd)
}

// ReadSecretData reads a secret and returns only the data fields as a map.
// This is useful when you only need the actual secret values.
func (v *VaultHelper) ReadSecretData(ctx context.Context, mount, path string) (map[string]string, int64, error) {
	cmd := fmt.Sprintf("vault kv get -format=json %s/%s", mount, path)
	output, err := v.ExecuteVaultCommand(ctx, cmd)
	if err != nil {
		return nil, -1, fmt.Errorf("failed to read secret data: %w", err)
	}

	secrets, version, err := extractSecretDataFromResponse(output)
	if err != nil {
		return nil, -1, err
	}

	return secrets, version, nil
}

// SetTokenTTL sets the token TTL and max TTL for the specified AppRole.
// It returns the output of the command execution.
func (v *VaultHelper) SetTokenTTL(ctx context.Context, approle string, ttl string, maxTTL string) (string, error) {
	cmd := fmt.Sprintf("vault write auth/approle/role/%s token_ttl=%s token_max_ttl=%s", approle, ttl, maxTTL)
	return v.ExecuteVaultCommand(ctx, cmd)
}

// QuickReset performs a faster reset by only clearing the most common test artifacts
// Use this if you know what specific resources need to be cleaned up
func (v *VaultHelper) QuickReset(ctx context.Context, mounts ...string) error {
	for _, mount := range mounts {
		cmd := fmt.Sprintf("vault secrets disable %s", mount)
		_, err := v.ExecuteVaultCommand(ctx, cmd)
		if err != nil {
			return fmt.Errorf("failed to disable KV mount %s: %w", mount, err)
		}
	}

	_, err := v.ExecuteVaultCommand(ctx, "vault auth disable approle")
	if err != nil {
		return fmt.Errorf("failed to disable AppRole auth: %w", err)
	}

	testPolicies := []string{"readwrite", "readonly"}
	for _, policy := range testPolicies {
		for _, mount := range mounts {
			policyName := fmt.Sprintf("%s-%s", policy, mount)
			cmd := fmt.Sprintf("vault policy delete %s", policyName)
			_, err := v.ExecuteVaultCommand(ctx, cmd)
			if err != nil {
				return fmt.Errorf("failed to delete policy %s: %w", policyName, err)
			}
		}
	}

	return nil
}

// ExecuteVaultCommand executes a command in the Vault container and returns the output.
// It uses the `sh -c` command to allow for complex commands and redirection.
func (v *VaultHelper) ExecuteVaultCommand(ctx context.Context, command string) (string, error) {
	_, output, err := v.container.Exec(ctx, []string{"sh", "-c", command}, exec.Multiplexed())
	if err != nil {
		return "", fmt.Errorf("failed to execute command %q in Vault container: %w", command, err)
	}

	byteOutput, _ := io.ReadAll(output)
	if os.Getenv("DEBUG_TESTCONTAINERS") != "" {
		fmt.Printf("Command: %s\nOutput: %s\n", command, string(byteOutput))
	}
	return string(byteOutput), nil
}

func formatDataForVault(data map[string]string) string {
	var formatted []string
	for key, value := range data {
		formatted = append(formatted, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(formatted, " ")
}

func (v *VaultHelper) createAppRole(ctx context.Context, roleName string, policies []string) (string, error) {
	cmd := fmt.Sprintf("vault write auth/approle/role/%s policies=%s", roleName, strings.Join(policies, ","))
	return v.ExecuteVaultCommand(ctx, cmd)
}

func (v *VaultHelper) getAppRoleIDAndSecret(ctx context.Context, approle string) (string, string, error) {
	role_id, err := v.GetAppRoleID(ctx, approle)
	if err != nil {
		return "", "", fmt.Errorf("failed to get AppRole ID: %w", err)
	}
	role_secret, err := v.GetAppSecret(ctx, approle)
	if err != nil {
		return "", "", fmt.Errorf("failed to get AppRole secret: %w", err)
	}
	return role_id, role_secret, nil
}

func extractSecretDataFromResponse(jsonStr string) (secretData map[string]string, version int64, err error) {
	var response map[string]any
	if strings.HasPrefix(jsonStr, "No value found at") {
		return nil, 0, fmt.Errorf("no secret found")
	}
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		return nil, 0, fmt.Errorf("failed to parse JSON: %w", err)
	}

	data, ok := response["data"].(map[string]any)
	if !ok {
		return nil, 0, fmt.Errorf("missing or invalid 'data' field")
	}

	rawSecretData, ok := data["data"].(map[string]any)
	if !ok {
		return nil, 0, fmt.Errorf("missing or invalid 'data.data' field")
	}

	secretData = make(map[string]string, len(rawSecretData))
	for k, v := range rawSecretData {
		str, ok := v.(string)
		if !ok {
			return nil, 0, fmt.Errorf("non-string value for key %q: %T", k, v)
		}
		secretData[k] = str
	}

	metadata, ok := data["metadata"].(map[string]any)
	if !ok {
		return secretData, 0, fmt.Errorf("missing or invalid 'data.metadata' field")
	}

	versionFloat, ok := metadata["version"].(float64)
	if !ok {
		return secretData, 0, fmt.Errorf("missing or invalid 'data.metadata.version' field")
	}

	return secretData, int64(versionFloat), nil
}
