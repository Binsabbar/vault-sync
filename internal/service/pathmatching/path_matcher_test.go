package pathmatching

import (
	"context"
	"testing"
	"vault-sync/internal/config"
	"vault-sync/internal/vault"
	"vault-sync/testutil"

	"github.com/stretchr/testify/suite"
)

type PathMatcherTestSuite struct {
	suite.Suite
	ctx          context.Context
	vaultCluster *testutil.VaultHelper
	vaultClient  vault.Syncer
	mounts       []string
}

type mountPath struct {
	mount string
	paths []string
}

func (suite *PathMatcherTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.mounts = []string{teamAMount, teamBMount, prodMount}

	vaultCluster, err := testutil.NewVaultClusterContainer(suite.ctx, "main")
	suite.Require().NoError(err, "Failed to create vault clusters")

	suite.vaultCluster = vaultCluster
	mainConfig, _ := testutil.SetupExistingClusters(
		suite.vaultCluster,
		nil, // No replicas needed for path matcher tests
		nil, // No replicas needed for path matcher tests
		suite.mounts...,
	)

	vaultClient, err := vault.NewMultiClusterVaultClient(
		suite.ctx,
		mainConfig,
		make([]*config.VaultClusterConfig, 0), // No replicas needed for path matcher tests
	)

	suite.Require().NoError(err, "Failed to create vault client")
	suite.vaultClient = vaultClient
}

func (suite *PathMatcherTestSuite) TearDownSuite() {
	testutil.TerminateAllClusters(suite.vaultCluster, nil, nil)
}

func (suite *PathMatcherTestSuite) SetupTest() {
	testutil.TruncateSecrets(suite.vaultCluster, nil, nil, suite.mounts...)
}

func TestPathMatcherTestSuite(t *testing.T) {
	suite.Run(t, new(PathMatcherTestSuite))
}

