package vault

import (
	"context"
	"fmt"
	"os"
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
	suite.NoError(err)

	suite.mainVault = clusters.MainVaultCluster
	suite.replica1Vault = clusters.ReplicasClusters[0]
	suite.replica2Vault = clusters.ReplicasClusters[1]
}

func (suite *MultiClusterVaultClientTestSuite) SetupSubTest() {
	helpers := []*testutil.VaultHelper{suite.mainVault, suite.replica1Vault, suite.replica2Vault}
	errors := make(chan error, len(helpers))

	for _, vaultHelper := range helpers {
		go func(vaultHelper *testutil.VaultHelper) {
			err := vaultHelper.Start(suite.ctx)
			if err != nil {
				errors <- fmt.Errorf("failed to start vault helper: %w", err)
				return
			}
			err1 := vaultHelper.EnableAppRoleAuth(suite.ctx)
			err2 := vaultHelper.EnableKVv2Mounts(suite.ctx, mounts...)
			if err1 != nil {
				errors <- fmt.Errorf("failed to enable AppRole auth method: %w", err1)
				return
			} else if err2 != nil {
				errors <- fmt.Errorf("failed to enable KV v2 mounts: %w", err2)
				return
			}
			errors <- nil
		}(vaultHelper)
	}

	suite.handleTestSetUpTearDownErrors(errors, len(helpers))

	suite.mainConfig, suite.replicaConfig = suite.setupMultiClusterVaultClientTestSuite()
}

func (suite *MultiClusterVaultClientTestSuite) TearDownSuite() {
	helpers := []*testutil.VaultHelper{suite.mainVault, suite.replica1Vault, suite.replica2Vault}
	errors := make(chan error, len(helpers))
	for _, helper := range helpers {
		if helper != nil {
			go func(helper *testutil.VaultHelper) {
				err := helper.Terminate(suite.ctx)
				errors <- err
			}(helper)
		}
	}

	suite.handleTestSetUpTearDownErrors(errors, len(helpers))

	suite.mainVault = nil
	suite.replica1Vault = nil
	suite.replica2Vault = nil
}

func (suite *MultiClusterVaultClientTestSuite) TearDownSubTest() {
	helpers := []*testutil.VaultHelper{suite.mainVault, suite.replica1Vault, suite.replica2Vault}
	errors := make(chan error, len(helpers))

	for _, vaultHelper := range helpers {
		go func(vaultHelper *testutil.VaultHelper) {
			vaultHelper.Start(suite.ctx)
			err := vaultHelper.QuickReset(suite.ctx, mounts...)
			errors <- err
		}(vaultHelper)
	}

	suite.handleTestSetUpTearDownErrors(errors, len(helpers))
}

