package vault

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"vault-sync/internal/config"
	"vault-sync/internal/models"
	"vault-sync/testutil"
)

type MultiClusterVaultClientTestSuite struct {
	suite.Suite
	mainVault  *testutil.VaultHelper
	mainConfig *config.VaultClusterConfig

	replica1Vault *testutil.VaultHelper
	replica2Vault *testutil.VaultHelper
	replicaConfig []*config.VaultClusterConfig

	ctx context.Context
}

var mounts = []string{"team-a", "team-b", "team-c"}

func (suite *MultiClusterVaultClientTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	clusters, err := testutil.NewVaultClusters(suite.T(), suite.ctx, 2)
	suite.NoError(err, "Failed to create MultiClusterVaultClient")

	suite.mainVault = clusters.MainVaultCluster
	suite.replica1Vault = clusters.ReplicasClusters[0]
	suite.replica2Vault = clusters.ReplicasClusters[1]
}

func (suite *MultiClusterVaultClientTestSuite) SetupSubTest() {
	wg := sync.WaitGroup{}
	for _, vaultHelper := range []*testutil.VaultHelper{
		suite.mainVault,
		suite.replica1Vault,
		suite.replica2Vault,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vaultHelper.Start(suite.ctx)
			err := vaultHelper.EnableAppRoleAuth(suite.ctx)
			suite.NoError(err, "Failed to enable AppRole auth method")
			err = vaultHelper.EnableKVv2Mounts(suite.ctx, mounts...)
			suite.NoError(err, "Failed to enable KV v2 mounts")
		}()
	}
	wg.Wait()
	suite.mainConfig, suite.replicaConfig = suite.setupMultiClusterVaultClientTestSuite()
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

func (suite *MultiClusterVaultClientTestSuite) TearDownSubTest() {
	wg := sync.WaitGroup{}

	for _, vaultHelper := range []*testutil.VaultHelper{
		suite.mainVault,
		suite.replica1Vault,
		suite.replica2Vault,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vaultHelper.Start(suite.ctx)
			err := vaultHelper.QuickReset(suite.ctx, mounts...)
			suite.NoError(err, "Failed to reset vault helper")
		}()
	}
	wg.Wait()
}

func TestMultiClusterVaultClientSuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}

	suite.Run(t, new(MultiClusterVaultClientTestSuite))
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

			_, err := NewMultiClusterVaultClient(ctx, brokenMainConfig, suite.replicaConfig)

			suite.Error(err, "Expected error when creating MultiClusterVaultClient")
			suite.ErrorContains(err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenMainConfig.AppRoleID, brokenMainConfig.AppRoleMount))
		})

		suite.Run("replica 1 cluster authentication fails", func() {
			brokenReplica1Config := testutil.CopyStruct(suite.replicaConfig[0])
			brokenReplica1Config.AppRoleSecret = "invalid-secret"
			_, err := NewMultiClusterVaultClient(ctx, suite.mainConfig, []*config.VaultClusterConfig{brokenReplica1Config})

			suite.ErrorContains(err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenReplica1Config.AppRoleID, brokenReplica1Config.AppRoleMount))
		})

		suite.Run("replica 2 cluster authentication fails", func() {
			brokenReplica2Config := testutil.CopyStruct(suite.replicaConfig[1])
			brokenReplica2Config.AppRoleSecret = "invalid-secret"
			_, err := NewMultiClusterVaultClient(ctx, suite.mainConfig, []*config.VaultClusterConfig{brokenReplica2Config})

			suite.ErrorContains(err, fmt.Sprintf("failed to authenticate with role ID: %s at mount %s.", brokenReplica2Config.AppRoleID, brokenReplica2Config.AppRoleMount))
		})
	})
}

func (suite *MultiClusterVaultClientTestSuite) TestGetSecretMounts() {
	suite.mainVault.EnableKVv2Mounts(suite.ctx, "common", "main_cluster_mount")
	suite.replica1Vault.EnableKVv2Mounts(suite.ctx, "common")
	type getSecretMountsTestCase struct {
		name           string
		secretPaths    []string
		expectedMounts []string
		expectError    bool
		errorMsg       string
	}

	testCases := []getSecretMountsTestCase{
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
	type getKeysUnderMountTestCase struct {
		name         string
		mount        string
		setupSecrets map[string]map[string]string // path -> secret data
		expectedKeys []string
		expectError  bool
		errorMsg     string
	}

	testCases := []getKeysUnderMountTestCase{
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
		})
	}
}

