package vault

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"vault-sync/internal/config"
	"vault-sync/testutil"
)

type MultiClusterVaultClientTestSuite struct {
	suite.Suite
	mainVault     *testutil.VaultHelper
	replica1Vault *testutil.VaultHelper
	replica2Vault *testutil.VaultHelper
	allInstances  []*testutil.VaultHelper
	client        *MultiClusterVaultClient
}

func TestMultiClusterVaultClientSuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(MultiClusterVaultClientTestSuite))
}

var mounts = []string{"team-a", "team-b", "team-c"}

func (vaultTest *MultiClusterVaultClientTestSuite) SetupSuite() {
	ctx := context.Background()
	clusters, err := testutil.NewVaultClusters(vaultTest.T(), ctx, 2)
	require.NoError(vaultTest.T(), err, "Failed to create MultiClusterVaultClient")
	vaultTest.mainVault = clusters.MainVaultCluster
	vaultTest.replica1Vault = clusters.ReplicasClusters[0]
	vaultTest.replica2Vault = clusters.ReplicasClusters[1]
	vaultTest.allInstances = []*testutil.VaultHelper{
		clusters.MainVaultCluster,
		clusters.ReplicasClusters[0],
		clusters.ReplicasClusters[1],
	}
}

func (vaultTest *MultiClusterVaultClientTestSuite) TearDownSuite() {
	ctx := context.Background()
	for _, vaultHelper := range vaultTest.allInstances {
		err := vaultHelper.Terminate(ctx)
		require.NoError(vaultTest.T(), err, "Failed to terminate vault helper")
	}
}

func (vaultTest *MultiClusterVaultClientTestSuite) SetupTest() {
	ctx := context.Background()

	for _, vaultHelper := range vaultTest.allInstances {
		err := vaultHelper.EnableAppRoleAuth(ctx)
		require.NoError(vaultTest.T(), err, "Failed to enable AppRole auth method")
		err = vaultHelper.EnableKVv2Mounts(ctx, mounts...)
		require.NoError(vaultTest.T(), err, "Failed to enable KV v2 mounts")
	}
}

type vaultAuthenticationTestCase struct {
	name          string
	clusterName   string
	expectedErr   error
	shouldSucceed bool
}

func (vaultTest *MultiClusterVaultClientTestSuite) TestCreateNewMultiClusterClient() {
	mainConfig, replicaConfigs := vaultTest.setupMultiClusterVaultClientTestSuite()
	ctx := context.Background()

	vaultTest.T().Run("does not return error if authentication is successful", func(t *testing.T) {
		_, err := NewMultiClusterVaultClient(ctx, mainConfig, replicaConfigs)

		assert.NoError(vaultTest.T(), err, "Failed to create MultiClusterVaultClient")
	})

	vaultTest.T().Run("returns error if authentication failed for clusters", func(t *testing.T) {
		vaultTest.T().Run("main cluster authentication fails", func(t *testing.T) {
			brokenMainConfig := mainConfig
			brokenMainConfig.AppRoleSecret = "invalid-secret"
			_, err := NewMultiClusterVaultClient(ctx, brokenMainConfig, replicaConfigs)

			assert.Error(vaultTest.T(), err, "Expected error when creating MultiClusterVaultClient")
			assert.ErrorContains(vaultTest.T(), err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenMainConfig.AppRoleID, brokenMainConfig.AppRoleMount))
		})

		vaultTest.T().Run("replica 1 cluster authentication fails", func(t *testing.T) {
			brokenReplica1Config := replicaConfigs[0]
			brokenReplica1Config.AppRoleSecret = "invalid-secret"
			_, err := NewMultiClusterVaultClient(ctx, mainConfig, []config.ReplicaCluster{brokenReplica1Config})

			assert.ErrorContains(vaultTest.T(), err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenReplica1Config.AppRoleID, brokenReplica1Config.AppRoleMount))
		})

		vaultTest.T().Run("replica 2 cluster authentication fails", func(t *testing.T) {
			brokenReplica2Config := replicaConfigs[1]
			brokenReplica2Config.AppRoleSecret = "invalid-secret"
			_, err := NewMultiClusterVaultClient(ctx, mainConfig, []config.ReplicaCluster{brokenReplica2Config})

			assert.ErrorContains(vaultTest.T(), err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenReplica2Config.AppRoleID, brokenReplica2Config.AppRoleMount))
		})
	})

}