func (suite *MultiClusterVaultClientTestSuite) handleTestSetUpTearDownErrors(errors chan error, expectedCount int) {
	for i := 0; i < expectedCount; i++ {
		select {
		case err := <-errors:
			if err != nil {
				suite.FailNow("Error during test setup/teardown", err.Error())
			}
		case <-time.After(10 * time.Second):
			suite.FailNow("Timed out waiting for vault helpers to terminate")
		}
	}
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

		suite.NoError(err)
	})

	suite.Run("returns error if authentication failed for clusters", func() {
		suite.Run("main cluster authentication fails", func() {
			brokenMainConfig := testutil.CopyStruct(suite.mainConfig)
			brokenMainConfig.AppRoleSecret = "invalid-secret"

			_, err := NewMultiClusterVaultClient(ctx, brokenMainConfig, suite.replicaConfig)

			suite.Error(err)
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
			client, _ := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)

			mounts, error := client.GetSecretMounts(suite.ctx, tc.secretPaths)

			if tc.expectError {
				suite.ErrorContains(error, tc.errorMsg)
			} else {
				suite.ElementsMatch(mounts, tc.expectedMounts)
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

	suite.Run("GetKeysUnderMount without filtering", func() {
		for _, tc := range testCases {
			suite.Run(tc.name, func() {
				for secretPath, secretData := range tc.setupSecrets {
					_, err := suite.mainVault.WriteSecret(suite.ctx, tc.mount, secretPath, secretData)
					suite.NoError(err)
				}

				client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
				suite.NoError(err)

				keys, err := client.GetKeysUnderMount(suite.ctx, tc.mount, func(path string) bool {
					return true
				})

				if tc.expectError {
					suite.Error(err)
					suite.ErrorContains(err, tc.errorMsg)
				} else {
					suite.NoError(err)
					suite.ElementsMatch(tc.expectedKeys, keys)
				}
			})
		}
	})

	suite.Run("GetKeysUnderMount with filtering", func() {
		for keypath, data := range map[string]map[string]string{
			"production/infra/argocd":  {"key": "api-key-123"},
			"production/infra/grafana": {"env": "production"},
			"production/infra/thanos":  {"secret": "secret-key"},
			"stage/app/app1":           {"db": "db-password"},
			"stage/app/app2":           {"db": "db-password"},
		} {
			_, err := suite.mainVault.WriteSecret(suite.ctx, mounts[0], keypath, data)
			suite.NoError(err)
		}

		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err)

		keys, err := client.GetKeysUnderMount(suite.ctx, mounts[0], func(path string) bool {
			fmt.Println("Checking path:", path)
			return path != "production/infra/grafana" && path != "stage/app/app1"
		})

		suite.NoError(err)
		suite.ElementsMatch([]string{"production/infra/argocd", "production/infra/thanos", "stage/app/app2"}, keys)
	})
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

			client, err := NewMultiClusterVaultClient(ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err)

			metadata, err := client.GetSecretMetadata(ctx, tc.mount, tc.keyPath)

			if tc.expectError {
				suite.Error(err)
				suite.ErrorContains(err, tc.errorMsg)
			} else {
				suite.NoError(err)
				suite.NotNil(metadata)
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
			result := params.result
			fmt.Println("Assertion for Result for cluster:", cluster, "Status:", result.Status)
			suite.NotNil(result)

			data, version, _ := params.vaultHelper.ReadSecretData(suite.ctx, params.mount, params.keypath)

			suite.Equal(cluster, result.DestinationCluster)
			suite.Equal(params.mount, result.SecretBackend)
			suite.Equal(params.keypath, result.SecretPath)
			suite.Equal(params.expectedStatus, result.Status)

			if result.Status == models.StatusSuccess {
				suite.Equal(params.expectedSecret, data)
				suite.Equal(result.DestinationVersion, version)
				suite.NotNil(result.LastSyncAttempt)
				suite.Nil(result.ErrorMessage)
			} else {
				suite.NotNil(result.ErrorMessage)
				suite.Nil(result.LastSyncSuccess)
			}
		}
	}

	suite.Run("successful sync to all replicas new secret", func() {
		suite.mainVault.WriteSecret(suite.ctx, mount, keyPath, secret)
		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err)

		results, err := client.SyncSecretToReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err)
		suite.Len(results, 2)

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
		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, []*config.VaultClusterConfig{})
		suite.NoError(err)

		twoSeconds := 2 * time.Second
		suite.replica2Vault.Stop(suite.ctx, &twoSeconds) // Simulate replica 2 being unavailable
		results, err := client.SyncSecretToReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err)
		suite.Len(results, 0)
	})

	suite.Run("returns failed results when all replica become unavailable", func() {
		suite.mainVault.WriteSecret(suite.ctx, mount, keyPath, secret)
		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err)

		twoSeconds := 2 * time.Second
		suite.replica1Vault.Stop(suite.ctx, &twoSeconds)
		suite.replica2Vault.Stop(suite.ctx, &twoSeconds)
		results, err := client.SyncSecretToReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err)
		suite.Len(results, 2)

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
			client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err)

			results, err := client.SyncSecretToReplicas(suite.ctx, "", keyPath)

			suite.Error(err)
			suite.Nil(results)
			suite.ErrorContains(err, "mount cannot be empty")
		})

		suite.Run("for empty key path", func() {
			client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err)

			results, err := client.SyncSecretToReplicas(suite.ctx, mount, "")

			suite.Error(err)
			suite.Nil(results)
			suite.ErrorContains(err, "key path cannot be empty")
		})

		suite.Run("when it fails to read source secret", func() {
			client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err)

			results, err := client.SyncSecretToReplicas(suite.ctx, "wrong-mount", keyPath)

			suite.Error(err)
			suite.Nil(results)
			suite.ErrorContains(err, "failed to read secret")
		})
	})

}

