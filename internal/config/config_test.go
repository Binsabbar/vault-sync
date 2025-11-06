package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type invalidConfigTestTable struct {
	name        string
	setFields   configFields
	errContains string
}

type configFields map[string]interface{}

var validAppConfig = configFields{
	"id":          "test",
	"log_level":   "info",
	"concurrency": 5,

	"sync_rule.interval":           "60s",
	"sync_rule.kv_mounts":          []string{"kv"},
	"sync_rule.paths_to_replicate": []string{"secret/data/test", "secret/data/test2"},
	"sync_rule.paths_to_ignore":    []string{"secret/data/test3", "secret/data/test4"},

	"postgres.address":                   "localhost",
	"postgres.port":                      5432,
	"postgres.username":                  "u",
	"postgres.password":                  "p",
	"postgres.db_name":                   "d",
	"postgres.max_connection":            "10",
	"vault.main_cluster.name":            "main-cluster",
	"vault.main_cluster.address":         "http://vault:8200",
	"vault.main_cluster.app_role_id":     "r",
	"vault.main_cluster.app_role_secret": "s",
	"vault.main_cluster.app_role_mount":  "s",
	"vault.main_cluster.tls_skip_verify": "true",
	"vault.replica_clusters":             []configFields{validVaultReplicaClusterConfig},
}

var validVaultReplicaClusterConfig = configFields{
	"name":            "replica-1",
	"address":         "http://vault-replica-1:8200",
	"app_role_id":     "r",
	"app_role_secret": "s",
	"app_role_mount":  "s",
	"tls_skip_verify": "true",
}

func TestConfigLoadFromYAML(t *testing.T) {
	cleanupEnv(t)
	viper.Reset()
	viper.SetConfigFile(testConfigPath("config.yaml"))
	viper.SetConfigType("yaml")

	cfg, err := Load()

	require.NoError(t, err)

	require.Equal(t, "test", cfg.ID)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, 5, cfg.Concurrency)

	require.Equal(t, time.Duration(60*time.Second), cfg.SyncRule.GetInterval())
	require.Equal(t, []string{"secret", "secret2"}, cfg.SyncRule.KvMounts)
	require.ElementsMatch(t, []string{"secret/data/test", "secret/data/test2"}, cfg.SyncRule.PathsToReplicate)
	require.ElementsMatch(t, []string{"secret/data/test3", "secret/data/test4"}, cfg.SyncRule.PathsToIgnore)

	// Check Postgres configuration
	require.Equal(t, "localhost", cfg.Postgres.Address)
	require.Equal(t, 5432, cfg.Postgres.Port)
	require.Equal(t, "postgres", cfg.Postgres.Username)
	require.Equal(t, "vault_sync", cfg.Postgres.DBName)
	require.Equal(t, "disable", cfg.Postgres.SSLMode)
	require.Equal(t, "/path/to/root.crt", cfg.Postgres.SSLRootCertFile)

	require.Equal(t, 10, cfg.Postgres.MaxConnections)

	// Check Vault configuration main cluster
	require.Equal(t, "main-cluster", cfg.Vault.MainCluster.Name)
	require.Equal(t, "http://vault:8200", cfg.Vault.MainCluster.Address)
	require.True(t, cfg.Vault.MainCluster.TLSSkipVerify)
	require.Equal(t, "/path/to/cert.pem", cfg.Vault.MainCluster.TLSCertFile)
	require.Equal(t, "my_app_role", cfg.Vault.MainCluster.AppRoleID)
	require.Equal(t, "my_app_secret", cfg.Vault.MainCluster.AppRoleSecret)
	require.Equal(t, "approle", cfg.Vault.MainCluster.AppRoleMount)

	// Check Vault configuration replica clusters
	require.Len(t, cfg.Vault.ReplicaClusters, 2)

	replica2 := cfg.Vault.ReplicaClusters[0]
	require.Equal(t, "replica-2", replica2.Name)
	require.True(t, replica2.TLSSkipVerify)
	require.Equal(t, "/path/to/cert-2.pem", replica2.TLSCertFile)
	require.Equal(t, "http://vault-replica-2:8200", replica2.Address)
	require.Equal(t, "my_app_role_replica_2", replica2.AppRoleID)
	require.Equal(t, "my_app_secret_replica_2", replica2.AppRoleSecret)
	require.Equal(t, "approle2", replica2.AppRoleMount)

	replica3 := cfg.Vault.ReplicaClusters[1]
	require.Equal(t, "replica-3", replica3.Name)
	require.True(t, replica3.TLSSkipVerify)
	require.Equal(t, "/path/to/cert-3.pem", replica3.TLSCertFile)
	require.Equal(t, "http://vault-replica-3:8200", replica3.Address)
	require.Equal(t, "my_app_role_replica_3", replica3.AppRoleID)
	require.Equal(t, "my_app_secret_replica_3", replica3.AppRoleSecret)
	require.Equal(t, "approle3", replica3.AppRoleMount)
}

