package testutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
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
	if err := v.executeVaultCommand(ctx, "vault auth enable approle"); err != nil {
		return fmt.Errorf("failed to enable AppRole auth: %w", err)
	}
	return nil
}

func (v *VaultHelper) GetAppSecret(ctx context.Context, approle string) (string, error) {
	cmd := fmt.Sprintf("vault write -force -field=secret_id auth/approle/role/%s/secret-id", approle)
	_, output, err := v.container.Exec(ctx, []string{"sh", "-c", cmd})
	if err != nil {
		return "", fmt.Errorf("failed to read AppRole secret: %w", err)
	}
	byteOutput, _ := io.ReadAll(output)
	return strings.TrimSpace(cleanOutput(string(byteOutput))), nil
}

func (v *VaultHelper) EnableKVv2Mounts(ctx context.Context, mounts ...string) error {
	for _, mount := range mounts {
		cmd := fmt.Sprintf("vault secrets enable -path=%s -version=2 kv", mount)
		if err := v.executeVaultCommand(ctx, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (v *VaultHelper) CreateApproleWithReadPermissions(ctx context.Context, approle string, mounts ...string) error {
	for _, mount := range mounts {
		policy := fmt.Sprintf(`
	path "sys/mounts" {
  	capabilities = ["read", "list"]
	}
	path "sys/mounts/*" {
		capabilities = ["read", "list"]
	}
	path "%s/data/*" { 
		capabilities = ["read", "list"] 
	}
	path "%s/metadata/*" { 
		capabilities = ["read", "list"] 
	}
	`, mount, mount)

		if err := v.executeVaultCommand(ctx, fmt.Sprintf("vault policy write readonly-%s -<<EOF\n%s\nEOF", mount, formatEoFInput(policy))); err != nil {
			return err
		}

		if err := v.createAppRole(ctx, approle, []string{fmt.Sprintf("readonly-%s", mount)}); err != nil {
			return err
		}
	}
	return nil
}

func (v *VaultHelper) CreateApproleWithRWPermissions(ctx context.Context, approle string, mounts ...string) error {
	for _, mount := range mounts {
		policy := fmt.Sprintf(`
	path "sys/mounts" {
  	capabilities = ["read", "list"]
	}
	path "sys/mounts/*" {
		capabilities = ["read", "list"]
	}
	path "%s/data/*" { 
		capabilities = ["create", "update", "read", "list"] 
	}
	path "%s/metadata/*" { 
		capabilities = ["create", "update", "read", "list"] 
	}
	`, mount, mount)

		if err := v.executeVaultCommand(ctx, fmt.Sprintf("vault policy write readwrite-%s -<<EOF\n%s\nEOF", mount, formatEoFInput(policy))); err != nil {
			return err
		}
		v.executeVaultCommand(ctx, "vault policy list")
		if err := v.createAppRole(ctx, approle, []string{fmt.Sprintf("readwrite-%s", mount)}); err != nil {
			return err
		}
	}
	return nil
}

func (v *VaultHelper) WriteSecret(ctx context.Context, mount, path string, data map[string]string) error {
	cmd := fmt.Sprintf("vault kv put %s/%s %s", mount, path, formatDataForVault(data))
	return v.executeVaultCommand(ctx, cmd)
}

func formatDataForVault(data map[string]string) string {
	var formatted []string
	for key, value := range data {
		formatted = append(formatted, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(formatted, " ")
}

func (v *VaultHelper) executeVaultCommand(ctx context.Context, command string) error {
	_, output, err := v.container.Exec(ctx, []string{"sh", "-c", command})
	if err != nil {
		return fmt.Errorf("failed to execute command %q in Vault container: %w", command, err)
	}
	if os.Getenv("DEBUG_TESTCONTAINERS") != "" {
		fmt.Println("Executing Vault command:", command)
		byteOutput, _ := io.ReadAll(output)
		fmt.Println("Vault command output:", string(byteOutput))
	}
	return nil
}

func (v *VaultHelper) createAppRole(ctx context.Context, roleName string, policies []string) error {
	cmd := fmt.Sprintf("vault write auth/approle/role/%s policies=%s", roleName, strings.Join(policies, ","))
	return v.executeVaultCommand(ctx, cmd)
}

func formatEoFInput(input string) string {
	return input
	// return strings.ReplaceAll(input, "\n", "\\n")
}

func cleanOutput(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) || r == '\n' || r == '\r' {
			return r
		}
		return -1
	}, s)
}