func (suite *MultiClusterVaultClientTestSuite) TestDeleteSecretFromReplicas() {
	mount := "team-a"
	keyPath := "app/database"
	secret := map[string]string{
		"host":     "main-db.example.com",
		"username": "admin",
		"password": "secret123",
	}

	type deleteResultAssertion struct {
		vaultHelper    *testutil.VaultHelper
		result         *models.SyncSecretDeletionResult
		expectedStatus models.SyncStatus
		mount          string
		keypath        string
	}

	assertResult := func(assertionParams []*deleteResultAssertion) {
		for _, params := range assertionParams {
			cluster := params.vaultHelper.Config.ClusterName
			result := params.result
			fmt.Println("Assertion for Result for cluster:", cluster, "Status:", result.Status)
			suite.NotNil(result)

			_, _, err := params.vaultHelper.ReadSecretData(suite.ctx, params.mount, params.keypath)
			suite.Equal(cluster, result.DestinationCluster)
			suite.Equal(params.mount, result.SecretBackend)
			suite.Equal(params.keypath, result.SecretPath)
			suite.Equal(params.expectedStatus, result.Status)

			if result.Status == models.StatusSuccess {
				suite.Error(err)
				suite.ErrorContains(err, "no secret found")
				suite.Nil(result.ErrorMessage)
			} else {
				suite.NotNil(result.ErrorMessage)
			}
		}
	}

	suite.Run("successful deletion from all replicas", func() {
		suite.replica1Vault.WriteSecret(suite.ctx, mount, keyPath, secret)
		suite.replica2Vault.WriteSecret(suite.ctx, mount, keyPath, secret)

		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err)

		results, err := client.DeleteSecretFromReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err)
		suite.Len(results, 2)

		assertResult([]*deleteResultAssertion{
			{
				vaultHelper:    suite.replica1Vault,
				result:         results[0],
				expectedStatus: models.StatusSuccess,
				mount:          mount,
				keypath:        keyPath,
			},
			{
				vaultHelper:    suite.replica2Vault,
				result:         results[1],
				expectedStatus: models.StatusSuccess,
				mount:          mount,
				keypath:        keyPath,
			},
		})
	})

	suite.Run("successful deletion of non-existent secret (no error)", func() {
		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err)

		results, err := client.DeleteSecretFromReplicas(suite.ctx, mount, "non/existent/secret")

		suite.NoError(err)
		suite.Len(results, 2)

		assertResult([]*deleteResultAssertion{
			{
				vaultHelper:    suite.replica1Vault,
				result:         results[0],
				expectedStatus: models.StatusSuccess,
				mount:          mount,
				keypath:        "non/existent/secret",
			},
			{
				vaultHelper:    suite.replica2Vault,
				result:         results[1],
				expectedStatus: models.StatusSuccess,
				mount:          mount,
				keypath:        "non/existent/secret",
			},
		})
	})

	suite.Run("returns empty results if no replica is configured", func() {
		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, []*config.VaultClusterConfig{})
		suite.NoError(err)

		results, err := client.DeleteSecretFromReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err)
		suite.Len(results, 0)
	})

	suite.Run("returns failed results when all replicas become unavailable", func() {
		client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
		suite.NoError(err)

		twoSeconds := 2 * time.Second
		suite.replica1Vault.Stop(suite.ctx, &twoSeconds) // Simulate replica 1 being unavailable
		suite.replica2Vault.Stop(suite.ctx, &twoSeconds) // Simulate replica 2 being unavailable

		results, err := client.DeleteSecretFromReplicas(suite.ctx, mount, keyPath)

		suite.NoError(err)
		suite.Len(results, 2)

		assertResult([]*deleteResultAssertion{
			{
				vaultHelper:    suite.replica1Vault,
				result:         results[0],
				expectedStatus: models.StatusFailed,
				mount:          mount,
				keypath:        keyPath,
			},
			{
				vaultHelper:    suite.replica2Vault,
				result:         results[1],
				expectedStatus: models.StatusFailed,
				mount:          mount,
				keypath:        keyPath,
			},
		})
	})

	suite.Run("returns error", func() {
		suite.Run("for empty mount", func() {
			client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err)

			results, err := client.DeleteSecretFromReplicas(suite.ctx, "", keyPath)

			suite.Nil(results)
			suite.Error(err)
			suite.ErrorContains(err, "mount cannot be empty")
		})

		suite.Run("for empty key path", func() {
			client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err)

			results, err := client.DeleteSecretFromReplicas(suite.ctx, mount, "")

			suite.Nil(results)
			suite.Error(err)
			suite.ErrorContains(err, "key path cannot be empty")
		})

		suite.Run("partial failure when one replica is unavailable", func() {
			suite.replica1Vault.WriteSecret(suite.ctx, mount, keyPath, secret)
			client, err := NewMultiClusterVaultClient(suite.ctx, suite.mainConfig, suite.replicaConfig)
			suite.NoError(err)

			twoSeconds := 2 * time.Second
			suite.replica2Vault.Stop(suite.ctx, &twoSeconds)

			results, err := client.DeleteSecretFromReplicas(suite.ctx, mount, keyPath)

			suite.NoError(err)
			suite.Len(results, 2)

			assertResult([]*deleteResultAssertion{
				{
					vaultHelper:    suite.replica1Vault,
					result:         results[0],
					expectedStatus: models.StatusSuccess,
					mount:          mount,
					keypath:        keyPath,
				},
				{
					vaultHelper:    suite.replica2Vault,
					result:         results[1],
					expectedStatus: models.StatusFailed,
					mount:          mount,
					keypath:        keyPath,
				},
			})

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