func (suite *PathMatcherTestSuite) TestDiscoverSecretsForSync() {

	type testCase struct {
		syncRules        *config.SyncRule
		expectedLen      int
		expectedContains []mountPath
	}
	type testSpec struct {
		name string
		skip bool
		test testCase
	}

	specs := []testSpec{
		{
			name: "returns all secrets when no patterns specified",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount},
					PathsToReplicate: []string{},
					PathsToIgnore:    []string{},
				},
				expectedLen: 6,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database, keyPathConfigsApp, keyPathTempCache},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI, keyPathConfigsWeb, keyPathConfigsAppTest},
					},
				},
			},
		},

		{
			name: "filters by single wildcard pattern - secrets/*",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 2,
				expectedContains: []mountPath{
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI},
					},
					{
						mount: prodMount,
						paths: []string{keyPathSecretsMain},
					},
				},
			},
		},

		{
			name: "filters by two-level wildcard pattern - secrets/*/*",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/*/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 1,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database},
					},
				},
			},
		},

		{
			name: "filters by three-level wildcard pattern - secrets/*/*/*",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/*/*/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 2,
				expectedContains: []mountPath{
					{
						mount: prodMount,
						paths: []string{keyPathSecretsApp1ConfigDB, keyPathSecretsApp1InfraDB},
					},
				},
			},
		},

		{
			name: "filters by recursive wildcard - secrets/**",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/**"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 5,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI},
					},
					{
						mount: prodMount,
						paths: []string{keyPathSecretsMain, keyPathSecretsApp1ConfigDB, keyPathSecretsApp1InfraDB},
					},
				},
			},
		},

		{
			name: "filters by multiple replicate patterns",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/*", "configs/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 4,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathConfigsApp},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI, keyPathConfigsWeb},
					},
					{
						mount: prodMount,
						paths: []string{keyPathSecretsMain},
					},
				},
			},
		},

		{
			name: "filters by character class pattern - project-[a-z]/*/*",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, prodMount},
					PathsToReplicate: []string{"project-[a-z]/*/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 2,
				expectedContains: []mountPath{
					{
						mount: prodMount,
						paths: []string{keyPathProjAApp2DB, keyPathProjBApp2DB},
					},
				},
			},
		},

		{
			name: "applies ignore patterns only",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{},
					PathsToIgnore:    []string{"project-[a-z]/*/*", "temp/*", "configs/web"},
				},
				expectedLen: 7,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database, keyPathConfigsApp},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI, keyPathConfigsAppTest},
					},
					{
						mount: prodMount,
						paths: []string{keyPathSecretsMain, keyPathSecretsApp1ConfigDB, keyPathSecretsApp1InfraDB},
					},
				},
			},
		},

		{
			name: "applies ignore with wildcard patterns - temp/**",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount},
					PathsToReplicate: []string{},
					PathsToIgnore:    []string{"temp/**"},
				},
				expectedLen: 5,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database, keyPathConfigsApp},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI, keyPathConfigsWeb, keyPathConfigsAppTest},
					},
				},
			},
		},

		{
			name: "applies both replicate and ignore with ignore precedence",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/*/*", "configs/app/*", "configs/app"},
					PathsToIgnore:    []string{"project-[a-z]/*/*", "temp/*", "configs/web"},
				},
				expectedLen: 3,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database, keyPathConfigsApp},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathConfigsAppTest},
					},
				},
			},
		},

		{
			name: "applies replicate pattern with ignore blocking all matches",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount},
					PathsToReplicate: []string{"secrets/**"},
					PathsToIgnore:    []string{"secrets/**"},
				},
				expectedLen:      0,
				expectedContains: []mountPath{},
			},
		},

		{
			name: "filters by specific path depth - */app*/*",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"*/app*/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 4,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathConfigsAppTest},
					},
					{
						mount: prodMount,
						paths: []string{keyPathProjAApp2DB, keyPathProjBApp2DB},
					},
				},
			},
		},

		{
			name: "filters by ending pattern - **/database",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"**/database"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 5,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathSecretsApp1Database},
					},
					{
						mount: prodMount,
						paths: []string{
							keyPathSecretsApp1ConfigDB, keyPathProjAApp2DB,
							keyPathProjBApp2DB, keyPathSecretsApp1InfraDB,
						},
					},
				},
			},
		},

		{
			name: "filters by multiple specific patterns",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/api", "configs/app", "project-a/*/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 3,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathConfigsApp},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI},
					},
					{
						mount: prodMount,
						paths: []string{keyPathProjAApp2DB},
					},
				},
			},
		},

		{
			name: "ignores specific nested paths with wildcards",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{},
					PathsToIgnore:    []string{"**/app1/**", "configs/web"},
				},
				expectedLen: 7,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathConfigsApp, keyPathTempCache},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI, keyPathConfigsAppTest},
					},
					{
						mount: prodMount,
						paths: []string{keyPathSecretsMain, keyPathProjAApp2DB, keyPathProjBApp2DB},
					},
				},
			},
		},

		{
			name: "filters by configs with wildcard variations",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount},
					PathsToReplicate: []string{"configs/**"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 3,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathConfigsApp},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathConfigsWeb, keyPathConfigsAppTest},
					},
				},
			},
		},

		{
			name: "handles single mount with all patterns",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{prodMount},
					PathsToReplicate: []string{"secrets/**", "project-*/**"},
					PathsToIgnore:    []string{"**/infra/*"},
				},
				expectedLen: 4,
				expectedContains: []mountPath{
					{
						mount: prodMount,
						paths: []string{keyPathSecretsMain, keyPathSecretsApp1ConfigDB, keyPathProjAApp2DB, keyPathProjBApp2DB},
					},
				},
			},
		},

		{
			name: "ignores everything in specific mount prefix",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"**"},
					PathsToIgnore:    []string{"secrets/**"},
				},
				expectedLen: 6,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathConfigsApp, keyPathTempCache},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathConfigsWeb, keyPathConfigsAppTest},
					},
					{
						mount: prodMount,
						paths: []string{keyPathProjAApp2DB, keyPathProjBApp2DB},
					},
				},
			},
		},

		{
			name: "filters by exact path match",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount, prodMount},
					PathsToReplicate: []string{"secrets/api", "secrets/main", "temp/cache"},
					PathsToIgnore:    []string{},
				},
				expectedLen: 3,
				expectedContains: []mountPath{
					{
						mount: teamAMount,
						paths: []string{keyPathTempCache},
					},
					{
						mount: teamBMount,
						paths: []string{keyPathSecretsAPI},
					},
					{
						mount: prodMount,
						paths: []string{keyPathSecretsMain},
					},
				},
			},
		},

		{
			name: "returns empty when patterns match nothing",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount, teamBMount},
					PathsToReplicate: []string{"nonexistent/*"},
					PathsToIgnore:    []string{},
				},
				expectedLen:      0,
				expectedContains: []mountPath{},
			},
		},

		{
			name: "ignores all when ignore pattern matches everything",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{teamAMount},
					PathsToReplicate: []string{},
					PathsToIgnore:    []string{"**"},
				},
				expectedLen:      0,
				expectedContains: []mountPath{},
			},
		},
		{
			name: "ignores all when mount is not in KvMounts",
			skip: false,
			test: testCase{
				syncRules: &config.SyncRule{
					KvMounts:         []string{"new-mount"},
					PathsToReplicate: []string{},
					PathsToIgnore:    []string{"**"},
				},
				expectedLen:      0,
				expectedContains: []mountPath{},
			},
		},
	}

	for _, tc := range specs {
		suite.Run(tc.name, func() {
			if tc.skip {
				suite.T().Skip()
				return
			}
			suite.createTestSecrets()
			pathMatcher := NewVaultPathMatcher(suite.vaultClient, tc.test.syncRules)
			secrets, err := pathMatcher.DiscoverSecretsForSync(suite.ctx)
			suite.NoError(err)

			suite.Len(secrets, tc.test.expectedLen)
			for _, check := range tc.test.expectedContains {
				for _, path := range check.paths {
					suite.containsSecret(secrets, check.mount, path)
				}
			}
		})
	}
}