func TestConfigurationValidation(t *testing.T) {
	cleanupEnv(t)
	// for validationCheck use newConfig since no load from file is required.
	// the test object is built dynamically in the test.

	t.Run("returns config without error when config is valid", func(t *testing.T) {
		viper.Reset()
		viper.SetConfigFile(testConfigPath("config.yaml"))
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadInConfig())

		cfg, err := Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)
	})

	t.Run("Return error when no config loaded", func(t *testing.T) {
		viper.Reset()
		viper.SetConfigType("yaml")

		_, err := Load()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no config file found")
	})

	t.Run("It fails when validation fails", func(t *testing.T) {
		tests := []invalidConfigTestTable{
			// root level
			{
				name:        "missing id",
				setFields:   deleteFromMap(validAppConfig, "id"),
				errContains: "Config.ID is required",
			},
			{
				name:        "invalid log_level value",
				setFields:   updateAndReturnMap(validAppConfig, "log_level", "invalid"),
				errContains: "Config.LogLevel must be one of [trace debug info warn error fatal panic]",
			},

			{
				name:        "invalid concurrency value",
				setFields:   updateAndReturnMap(validAppConfig, "concurrency", -1),
				errContains: "Config.Concurrency must be greater than 0",
			},

			{
				name:        "max concurrency value",
				setFields:   updateAndReturnMap(validAppConfig, "concurrency", 101),
				errContains: "Config.Concurrency must be less than 101",
			},

			{
				name:        "invalid concurrency value",
				setFields:   updateAndReturnMap(validAppConfig, "concurrency", -1),
				errContains: "Config.Concurrency must be greater than 0",
			},

			// sync rule level
			{
				name:        "missing interval",
				setFields:   deleteFromMap(validAppConfig, "sync_rule.interval"),
				errContains: "Config.SyncRule.Interval is required",
			},
			{
				name:        "interval not string",
				setFields:   updateAndReturnMap(validAppConfig, "sync_rule.interval", 123),
				errContains: "Config.SyncRule.Interval must match the format of a valid duration (e.g., 1s, 5m, 2h)",
			},
			{
				name:        "interval not valid duration",
				setFields:   updateAndReturnMap(validAppConfig, "sync_rule.interval", "invalid"),
				errContains: "Config.SyncRule.Interval must match the format of a valid duration (e.g., 1s, 5m, 2h)",
			},
			{
				name:        "interval is less than 60s",
				setFields:   updateAndReturnMap(validAppConfig, "sync_rule.interval", "59s"),
				errContains: "Config.SyncRule.Interval must be greater than or equal to 60s",
			},
			{
				name:        "interval is greater than 24h",
				setFields:   updateAndReturnMap(validAppConfig, "sync_rule.interval", "25h"),
				errContains: "Config.SyncRule.Interval must be less than or equal to 24h",
			},
			{
				name:        "kv_mounts is not present",
				setFields:   deleteFromMap(validAppConfig, "sync_rule.kv_mounts"),
				errContains: "Config.SyncRule.KvMounts is required",
			},
			{
				name:        "kv_mounts is empty",
				setFields:   updateAndReturnMap(validAppConfig, "sync_rule.kv_mounts", []string{}),
				errContains: "Config.SyncRule.KvMounts must have at least 1 items",
			},
			{
				name: "kv_mounts contains duplicated items",
				setFields: updateAndReturnMap(
					validAppConfig,
					"sync_rule.kv_mounts",
					[]string{"secret", "secret", "secret2"},
				),
				errContains: "Config.SyncRule.KvMounts must contain unique items",
			},
			{
				name: "duplicated paths in sync_rule.paths_to_replicate",
				setFields: updateAndReturnMap(
					validAppConfig,
					"sync_rule.paths_to_replicate",
					[]string{"secret/data/test", "secret/data/test", "secret/data/test2"},
				),
				errContains: "Config.SyncRule.PathsToReplicate must contain unique items",
			},
			{
				name: "duplicated paths in sync_rule.paths_to_ignore",
				setFields: updateAndReturnMap(
					validAppConfig,
					"sync_rule.paths_to_ignore",
					[]string{"secret/data/test", "secret/data/test", "secret/data/test2"},
				),
				errContains: "Config.SyncRule.PathsToIgnore must contain unique items",
			},
			{
				name: "mautual execlusive paths in sync_rule.paths_to_replicate and sync_rule.paths_to_ignore",
				setFields: updateAndReturnMap(
					updateAndReturnMap(
						validAppConfig,
						"sync_rule.paths_to_replicate",
						[]string{"secret/data/test1", "secret/data/test3"},
					),
					"sync_rule.paths_to_ignore",
					[]string{"secret/data/test4", "secret/data/test3"},
				),
				errContains: "Config.SyncRule.PathsToIgnore must not contain items that are also in PathsToReplicate, Config.SyncRule.PathsToReplicate must not contain items that are also in PathsToReplicate",
			},

			// postgres level
			{
				name:        "missing postgres.address",
				setFields:   deleteFromMap(validAppConfig, "postgres.address"),
				errContains: "Config.Postgres.Address is required",
			},
			{
				name:        "invalid postgres.address",
				setFields:   updateAndReturnMap(validAppConfig, "postgres.address", "sfg://a"),
				errContains: "Config.Postgres.Address must be a valid hostname or IP address",
			},
			{
				name:        "missing postgres.port",
				setFields:   deleteFromMap(validAppConfig, "postgres.port"),
				errContains: "Config.Postgres.Port is required",
			},
			{
				name:        "invalid postgres.port greater than 65536",
				setFields:   updateAndReturnMap(validAppConfig, "postgres.port", 70000),
				errContains: "Config.Postgres.Port must be less than 65536",
			},
			{
				name:        "invalid postgres.port less than 0",
				setFields:   updateAndReturnMap(validAppConfig, "postgres.port", -1),
				errContains: "Config.Postgres.Port must be greater than 0",
			},
			{
				name:        "invalid postgres.port",
				setFields:   updateAndReturnMap(validAppConfig, "postgres.port", "a"),
				errContains: "'postgres.port' cannot parse value as 'int'",
			},
			{
				name:        "missing postgres.username",
				setFields:   deleteFromMap(validAppConfig, "postgres.username"),
				errContains: "Config.Postgres.Username is required",
			},
			{
				name:        "missing postgres.password",
				setFields:   deleteFromMap(validAppConfig, "postgres.password"),
				errContains: "Config.Postgres.Password is required",
			},
			{
				name:        "missing postgres.db_name",
				setFields:   deleteFromMap(validAppConfig, "postgres.db_name"),
				errContains: "Config.Postgres.DBName is required",
			},
			{
				name:        "invalid postgres.ssl_mode",
				setFields:   updateAndReturnMap(validAppConfig, "postgres.ssl_mode", "invalid"),
				errContains: "Config.Postgres.SSLMode must be one of [disable allow prefer require verify-ca verify-full]",
			},
			{
				name:        "invalid postgres.ssl_root_cert_file",
				setFields:   updateAndReturnMap(validAppConfig, "postgres.ssl_root_cert_file", "invalid+/"),
				errContains: "Config.Postgres.SSLRootCertFile must be a valid file path",
			},

			// vault level
			{
				name:        "missing vault.main_cluster.name",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.name"),
				errContains: "Config.Vault.MainCluster.Name is required",
			},
			{
				name:        "missing vault.main_cluster.address",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.address"),
				errContains: "Config.Vault.MainCluster.Address is required",
			},
			{
				name:        "invalid vault.main_cluster.address",
				setFields:   updateAndReturnMap(validAppConfig, "vault.main_cluster.address", "invalid"),
				errContains: "Config.Vault.MainCluster.Address must be a valid URL",
			},
			{
				name:        "missing vault.main_cluster.app_role_id",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.app_role_id"),
				errContains: "Config.Vault.MainCluster.AppRoleID is required",
			},
			{
				name:        "missing vault.main_cluster.app_role_secret",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.app_role_secret"),
				errContains: "Config.Vault.MainCluster.AppRoleSecret is required",
			},
			{
				name:        "invalid vault.main_cluster.tls_skip_verify",
				setFields:   updateAndReturnMap(validAppConfig, "vault.main_cluster.tls_skip_verify", "invalid"),
				errContains: "vault.main_cluster.tls_skip_verify' cannot parse value as 'bool'",
			},
			{
				name: "invalid vault.main_cluster.tls_cert_file",
				setFields: updateAndReturnMap(
					updateAndReturnMap(validAppConfig, "vault.main_cluster.tls_cert_file", "invalid+/"),
					"vault.main_cluster.tls_skip_verify",
					"false",
				),
				errContains: "Config.Vault.MainCluster.TLSCertFile must be a valid file path",
			},
			{
				name:        "replica_clusters must not be empty",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", []configFields{}),
				errContains: "Config.Vault.ReplicaClusters must have at least 1 items/characters",
			},
			{
				name: "missing vault.replica_cluster.name",
				setFields: updateAndReturnMap(
					validAppConfig,
					"vault.replica_clusters",
					deleteFromMap(validVaultReplicaClusterConfig, "name"),
				),
				errContains: "Config.Vault.ReplicaClusters[0].Name is required",
			},
			{
				name: "missing vault.replica_cluster.address	",
				setFields: updateAndReturnMap(
					validAppConfig,
					"vault.replica_clusters",
					deleteFromMap(validVaultReplicaClusterConfig, "address"),
				),
				errContains: "Config.Vault.ReplicaClusters[0].Address is required",
			},
			{
				name: "invalid vault.replica_cluster.address	",
				setFields: updateAndReturnMap(
					validAppConfig,
					"vault.replica_clusters",
					updateAndReturnMap(validVaultReplicaClusterConfig, "address", "invalid"),
				),
				errContains: "Config.Vault.ReplicaClusters[0].Address must be a valid URL",
			},
			{
				name: "missing vault.replica_cluster.app_role_id",
				setFields: updateAndReturnMap(
					validAppConfig,
					"vault.replica_clusters",
					deleteFromMap(validVaultReplicaClusterConfig, "app_role_id"),
				),
				errContains: "Config.Vault.ReplicaClusters[0].AppRoleID is required",
			},
			{
				name: "missing vault.replica_cluster.app_role_secret",
				setFields: updateAndReturnMap(
					validAppConfig,
					"vault.replica_clusters",
					deleteFromMap(validVaultReplicaClusterConfig, "app_role_secret"),
				),
				errContains: "Config.Vault.ReplicaClusters[0].AppRoleSecret is required",
			},
			{
				name: "invalid vault.replica_cluster.tls_cert_file",
				setFields: updateAndReturnMap(
					validAppConfig,
					"vault.replica_clusters",
					updateAndReturnMap(validVaultReplicaClusterConfig, "tls_cert_file", "invalid+/"),
				),
				errContains: "Config.Vault.ReplicaClusters[0].TLSCertFile must be a valid file path",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				viper.Reset()
				for k, v := range tt.setFields {
					viper.Set(k, v)
				}

				_, err := newConfig()

				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
			})
		}
	})

	t.Run("It sets default values for optional params", func(t *testing.T) {
		viper.Reset()
		config := deleteFromMap(
			updateAndReturnMap(
				validAppConfig,
				"vault.replica_clusters",
				deleteFromMap(validVaultReplicaClusterConfig, "app_role_mount", "tls_skip_verify"),
			),
			"log_level",
			"postgres.ssl_mode",
			"vault.main_cluster.app_role_mount",
			"vault.main_cluster.tls_skip_verify",
		)
		for k, v := range config {
			viper.Set(k, v)
		}

		cfg, _ := newConfig()

		assert.Equal(t, "info", cfg.LogLevel, "Default value for log_level should be 'info'")
		assert.Equal(t, "disable", cfg.Postgres.SSLMode, "Default value for postgres.ssl_mode should be 'disable'")
		assert.Equal(
			t,
			"approle",
			cfg.Vault.MainCluster.AppRoleMount,
			"Default value for vault.main_cluster.app_role_mount should be 'approle'",
		)
		assert.Equal(
			t,
			false,
			cfg.Vault.MainCluster.TLSSkipVerify,
			"Default value for vault.main_cluster.tls_skip_verify should be 'false'",
		)
		assert.Equal(
			t,
			"approle",
			cfg.Vault.ReplicaClusters[0].AppRoleMount,
			"Default value for vault.replica_clusters[0].app_role_mount should be 'approle'",
		)
		assert.Equal(
			t,
			false,
			cfg.Vault.ReplicaClusters[0].TLSSkipVerify,
			"Default value for vault.replica_clusters[0].tls_skip_verify should be 'false'",
		)

	})
}

