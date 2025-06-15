package testutil

import (
	"context"
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

	"vault-sync/pkg/log"
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

	log.Logger.Info().Str("address", vaultConfig.Address).Msg("Vault container started")
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
		return v.container.Stop(ctx, timeout)
	}
	return nil
}

func (v *VaultHelper) Start(ctx context.Context) error {
	if v.container != nil {
		return v.container.Start(ctx)
	}
	return nil
}

// Vault Operations
func (v *VaultHelper) EnableAppRoleAuth(ctx context.Context) error {
	if _, err := v.ExecuteVaultCommand(ctx, "vault auth enable approle"); err != nil {
		return fmt.Errorf("failed to enable AppRole auth: %w", err)
	}
	return nil
}

func (v *VaultHelper) GetAppRoleID(ctx context.Context, approle string) (string, error) {
	cmd := fmt.Sprintf("vault read -field=role_id auth/approle/role/%s/role-id", approle)
	output, err := v.ExecuteVaultCommand(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read AppRole ID: %w", err)
	}
	return output, nil
}

func (v *VaultHelper) GetAppSecret(ctx context.Context, approle string) (string, error) {
	cmd := fmt.Sprintf("vault write -force -field=secret_id auth/approle/role/%s/secret-id", approle)
	output, err := v.ExecuteVaultCommand(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read AppRole secret: %w", err)
	}
	return output, nil
}

func (v *VaultHelper) EnableKVv2Mounts(ctx context.Context, mounts ...string) error {
	for _, mount := range mounts {
		cmd := fmt.Sprintf("vault secrets enable -path=%s -version=2 kv", mount)
		if _, err := v.ExecuteVaultCommand(ctx, cmd); err != nil {
			return err
		}
	}
	return nil
}

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

func (v *VaultHelper) CreateApproleWithRWPermissions(ctx context.Context, approle string, mounts ...string) (string, string, error) {
	for _, mount := range mounts {
		policyPaths := []string{
			`path "auth/approle/login" { capabilities = ["create"] }`,
			`path "sys/mounts" { capabilities = ["read", "list"] }`,
			`path "sys/mounts/*" { capabilities = ["read", "list"] }`,
		}
		for _, mount := range mounts {
			policyPaths = append(policyPaths,
				fmt.Sprintf(`path "%s/data/*" { capabilities = ["create", "update", "read", "list"]  }`, mount),
				fmt.Sprintf(`path "%s/metadata/*" { capabilities = ["create", "update", "read", "list"]  }`, mount),
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

func (v *VaultHelper) WriteSecret(ctx context.Context, mount, path string, data map[string]string) (string, error) {
	cmd := fmt.Sprintf("vault kv put %s/%s %s", mount, path, formatDataForVault(data))
	return v.ExecuteVaultCommand(ctx, cmd)
}

func (v *VaultHelper) ExecuteVaultCommand(ctx context.Context, command string) (string, error) {
	_, output, err := v.container.Exec(ctx, []string{"sh", "-c", command}, exec.Multiplexed())
	if err != nil {
		return "", fmt.Errorf("failed to execute command %q in Vault container: %w", command, err)
	}

	byteOutput, _ := io.ReadAll(output)
	if os.Getenv("DEBUG_TESTCONTAINERS") != "" {
		log.Logger.Info().Str("command", command).Msg("Executing Vault command")
		log.Logger.Info().Str("output", string(byteOutput)).Msg("Vault command output")
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