func (suite *MultiClusterVaultClientTestSuite) TestGetSecretMetadata() {
	ctx := context.Background()
	type getSecretMetadataTestCase struct {
		name           string
		mount          string
		keyPath        string
		setupSecrets   map[string]map[string]string // version -> secret data
		expectError    bool
		errorMsg       string
		validateResult func(*MultiClusterVaultClientTestSuite, *VaultSecretMetadataResponse)
	}

	testCases := []getSecretMetadataTestCase{
		{
			name:    "retrieve metadata for existing secret with single version",
			mount:   "team-a",
			keyPath: "app1/database",
			setupSecrets: map[string]map[string]string{
				"v1": {"host": "db1.example.com", "password": "secret1"},
			},
			expectError: false,
			validateResult: func(suite *MultiClusterVaultClientTestSuite, metadata *VaultSecretMetadataResponse) {
				suite.Equal(int64(1), metadata.CurrentVersion)
				suite.Len(metadata.Versions, 1)
				suite.Contains(metadata.Versions, "1")
				suite.False(metadata.Versions["1"].Destroyed)
				suite.NotZero(metadata.CreatedTime)
				suite.NotZero(metadata.UpdatedTime)
			},
		},
		{
			name:    "retrieve metadata for secret with multiple versions",
			mount:   "team-a",
			keyPath: "app1/api",
			setupSecrets: map[string]map[string]string{
				"v1": {"key": "api-key-v1"},
				"v2": {"key": "api-key-v2", "env": "production"},
				"v3": {"key": "api-key-v3", "env": "production", "rate_limit": "1000"},
			},
			expectError: false,
			validateResult: func(suite *MultiClusterVaultClientTestSuite, metadata *VaultSecretMetadataResponse) {
				suite.Equal(int64(3), metadata.CurrentVersion)
				suite.Len(metadata.Versions, 3)
				suite.Contains(metadata.Versions, "1")
				suite.Contains(metadata.Versions, "2")
				suite.Contains(metadata.Versions, "3")

				// Verify version ordering
				v1Time := metadata.Versions["1"].CreatedTime
				v2Time := metadata.Versions["2"].CreatedTime
				v3Time := metadata.Versions["3"].CreatedTime
				suite.True(v1Time.Before(v2Time) || v1Time.Equal(v2Time))
				suite.True(v2Time.Before(v3Time) || v2Time.Equal(v3Time))
			},
		},
		{
			name:    "nested path secret metadata",
			mount:   "team-b",
			keyPath: "infrastructure/k8s/prod/secrets",
			setupSecrets: map[string]map[string]string{
				"v1": {"cluster": "prod-cluster", "namespace": "default"},
			},
			expectError: false,
			validateResult: func(suite *MultiClusterVaultClientTestSuite, metadata *VaultSecretMetadataResponse) {
				suite.Equal(int64(1), metadata.CurrentVersion)
				suite.Len(metadata.Versions, 1)
			},
		},
		{
			name:        "non-existent secret returns error",
			mount:       "team-a",
			keyPath:     "non-existent/secret",
			expectError: true,
			errorMsg:    "failed to read metadata",
		},
		{
			name:        "empty mount parameter returns error",
			mount:       "",
			keyPath:     "some/path",
			expectError: true,
			errorMsg:    "mount cannot be empty",
		},
		{
			name:        "empty key path parameter returns error",
			mount:       "team-a",
			keyPath:     "",
			expectError: true,
			errorMsg:    "key path cannot be empty",
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			for version, secretData := range tc.setupSecrets {
				_, err := suite.mainVault.WriteSecret(ctx, tc.mount, tc.keyPath, secretData)
				suite.NoError(err, "Failed to write secret %s version %s", tc.keyPath, version)
				time.Sleep(10 * time.Millisecond)
			}

			mclient, err := NewMultiClusterVaultClient(ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err, "Failed to create MultiClusterVaultClient")

			metadata, err := mclient.GetSecretMetadata(ctx, tc.mount, tc.keyPath)

			if tc.expectError {
				suite.Error(err, "Expected error for test case: %s", tc.name)
				suite.ErrorContains(err, tc.errorMsg, "Expected error message to contain: %s", tc.errorMsg)
			} else {
				suite.NoError(err, "Expected no error for test case: %s", tc.name)
				suite.NotNil(metadata, "Expected metadata to be returned")
				tc.validateResult(suite, metadata)
			}
		})
	}
}