func TestConfigEnvironmentVariableSubstitution(t *testing.T) {
	cleanupEnv(t)
	t.Run("substitutes environment variables using ${VAR} syntax", func(t *testing.T) {
		// Set environment variables
		t.Setenv("TEST_ID", "env_id_123")
		t.Setenv("TEST_POSTGRES_PASSWORD", "secret_password")
		t.Setenv("TEST_MAIN_APP_ROLE_ID", "main-role-id-123")
		t.Setenv("TEST_MAIN_APP_ROLE_SECRET", "main-role-secret-456")
		t.Setenv("TEST_REPLICA_0_APP_ROLE_ID", "replica-0-role-id-789")
		t.Setenv("TEST_REPLICA_0_APP_ROLE_SECRET", "replica-0-role-secret-abc")

		// Reset viper and load config with env var substitution
		viper.Reset()
		viper.SetConfigFile(testConfigPath("config-with-env-vars.yaml"))
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadInConfig())

		cfg, err := Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Verify substituted values
		assert.Equal(t, "env_id_123", cfg.ID)
		assert.Equal(t, "secret_password", cfg.Postgres.Password)
		assert.Equal(t, "main-role-id-123", cfg.Vault.MainCluster.AppRoleID)
		assert.Equal(t, "main-role-secret-456", cfg.Vault.MainCluster.AppRoleSecret)
		assert.Equal(t, "replica-0-role-id-789", cfg.Vault.ReplicaClusters[0].AppRoleID)
		assert.Equal(t, "replica-0-role-secret-abc", cfg.Vault.ReplicaClusters[0].AppRoleSecret)
	})

	t.Run("raises an error when environment variable is not set if required var", func(t *testing.T) {
		viper.Reset()
		viper.SetConfigFile(testConfigPath("config-with-env-vars.yaml"))
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadInConfig())

		_, err := Load()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is required")
	})
}

