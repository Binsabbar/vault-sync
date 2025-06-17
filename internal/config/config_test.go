package config

import (
	"maps"
	"path/filepath"
	"testing"

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
	"id":                                 "test",
	"interval":                           5,
	"log_level":                          "info",
	"postgres.address":                   "localhost",
	"postgres.port":                      5432,
	"postgres.username":                  "u",
	"postgres.password":                  "p",
	"postgres.db_name":                   "d",
	"postgres.max_connection":            "10",
	"vault.main_cluster.address":         "http://vault:8200",
	"vault.main_cluster.app_role_id":     "r",
	"vault.main_cluster.app_role_secret": "s",
	"vault.main_cluster.app_role_mount":  "s",
	"vault.main_cluster.tls_skip_verify": "true",
	"vault.replica_clusters":             []configFields{validVaultReplicaClusterConfig},
}

var validVaultReplicaClusterConfig = configFields{
	"name":            "r2",
	"address":         "http://vault-replica-2:8200",
	"app_role_id":     "r",
	"app_role_secret": "s",
	"app_role_mount":  "s",
	"tls_skip_verify": "true",
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
	require.Equal(t, "info", cfg.LogLevel)

	// Check Postgres configuration
	require.Equal(t, "localhost", cfg.Postgres.Address)
	require.Equal(t, 5432, cfg.Postgres.Port)
	require.Equal(t, "postgres", cfg.Postgres.Username)
	require.Equal(t, "vault_sync", cfg.Postgres.DBName)
	require.Equal(t, "disable", cfg.Postgres.SSLMode)
	require.Equal(t, "/path/to/root.crt", cfg.Postgres.SSLRootCertFile)

	require.Equal(t, 10, cfg.Postgres.MaxConnections)

	// Check Vault configuration main cluster
	require.Equal(t, "http://vault:8200", cfg.Vault.MainCluster.Address)
	require.True(t, cfg.Vault.MainCluster.TLSSkipVerify)
	require.Equal(t, "/path/to/cert.pem", cfg.Vault.MainCluster.TLSCertFile)
	require.Equal(t, "my_app_role", cfg.Vault.MainCluster.AppRoleID)
	require.Equal(t, "my_app_secret", cfg.Vault.MainCluster.AppRoleSecret)
	require.Equal(t, "approle", cfg.Vault.MainCluster.AppRoleMount)
	require.ElementsMatch(t, []string{"secret/data/test", "secret/data/test2"}, cfg.Vault.MainCluster.PathsToReplicate)
	require.ElementsMatch(t, []string{"secret/data/test3", "secret/data/test4"}, cfg.Vault.MainCluster.PathsToIgnore)

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

	t.Run("It fails when validation fails", func(t *testing.T) {
		tests := []invalidConfigTestTable{
			// root level
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
				name:        "invalid log_level value",
				setFields:   updateAndReturnMap(validAppConfig, "log_level", "invalid"),
				errContains: "Config.LogLevel must be one of [trace debug info warn error fatal panic]",
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
				errContains: "cannot parse 'vault.main_cluster.tls_skip_verify' as bool",
			},
			{
				name:        "invalid vault.main_cluster.tls_cert_file",
				setFields:   updateAndReturnMap(updateAndReturnMap(validAppConfig, "vault.main_cluster.tls_cert_file", "invalid+/"), "vault.main_cluster.tls_skip_verify", "false"),
				errContains: "Config.Vault.MainCluster.TLSCertFile must be a valid file path",
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
				name:        "missing vault.replica_cluster.name",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "name")),
				errContains: "Config.Vault.ReplicaClusters[0].Name is required",
			},
			{
				name:        "missing vault.replica_cluster.address	",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "address")),
				errContains: "Config.Vault.ReplicaClusters[0].Address is required",
			},
			{
				name:        "invalid vault.replica_cluster.address	",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", updateAndReturnMap(validVaultReplicaClusterConfig, "address", "invalid")),
				errContains: "Config.Vault.ReplicaClusters[0].Address must be a valid URL",
			},
			{
				name:        "missing vault.replica_cluster.app_role_id",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "app_role_id")),
				errContains: "Config.Vault.ReplicaClusters[0].AppRoleID is required",
			},
			{
				name:        "missing vault.replica_cluster.app_role_secret",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "app_role_secret")),
				errContains: "Config.Vault.ReplicaClusters[0].AppRoleSecret is required",
			},
			{
				name:        "invalid vault.replica_cluster.tls_cert_file",
				setFields:   updateAndReturnMap(validAppConfig, "vault.replica_clusters", updateAndReturnMap(validVaultReplicaClusterConfig, "tls_cert_file", "invalid+/")),
				errContains: "Config.Vault.ReplicaClusters[0].TLSCertFile must be a valid file path",
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

	t.Run("It sets default values for optional params", func(t *testing.T) {
		viper.Reset()
		config := deleteFromMap(
			updateAndReturnMap(validAppConfig, "vault.replica_clusters", deleteFromMap(validVaultReplicaClusterConfig, "app_role_mount", "tls_skip_verify")),
			"log_level",
			"postgres.ssl_mode",
			"vault.main_cluster.app_role_mount",
			"vault.main_cluster.tls_skip_verify",
		)
		for k, v := range config {
			viper.Set(k, v)
		}

		cfg, _ := NewConfig()

		assert.Equal(t, "info", cfg.LogLevel, "Default value for log_level should be 'info'")
		assert.Equal(t, "disable", cfg.Postgres.SSLMode, "Default value for postgres.ssl_mode should be 'disable'")
		assert.Equal(t, "approle", cfg.Vault.MainCluster.AppRoleMount, "Default value for vault.main_cluster.app_role_mount should be 'approle'")
		assert.Equal(t, false, cfg.Vault.MainCluster.TLSSkipVerify, "Default value for vault.main_cluster.tls_skip_verify should be 'false'")
		assert.Equal(t, "approle", cfg.Vault.ReplicaClusters[0].AppRoleMount, "Default value for vault.replica_clusters[0].app_role_mount should be 'approle'")
		assert.Equal(t, false, cfg.Vault.ReplicaClusters[0].TLSSkipVerify, "Default value for vault.replica_clusters[0].tls_skip_verify should be 'false'")

	})
}