func (suite *MultiClusterVaultClientTestSuite) TestSyncSecretToReplicas() {
	mount := "team-a"
	keyPath := "app/database"
	secret := map[string]string{
		"host":     "main-db.example.com",
		"username": "admin",
		"password": "secret123",
	}

	type syncResultAssertion struct {
		vaultHelper    *testutil.VaultHelper
		result         *models.SyncedSecret
		expectedSecret map[string]string
		expectedStatus models.SyncStatus
		mount          string
		keypath        string
	}

	assertResult := func(assertionParams []*syncResultAssertion) {
		for _, params := range assertionParams {
			cluster := params.vaultHelper.Config.ClusterName
			suite.NotNil(params.result, "Expected result to be non-nil for cluster %s", cluster)

			data, version, _ := params.vaultHelper.ReadSecretData(suite.ctx, params.mount, params.keypath)

			suite.Equal(cluster, params.result.DestinationCluster, "Expected destination cluster to match for cluster %s", cluster)
			suite.Equal(params.mount, params.result.SecretBackend, "Expected secret backend to match for cluster %s", cluster)
			suite.Equal(params.keypath, params.result.SecretPath, "Expected secret path to match for cluster %s", cluster)
			suite.Equal(params.expectedStatus, params.result.Status, "Expected status to match for cluster %s", cluster)

			if params.result.Status == models.StatusSuccess {
				suite.Equal(params.expectedSecret, data, "Expected secret data to match for cluster %s", cluster)
				suite.Equal(params.result.DestinationVersion, version, "Expected destination version to match for cluster %s", cluster)
				suite.NotNil(params.result.LastSyncAttempt, "Expected LastSyncAttempt to be set for successful sync")
				suite.Nil(params.result.ErrorMessage, "Expected no error message for successful sync")
			} else {
				suite.NotNil(params.result.ErrorMessage, "Expected ErrorMessage to be set for failed sync")
				suite.Nil(params.result.LastSyncSuccess, "Expected LastSyncSuccess to be nil for failed sync")
			}
		}
	}

	suite.Run("successful sync to all replicas new secret", func() {
		suite.mainVault.WriteSecret(suite.ctx, mount, keyPath, secret)
		mclient, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err, "Failed to create MultiClusterVaultClient")

		results, err := mclient.SyncSecretToReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err, "Expected no error for successful sync")
		suite.NotNil(results, "Expected results to be returned")
		suite.Len(results, 2, "Expected results for 2 replicas")

		assertResult([]*syncResultAssertion{
			{
				vaultHelper:    suite.replica1Vault,
				result:         results[0],
				expectedStatus: models.StatusSuccess,
				mount:          mount,
				keypath:        keyPath,
				expectedSecret: secret,
			},
			{
				vaultHelper:    suite.replica2Vault,
				result:         results[1],
				expectedStatus: models.StatusSuccess,
				mount:          mount,
				keypath:        keyPath,
				expectedSecret: secret,
			},
		})
	})

	suite.Run("returns empty results if no replica is configured", func() {
		suite.mainVault.WriteSecret(suite.ctx, mount, keyPath, secret)
		mclient, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, []*config.VaultClusterConfig{})
		suite.NoError(err, "Failed to create MultiClusterVaultClient")

		twoSeconds := 2 * time.Second
		suite.replica2Vault.Stop(suite.ctx, &twoSeconds) // Simulate replica 2 being unavailable
		results, err := mclient.SyncSecretToReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err, "Expected no error for successful sync")
		suite.NotNil(results, "Expected results to be returned")
		suite.Len(results, 0, "Expected no results when no replicas are configured")
	})

	suite.Run("returns failed results when all replica become unavailable", func() {
		suite.mainVault.WriteSecret(suite.ctx, mount, keyPath, secret)
		mclient, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err, "Failed to create MultiClusterVaultClient")

		twoSeconds := 2 * time.Second
		suite.replica1Vault.Stop(suite.ctx, &twoSeconds) // Simulate replica 1 being unavailable
		suite.replica2Vault.Stop(suite.ctx, &twoSeconds) // Simulate replica 2 being unavailable
		results, err := mclient.SyncSecretToReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err, "Expected no error for successful sync")
		suite.NotNil(results, "Expected results to be returned")
		suite.Len(results, 2, "Expected results for 2 replicas")

		assertResult([]*syncResultAssertion{
			{
				vaultHelper:    suite.replica1Vault,
				result:         results[0],
				expectedStatus: models.StatusFailed,
				mount:          mount,
				keypath:        keyPath,
				expectedSecret: secret,
			},
			{
				vaultHelper:    suite.replica2Vault,
				result:         results[1],
				expectedStatus: models.StatusFailed,
				mount:          mount,
				keypath:        keyPath,
				expectedSecret: secret,
			},
		})
	})

	suite.Run("returns error", func() {
		suite.Run("for empty mount", func() {
			mclient, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err, "Failed to create MultiClusterVaultClient")

			results, err := mclient.SyncSecretToReplicas(suite.ctx, "", keyPath)

			suite.Error(err, "Expected error for empty mount")
			suite.Nil(results, "Expected no results for empty mount")
			suite.ErrorContains(err, "mount cannot be empty", "Expected error message to contain: mount cannot be empty")
		})

		suite.Run("for empty key path", func() {
			mclient, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err, "Failed to create MultiClusterVaultClient")

			results, err := mclient.SyncSecretToReplicas(suite.ctx, mount, "")

			suite.Error(err, "Expected error for empty key path")
			suite.Nil(results, "Expected no results for empty key path")
			suite.ErrorContains(err, "key path cannot be empty", "Expected error message to contain: key path cannot be empty")
		})

		suite.Run("when it fails to read source secret", func() {
			mclient, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err, "Failed to create MultiClusterVaultClient")

			results, err := mclient.SyncSecretToReplicas(suite.ctx, "wrong-mount", keyPath)

			suite.Error(err, "Expected error for failed read")
			suite.Nil(results, "Expected no results for failed read")
			suite.ErrorContains(err, "failed to read secret", "Expected error message to contain: failed to read secret")
		})
	})

}