func TestConfigViperAutomaticEnvOverride(t *testing.T) {
	cleanupEnv(t)
	t.Run("overrides config values with VAULT_SYNC_ prefixed env vars", func(t *testing.T) {
		t.Setenv("VAULT_SYNC_ID", "overridden-id")
		t.Setenv("VAULT_SYNC_LOG_LEVEL", "debug")
		t.Setenv("VAULT_SYNC_CONCURRENCY", "10")
		t.Setenv("VAULT_SYNC_POSTGRES_PASSWORD", "overridden-password")
		t.Setenv("VAULT_SYNC_POSTGRES_USERNAME", "admin")
		t.Setenv("VAULT_SYNC_VAULT_MAIN_CLUSTER_NAME", "production-vault")
		t.Setenv("VAULT_SYNC_VAULT_MAIN_CLUSTER_ADDRESS", "https://vault-prod.example.com")

		viper.Reset()
		viper.SetConfigFile(testConfigPath("config.yaml"))
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadInConfig())

		cfg, err := Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Verify overridden values
		assert.Equal(t, "overridden-id", cfg.ID)
		assert.Equal(t, "debug", cfg.LogLevel)
		assert.Equal(t, 10, cfg.Concurrency)
		assert.Equal(t, "overridden-password", cfg.Postgres.Password)
		assert.Equal(t, "admin", cfg.Postgres.Username)
		assert.Equal(t, "production-vault", cfg.Vault.MainCluster.Name)
		assert.Equal(t, "https://vault-prod.example.com", cfg.Vault.MainCluster.Address)
	})

	t.Run("env substitution takes precedence over viper automatic env", func(t *testing.T) {
		// Set both styles of env vars
		t.Setenv("TEST_POSTGRES_PASSWORD", "substituted-password")
		t.Setenv("VAULT_SYNC_POSTGRES_PASSWORD", "viper-password")

		viper.Reset()
		viper.SetConfigFile(testConfigPath("config-precedence.yaml"))
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadInConfig())

		cfg, err := Load()

		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, "viper-password", cfg.Postgres.Password)
	})

	t.Run("validates overridden values", func(t *testing.T) {
		t.Setenv("VAULT_SYNC_LOG_LEVEL", "invalid-level")
		t.Setenv("VAULT_SYNC_POSTGRES_PASSWORD", "test-pass")

		viper.Reset()
		viper.SetConfigFile(testConfigPath("config.yaml"))
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadInConfig())

		_, err := Load()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Config.LogLevel must be one of [trace debug info warn error fatal panic]")
	})
}

func deleteFromMap(m configFields, keys ...string) configFields {
	clonedMap := maps.Clone(m)
	for _, argument := range keys {
		delete(clonedMap, argument)
	}
	return clonedMap
}

func updateAndReturnMap(m configFields, key string, value interface{}) configFields {
	clonedMap := maps.Clone(m)
	clonedMap[key] = value
	return clonedMap
}

func getTestdataPath() string {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	return filepath.Join(dir, "testdata")
}

// testConfigPath returns the path to a test config file
// Panics if the file doesn't exist (fail fast in tests)
func testConfigPath(filename string) string {
	path := filepath.Join(getTestdataPath(), filename)

	// Check if file exists
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		panic(fmt.Sprintf("test config file does not exist: %s", path))
	}

	return path
}

// cleanupEnv removes all test-related environment variables before a test runs
// This prevents issues with stale env vars from previous terminal sessions
func cleanupEnv(t *testing.T) {
	t.Helper()

	prefixes := []string{
		"VAULT_SYNC_",
		"TEST_",
	}

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key := pair[0]

		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				t.Setenv(key, "")
				break
			}
		}
	}
}