func TestMainCluster_MapToVaultConfig(t *testing.T) {
	main := &MainCluster{
		Address:          "http://vault:8200",
		AppRoleID:        "role",
		AppRoleSecret:    "secret",
		AppRoleMount:     "approle-team1",
		TLSSkipVerify:    true,
		TLSCertFile:      "/path/to/cert.pem",
		PathsToReplicate: []string{"secret/data/a"},
		PathsToIgnore:    []string{"secret/data/b"},
	}

	want := &VaultConfig{
		Address:       main.Address,
		AppRoleID:     main.AppRoleID,
		AppRoleSecret: main.AppRoleSecret,
		AppRoleMount:  main.AppRoleMount,
		TLSSkipVerify: main.TLSSkipVerify,
		TLSCertFile:   main.TLSCertFile,
	}

	got := main.MapToVaultConfig()
	require.Equal(t, want, got)
}

func TestReplicaCluster_MapToVaultConfig(t *testing.T) {
	replica := &ReplicaCluster{
		Name:          "replica-1",
		Address:       "http://vault-replica:8200",
		AppRoleID:     "role-replica",
		AppRoleSecret: "secret-replica",
		AppRoleMount:  "myappmount",
		TLSSkipVerify: false,
		TLSCertFile:   "/path/to/replica-cert.pem",
	}

	want := &VaultConfig{
		Address:       replica.Address,
		AppRoleID:     replica.AppRoleID,
		AppRoleSecret: replica.AppRoleSecret,
		AppRoleMount:  replica.AppRoleMount,
		TLSSkipVerify: replica.TLSSkipVerify,
		TLSCertFile:   replica.TLSCertFile,
	}

	got := replica.MapToVaultConfig()
	require.Equal(t, want, got)
}