func (suite *MultiClusterVaultClientTestSuite) setupMultiClusterVaultClientTestSuite() (*config.VaultClusterConfig, []*config.VaultClusterConfig) {
	mainConfig := &config.VaultClusterConfig{
		Name:          suite.mainVault.Config.ClusterName,
		Address:       suite.mainVault.Config.Address,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}
	mainConfig.AppRoleID, mainConfig.AppRoleSecret, _ = suite.mainVault.CreateApproleWithReadPermissions(suite.ctx, "main", mounts...)

	replica1Config := &config.VaultClusterConfig{
		Name:          suite.replica1Vault.Config.ClusterName,
		Address:       suite.replica1Vault.Config.Address,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}
	replica1Config.AppRoleID, replica1Config.AppRoleSecret, _ = suite.replica1Vault.CreateApproleWithRWPermissions(suite.ctx, "replica-1", mounts...)

	replica2Config := &config.VaultClusterConfig{
		Name:          suite.replica2Vault.Config.ClusterName,
		Address:       suite.replica2Vault.Config.Address,
		AppRoleMount:  "approle",
		TLSSkipVerify: true,
	}
	replica2Config.AppRoleID, replica2Config.AppRoleSecret, _ = suite.replica2Vault.CreateApproleWithRWPermissions(suite.ctx, "replica-2", mounts...)

	return mainConfig, []*config.VaultClusterConfig{replica1Config, replica2Config}
}