type mountTestCase struct {
	name           string
	secretPaths    []string
	expectedMounts []string
	expectError    bool
	errorMsg       string
}

func (vaultTest *MultiClusterVaultClientTestSuite) TestGetSecretMountsExist() {
	ctx := context.Background()
	mainConfig, replicaConfigs := vaultTest.setupMultiClusterVaultClientTestSuite()
	vaultTest.mainVault.EnableKVv2Mounts(ctx, "common", "main_cluster_mount")
	vaultTest.replica1Vault.EnableKVv2Mounts(ctx, "common")

	testCases := []mountTestCase{
		{
			name:           "mounts exist in all clusters",
			secretPaths:    []string{"team-a/myapp/database", "team-b/myapp/config"},
			expectedMounts: []string{"team-a", "team-b"},
			expectError:    false,
		},
		{
			name:           "duplicated mounts in paths returns unique mounts",
			secretPaths:    []string{"team-a/myapp/database", "team-a/myapp", "team-b/infra/myinfratool"},
			expectedMounts: []string{"team-a", "team-b"},
			expectError:    false,
		},
		{
			// there are multiple mounts enabled, but only those in secret paths should be returned
			name:           "returns only mounts that exist in secret paths if multiple mounts are enabled",
			secretPaths:    []string{"team-a/myapp/database", "team-a/myapp", "team-c/infra/myinfratool"},
			expectedMounts: []string{"team-a", "team-c"},
			expectError:    false,
		},
		{
			name:        "mounts does not exist in main cluster",
			secretPaths: []string{"kv/myapp/database", "do_not_exist/infra/myinfratool"},
			expectError: true,
			errorMsg:    "missing mounts in main cluster",
		},
		{
			name:        "mounts exists in main cluster but not in replica clusters",
			secretPaths: []string{"main_cluster_mount/myapp/database"},
			expectError: true,
			errorMsg:    "missing mounts in replica cluster",
		},
		{
			name:        "mounts exists in main and one replica cluster but not in the other",
			secretPaths: []string{"team-a/myapp/database", "team-b/infra/myinfratool", "common/myapp/config"},
			expectError: true,
			errorMsg:    "missing mounts in replica cluster",
		},
	}

	for _, tc := range testCases {
		vaultTest.T().Run(tc.name, func(t *testing.T) {
			mclient, _ := NewMultiClusterVaultClient(ctx, mainConfig, replicaConfigs)

			mounts, error := mclient.GetSecretMounts(ctx, tc.secretPaths)

			if tc.expectError {
				assert.ErrorContains(t, error, tc.errorMsg, "Expected error message to contain: %s", tc.errorMsg)
			} else {
				assert.ElementsMatch(t, mounts, tc.expectedMounts, "Expected mounts to match: %v", mounts)
			}
		})
	}
}

func (vaultTest *MultiClusterVaultClientTestSuite) setupMultiClusterVaultClientTestSuite() (config.MainCluster, []config.ReplicaCluster) {
	ctx := context.Background()
	roleID, appSecret, _ := vaultTest.mainVault.CreateApproleWithReadPermissions(ctx, "main", mounts...)
	mainConfig := config.MainCluster{
		Address:       vaultTest.mainVault.Config.Address,
		AppRoleID:     roleID,
		AppRoleSecret: appSecret,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}

	replica1RoleID, replica1AppSecret, _ := vaultTest.replica1Vault.CreateApproleWithRWPermissions(ctx, "replica-1", mounts...)
	replica2RoleID, replica2AppSecret, _ := vaultTest.replica2Vault.CreateApproleWithRWPermissions(ctx, "replica-2", mounts...)
	replicaConfigs := []config.ReplicaCluster{
		{
			Name:          vaultTest.replica1Vault.Config.ClusterName,
			Address:       vaultTest.replica1Vault.Config.Address,
			AppRoleID:     replica1RoleID,
			AppRoleSecret: replica1AppSecret,
			AppRoleMount:  "approle",
			TLSSkipVerify: true,
		},
		{
			Name:          vaultTest.replica2Vault.Config.ClusterName,
			Address:       vaultTest.replica2Vault.Config.Address,
			AppRoleID:     replica2RoleID,
			AppRoleSecret: replica2AppSecret,
			AppRoleMount:  "approle",
			TLSSkipVerify: true,
		},
	}
	return mainConfig, replicaConfigs
}