func (suite *PathMatcherTestSuite) TestDiscoverFromMounts() {
	suite.Run("discovers all secrets from specified mounts", func() {
		suite.createTestSecrets()

		pathMatcher := NewVaultPathMatcher(suite.vaultClient, &config.SyncRule{})

		secrets, err := pathMatcher.DiscoverFromMounts(suite.ctx, []string{teamAMount, prodMount})

		suite.NoError(err)
		suite.Len(secrets, 8)

		suite.containsSecret(secrets, teamAMount, keyPathSecretsApp1Database)
		suite.containsSecret(secrets, teamAMount, keyPathConfigsApp)
		suite.containsSecret(secrets, teamAMount, keyPathTempCache)

		suite.containsSecret(secrets, prodMount, keyPathSecretsMain)
		suite.containsSecret(secrets, prodMount, keyPathSecretsApp1ConfigDB)
		suite.containsSecret(secrets, prodMount, keyPathSecretsApp1InfraDB)
		suite.containsSecret(secrets, prodMount, keyPathProjAApp2DB)
		suite.containsSecret(secrets, prodMount, keyPathProjBApp2DB)

	})

	suite.Run("handles empty mounts gracefully", func() {
		pathMatcher := NewVaultPathMatcher(suite.vaultClient, &config.SyncRule{})

		secrets, err := pathMatcher.DiscoverFromMounts(suite.ctx, []string{})

		suite.NoError(err)
		suite.Len(secrets, 0)
	})

	suite.Run("handles non-existent mounts gracefully", func() {
		pathMatcher := NewVaultPathMatcher(suite.vaultClient, &config.SyncRule{})

		secrets, err := pathMatcher.DiscoverFromMounts(suite.ctx, []string{"non-existent-mount"})

		suite.NoError(err)
		suite.Len(secrets, 0) // Should handle gracefully
	})
}

