package vault

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"vault-sync/internal/config"
	"vault-sync/testutil"
)

type MultiClusterVaultClientTestSuite struct {
	suite.Suite
	mainVault  *testutil.VaultHelper
	mainConfig *config.MainCluster

	replica1Vault *testutil.VaultHelper
	replica2Vault *testutil.VaultHelper
	replicaConfig []*config.ReplicaCluster

	client *MultiClusterVaultClient
	ctx    context.Context
}

var mounts = []string{"team-a", "team-b", "team-c"}

func (suite *MultiClusterVaultClientTestSuite) SetupSuite() {
	suite.ctx = context.Background()
}

func (suite *MultiClusterVaultClientTestSuite) BeforeTest(suiteName, testName string) {
	clusters, err := testutil.NewVaultClusters(suite.T(), suite.ctx, 2)
	suite.NoError(err, "Failed to create MultiClusterVaultClient")

	suite.mainVault = clusters.MainVaultCluster
	suite.replica1Vault = clusters.ReplicasClusters[0]
	suite.replica2Vault = clusters.ReplicasClusters[1]

	for _, vaultHelper := range []*testutil.VaultHelper{
		suite.mainVault,
		suite.replica1Vault,
		suite.replica2Vault,
	} {
		err := vaultHelper.EnableAppRoleAuth(suite.ctx)
		suite.NoError(err, "Failed to enable AppRole auth method")
		err = vaultHelper.EnableKVv2Mounts(suite.ctx, mounts...)
		suite.NoError(err, "Failed to enable KV v2 mounts")
	}

	switch testName {
	case
		"TestCreateNewMultiClusterClient",
		"TestGetSecretMounts",
		"TestGetKeysUnderMount":
		suite.mainConfig, suite.replicaConfig = suite.setupMultiClusterVaultClientTestSuite()
	}
}

func (suite *MultiClusterVaultClientTestSuite) TearDownSuite() {

	for _, vaultHelper := range []*testutil.VaultHelper{
		suite.mainVault,
		suite.replica1Vault,
		suite.replica2Vault,
	} {
		if vaultHelper != nil {
			err := vaultHelper.Terminate(suite.ctx)
			suite.NoError(err, "Failed to terminate vault helper")
			vaultHelper = nil
		}
	}
}

func (suite *MultiClusterVaultClientTestSuite) TearDownTest() {
	for _, vaultHelper := range []*testutil.VaultHelper{
		suite.mainVault,
		suite.replica1Vault,
		suite.replica2Vault,
	} {
		if vaultHelper != nil {
			err := vaultHelper.Terminate(suite.ctx)
			suite.NoError(err, "Failed to terminate vault helper")
		}
	}
	suite.mainVault = nil
	suite.replica1Vault = nil
	suite.replica2Vault = nil
}

func TestMultiClusterVaultClientSuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}

	suite.Run(t, new(MultiClusterVaultClientTestSuite))
}

type vaultAuthenticationTestCase struct {
	name          string
	clusterName   string
	expectedErr   error
	shouldSucceed bool
}

func (suite *MultiClusterVaultClientTestSuite) TestCreateNewMultiClusterClient() {
	ctx := context.Background()

	suite.Run("does not return error if authentication is successful", func() {
		_, err := NewMultiClusterVaultClient(ctx, suite.mainConfig, suite.replicaConfig)

		suite.NoError(err, "Failed to create MultiClusterVaultClient")
	})

	suite.Run("returns error if authentication failed for clusters", func() {
		suite.Run("main cluster authentication fails", func() {
			brokenMainConfig := testutil.CopyStruct(suite.mainConfig)
			brokenMainConfig.AppRoleSecret = "invalid-secret"
			fmt.Println("Broken main config:", brokenMainConfig)

			_, err := NewMultiClusterVaultClient(ctx, brokenMainConfig, suite.replicaConfig)

			suite.Error(err, "Expected error when creating MultiClusterVaultClient")
			suite.ErrorContains(err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenMainConfig.AppRoleID, brokenMainConfig.AppRoleMount))
		})

		suite.Run("replica 1 cluster authentication fails", func() {
			brokenReplica1Config := testutil.CopyStruct(suite.replicaConfig[0])
			brokenReplica1Config.AppRoleSecret = "invalid-secret"
			fmt.Println("original main config:", suite.mainConfig)
			_, err := NewMultiClusterVaultClient(ctx, suite.mainConfig, []*config.ReplicaCluster{brokenReplica1Config})

			suite.ErrorContains(err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenReplica1Config.AppRoleID, brokenReplica1Config.AppRoleMount))
		})

		suite.Run("replica 2 cluster authentication fails", func() {
			brokenReplica2Config := testutil.CopyStruct(suite.replicaConfig[1])
			brokenReplica2Config.AppRoleSecret = "invalid-secret"
			_, err := NewMultiClusterVaultClient(ctx, suite.mainConfig, []*config.ReplicaCluster{brokenReplica2Config})

			suite.ErrorContains(err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenReplica2Config.AppRoleID, brokenReplica2Config.AppRoleMount))
		})
	})
}

