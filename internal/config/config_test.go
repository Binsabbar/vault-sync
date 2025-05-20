package config

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

type configTestTable struct {
	name        string
	setFields   configFields
	errContains string
}

type configFields map[string]interface{}

var validAppConfig = configFields{
	"id":                                 "test",
	"interval":                           5,
	"postgres.address":                   "localhost",
	"postgres.port":                      5432,
	"postgres.username":                  "u",
	"postgres.password":                  "p",
	"postgres.db_name":                   "d",
	"postgres.max_connection":            "10",
	"vault.main_cluster.address":         "a",
	"vault.main_cluster.app_role":        "r",
	"vault.main_cluster.app_secret":      "s",
	"vault.main_cluster.tls_skip_verify": true,
	"vault.replica_clusters":             []configFields{validVaultReplicaClusterConfig},
}

var validVaultReplicaClusterConfig = configFields{
	"name":            "r2",
	"address":         "a",
	"app_role":        "r",
	"app_secret":      "s",
	"tls_skip_verify": "true",
	"tls_cert_file":   "",
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

func TestConfigLoadFromYAML(t *testing.T) {
	viper.Reset()
	viper.SetConfigFile(filepath.Join("testdata", "config.yaml"))
	viper.SetConfigType("yaml")
	viper.ReadInConfig()

	cfg, err := NewConfig()

	require.NoError(t, err)

	require.Equal(t, "test", cfg.ID)
	require.Equal(t, 5, cfg.Interval)

	// Check Postgres configuration
	require.Equal(t, "localhost", cfg.Postgres.Address)
	require.Equal(t, 5432, cfg.Postgres.Port)
	require.Equal(t, "postgres", cfg.Postgres.Username)
	require.Equal(t, "vault_sync", cfg.Postgres.DBName)
	require.Equal(t, "disable", cfg.Postgres.SSLMode)
	require.Equal(t, 10, cfg.Postgres.MaxConnections)

	// Check Vault configuration main cluster
	require.Equal(t, "http://vault:8200", cfg.Vault.MainCluster.Address)
	require.True(t, cfg.Vault.MainCluster.TLSSkipVerify)
	require.Equal(t, "my_app_role", cfg.Vault.MainCluster.AppRole)
	require.Equal(t, "my_app_secret", cfg.Vault.MainCluster.AppSecret)
	require.ElementsMatch(t, []string{"secret/data/test", "secret/data/test2"}, cfg.Vault.MainCluster.PathsToReplicate)
	require.ElementsMatch(t, []string{"secret/data/test3", "secret/data/test4"}, cfg.Vault.MainCluster.PathsToIgnore)

	// Check Vault configuration replica clusters
	require.Len(t, cfg.Vault.ReplicaClusters, 2)

	replica2 := cfg.Vault.ReplicaClusters[0]
	require.Equal(t, "replica-2", replica2.Name)
	require.Equal(t, "http://vault-replica-2:8200", replica2.Address)
	require.Equal(t, "my_app_role_replica_2", replica2.AppRole)
	require.Equal(t, "my_app_secret_replica_2", replica2.AppSecret)

	replica3 := cfg.Vault.ReplicaClusters[1]
	require.Equal(t, "replica-3", replica3.Name)
	require.Equal(t, "http://vault-replica-3:8200", replica3.Address)
	require.Equal(t, "my_app_role_replica_3", replica3.AppRole)
	require.Equal(t, "my_app_secret_replica_3", cfg.Vault.ReplicaClusters[1].AppSecret)
}

func TestConfigurationValidation(t *testing.T) {
	t.Run("returns config without error when config is valid", func(t *testing.T) {
		viper.Reset()
		viper.SetConfigFile(filepath.Join("testdata", "config.yaml"))
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadInConfig())

		cfg, err := NewConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg)
	})

	t.Run("Return error when no config loaded", func(t *testing.T) {
		viper.Reset()
		viper.SetConfigType("yaml")

		_, err := NewConfig()
		require.Error(t, err)
		require.Contains(t, err.Error(), "is required")
	})

	t.Run("It fails on all required field if any is missing", func(t *testing.T) {
		tests := []configTestTable{
			{
				name:        "missing id",
				setFields:   deleteFromMap(validAppConfig, "id"),
				errContains: "Config.ID is required",
			},
			{
				name:        "missing interval",
				setFields:   deleteFromMap(validAppConfig, "interval"),
				errContains: "Config.Interval is required",
			},
			{
				name:        "interval not int",
				setFields:   updateAndReturnMap(validAppConfig, "interval", "a"),
				errContains: "cannot parse 'interval' as int",
			},
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
				errContains: "cannot parse 'postgres.port' as int",
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
				name:        "missing vault.main_cluster.address",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.address"),
				errContains: "Config.Vault.MainCluster.Address is required",
			},
			{
				name:        "missing vault.main_cluster.app_role",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.app_role"),
				errContains: "Config.Vault.MainCluster.AppRole is required",
			},
			{
				name:        "missing vault.main_cluster.app_secret",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.app_secret"),
				errContains: "Config.Vault.MainCluster.AppSecret is required",
			},
			{
				name:        "missing vault.main_cluster.tls_skip_verify",
				setFields:   deleteFromMap(validAppConfig, "vault.main_cluster.tls_skip_verify"),
				errContains: "Config.Vault.MainCluster.TLSSkipVerify is required",
			},
			{
				name:        "duplicated paths in vault.main_cluster.paths_to_replicate",
				setFields:   updateAndReturnMap(validAppConfig, "vault.main_cluster.paths_to_replicate", []string{"secret/data/test", "secret/data/test", "secret/data/test2"}),
				errContains: "Config.Vault.MainCluster.PathsToReplicate must contain unique items",
			},
			{
				name:        "duplicated paths in vault.main_cluster.paths_to_ignore",
				setFields:   updateAndReturnMap(validAppConfig, "vault.main_cluster.paths_to_ignore", []string{"secret/data/test", "secret/data/test", "secret/data/test2"}),
				errContains: "Config.Vault.MainCluster.PathsToIgnore must contain unique items",
			},
			{
				name: "mautual execlusive paths in vault.main_cluster.paths_to_replicate and vault.main_cluster.paths_to_ignore",
				setFields: updateAndReturnMap(
					updateAndReturnMap(validAppConfig, "vault.main_cluster.paths_to_replicate", []string{"secret/data/test1", "secret/data/test3"}),
					"vault.main_cluster.paths_to_ignore", []string{"secret/data/test4", "secret/data/test3"},
				),
				errContains: "Config.Vault.MainCluster.PathsToIgnore must not contain items that are also in PathsToReplicate, Config.Vault.MainCluster.PathsToReplicate must not contain items that are also in PathsToReplicate",
			},
			{
				name:        "replica_clusters must not be empty",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", []configFields{}),
				errContains: "Config.Vault.ReplicaClusters must have at least 1 items/characters",
			},
			{
				name:        "missing vault.replicat_cluster.name",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "name")),
				errContains: "Config.Vault.ReplicaClusters[0].Name is required",
			},
			{
				name:        "missing vault.replicat_cluster.address	",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "address")),
				errContains: "Config.Vault.ReplicaClusters[0].Address is required",
			},
			{
				name:        "missing vault.replicat_cluster.app_role",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "app_role")),
				errContains: "Config.Vault.ReplicaClusters[0].AppRole is required",
			},
			{
				name:        "missing vault.replicat_cluster.app_secret",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "app_secret")),
				errContains: "Config.Vault.ReplicaClusters[0].AppSecret is required",
			},
			{
				name:        "missing vault.replicat_cluster.tls_skip_verify",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "tls_skip_verify")),
				errContains: "Config.Vault.ReplicaClusters[0].TLSSkipVerify is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				viper.Reset()
				for k, v := range tt.setFields {
					viper.Set(k, v)
				}

				_, err := NewConfig()

				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
			})
		}
	})
}