func (suite *PathMatcherTestSuite) TestShouldSync() {
	suite.Run("skipped", func() {
		suite.T().Skip("whole implementation of ShouldSync is tested by TestDiscoverSecretsForSync")
	})

	suite.Run("no mounts allowed - always false", func() {
		pathMatcher := NewVaultPathMatcher(suite.vaultClient, &config.SyncRule{
			KvMounts:         []string{teamAMount},
			PathsToReplicate: []string{},
			PathsToIgnore:    []string{},
		})

		suite.False(pathMatcher.ShouldSync(prodMount, keyPathSecretsApp1Database))
	})
}

// Helper methods

type TestSecret struct {
	Key    string
	Values map[string]string
}

type TestSecretMount struct {
	Mount   string
	Secrets []TestSecret
}

const (
	teamAMount = "team-a"
	teamBMount = "team-b"
	prodMount  = "production"

	// team a
	keyPathSecretsApp1Database = "secrets/app1/database"
	keyPathConfigsApp          = "configs/app"
	keyPathTempCache           = "temp/cache"

	// team b
	keyPathSecretsAPI     = "secrets/api"
	keyPathConfigsWeb     = "configs/web"
	keyPathConfigsAppTest = "configs/app/test"

	// production
	keyPathSecretsMain         = "secrets/main"
	keyPathSecretsApp1ConfigDB = "secrets/app1/config/database"
	keyPathSecretsApp1InfraDB  = "secrets/app1/infra/database"
	keyPathProjAApp2DB         = "project-a/app2/database"
	keyPathProjBApp2DB         = "project-b/app2/database"
)

func (suite *PathMatcherTestSuite) createTestSecrets() {
	ctx := suite.ctx
	var testSecrets = []TestSecretMount{
		{
			Mount: teamAMount,
			Secrets: []TestSecret{
				{
					Key: keyPathSecretsApp1Database,
					Values: map[string]string{
						"host":     "db.team-a.local",
						"username": "admin",
						"password": "secret123",
					},
				},
				{
					Key: keyPathConfigsApp,
					Values: map[string]string{
						"debug":     "true",
						"log_level": "info",
					},
				},
				{
					Key: keyPathTempCache,
					Values: map[string]string{
						"redis_host": "localhost",
						"ttl":        "300",
					},
				},
			},
		},
		{
			Mount: teamBMount,
			Secrets: []TestSecret{
				{
					Key: keyPathSecretsAPI,
					Values: map[string]string{
						"api_key":    "abc123",
						"api_secret": "xyz789",
					},
				},
				{
					Key: keyPathConfigsWeb,
					Values: map[string]string{
						"port":    "8080",
						"timeout": "30s",
					},
				},
				{
					Key: keyPathConfigsAppTest,
					Values: map[string]string{
						"port":    "8080",
						"timeout": "30s",
					},
				},
			},
		},
		{
			Mount: prodMount,
			Secrets: []TestSecret{
				{
					Key: keyPathSecretsMain,
					Values: map[string]string{
						"master_key": "prod-key-123",
						"env":        "production",
					},
				},
				{
					Key: keyPathSecretsApp1ConfigDB,
					Values: map[string]string{
						"master_key": "prod-key-123",
						"env":        "production",
					},
				},
				{
					Key: keyPathSecretsApp1InfraDB,
					Values: map[string]string{
						"master_key": "prod-key-123",
						"env":        "production",
					},
				},
				{
					Key: keyPathProjAApp2DB,
					Values: map[string]string{
						"master_key": "prod-key-123",
						"env":        "production",
					},
				},
				{
					Key: keyPathProjBApp2DB,
					Values: map[string]string{
						"master_key": "prod-key-123",
						"env":        "production",
					},
				},
			},
		},
	}

	for _, item := range testSecrets {
		for _, secret := range item.Secrets {
			_, err := suite.vaultCluster.WriteSecret(ctx, item.Mount, secret.Key, secret.Values)
			suite.Require().NoError(err)
		}
	}
}

func (suite *PathMatcherTestSuite) containsSecret(secrets []SecretPath, mount, keyPath string) {
	expected := SecretPath{Mount: mount, KeyPath: keyPath}
	suite.Contains(secrets, expected, "should contain secret %s/%s", mount, keyPath)
}