func (suite *MultiClusterVaultClientTestSuite) TestGetSecretMounts() {
	suite.mainVault.EnableKVv2Mounts(suite.ctx, "common", "main_cluster_mount")
	suite.replica1Vault.EnableKVv2Mounts(suite.ctx, "common")
	type mountTestCase struct {
		name           string
		secretPaths    []string
		expectedMounts []string
		expectError    bool
		errorMsg       string
	}

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
		{
			name:        "empty secret paths returns error",
			secretPaths: []string{},
			expectError: true,
			errorMsg:    "no valid mounts found in provided secret paths",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			mclient, _ := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)

			mounts, error := mclient.GetSecretMounts(suite.ctx, tc.secretPaths)

			if tc.expectError {
				suite.ErrorContains(error, tc.errorMsg, "Expected error message to contain: %s", tc.errorMsg)
			} else {
				suite.ElementsMatch(mounts, tc.expectedMounts, "Expected mounts to match: %v", mounts)
			}
		})
	}
}

func (suite *MultiClusterVaultClientTestSuite) TestGetKeysUnderMount() {
	type keyTestCase struct {
		name         string
		mount        string
		setupSecrets map[string]map[string]string // path -> secret data
		expectedKeys []string
		expectError  bool
		errorMsg     string
	}

	testCases := []keyTestCase{
		{
			name:  "retrieve keys from mount with nested structure",
			mount: "team-a",
			setupSecrets: map[string]map[string]string{
				"team-a/app1/database":                   {"host": "db1.example.com", "password": "secret1"},
				"team-a/app1/api":                        {"key": "api-key-123"},
				"team-a/app2/database":                   {"host": "db2.example.com", "password": "secret2"},
				"team-a/shared/config":                   {"env": "production"},
				"team-a/infrastructure/k8s":              {"cluster": "prod-cluster"},
				"team-a/infrastructure/internal/grafana": {"username": "test-user"},
			},
			expectedKeys: []string{
				"app1/database",
				"app1/api",
				"app2/database",
				"shared/config",
				"infrastructure/k8s",
				"infrastructure/internal/grafana",
			},
			expectError: false,
		},
		{
			name:  "retrieve keys from mount with single level",
			mount: "team-b",
			setupSecrets: map[string]map[string]string{
				"team-b/database": {"host": "db.example.com"},
				"team-b/api":      {"key": "api-key"},
				"team-b/cache":    {"redis": "redis.example.com"},
			},
			expectedKeys: []string{"database", "api", "cache"},
			expectError:  false,
		},
		{
			name:         "empty mount returns empty list",
			mount:        "team-c",
			setupSecrets: map[string]map[string]string{},
			expectedKeys: []string{},
			expectError:  false,
		},
		{
			name:        "non-existent mount returns error",
			mount:       "non-existent",
			expectError: true,
			errorMsg:    "failed to get keys under mount non-existent",
		},
		{
			name:        "empty mount parameter returns error",
			mount:       "",
			expectError: true,
			errorMsg:    "mount cannot be empty",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			// Setup: Create secrets in main cluster only
			for secretPath, secretData := range tc.setupSecrets {
				_, err := suite.mainVault.WriteSecret(suite.ctx, tc.mount, secretPath, secretData)
				suite.NoError(err, "Failed to write secret %s", secretPath)
			}

			// Create client
			mclient, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err, "Failed to create MultiClusterVaultClient")

			// Test the function
			keys, err := mclient.GetKeysUnderMount(suite.ctx, tc.mount)

			if tc.expectError {
				suite.Error(err, "Expected error for test case: %s", tc.name)
				suite.ErrorContains(err, tc.errorMsg, "Expected error message to contain: %s", tc.errorMsg)
			} else {
				suite.NoError(err, "Expected no error for test case: %s", tc.name)
				suite.ElementsMatch(tc.expectedKeys, keys, "Expected keys to match for test case: %s", tc.name)
			}

			// Cleanup: Remove secrets after test
			for secretPath := range tc.setupSecrets {
				suite.mainVault.DeleteSecret(suite.ctx, secretPath)
			}
		})
	}
}

func (suite *MultiClusterVaultClientTestSuite) setupMultiClusterVaultClientTestSuite() (*config.MainCluster, []*config.ReplicaCluster) {
	mainConfig := &config.MainCluster{
		Address:       suite.mainVault.Config.Address,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}
	mainConfig.AppRoleID, mainConfig.AppRoleSecret, _ = suite.mainVault.CreateApproleWithReadPermissions(suite.ctx, "main", mounts...)

	replica1Config := &config.ReplicaCluster{
		Name:          suite.replica1Vault.Config.ClusterName,
		Address:       suite.replica1Vault.Config.Address,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}
	replica1Config.AppRoleID, replica1Config.AppRoleSecret, _ = suite.replica1Vault.CreateApproleWithRWPermissions(suite.ctx, "replica-1", mounts...)

	replica2Config := &config.ReplicaCluster{
		Name:          suite.replica2Vault.Config.ClusterName,
		Address:       suite.replica2Vault.Config.Address,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}
	replica2Config.AppRoleID, replica2Config.AppRoleSecret, _ = suite.replica2Vault.CreateApproleWithRWPermissions(suite.ctx, "replica-2", mounts...)

	return mainConfig, []*config.ReplicaCluster{replica1Config, replica2Config}
}
