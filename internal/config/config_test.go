package config

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestConfigLoadFromYAML(t *testing.T) {
	// Set viper as the global instance for NewConfig
	viper.SetConfigFile(filepath.Join("testdata", "config.yaml"))
	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadInConfig())

	cfg, err := NewConfig()
	require.NoError(t, err)

	require.Equal(t, "test", cfg.ID)
	require.Equal(t, 5, cfg.Interval)

	// Check Postgres configuration
	require.Equal(t, "localhost:5432", cfg.Postgres.Address)
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

func TestConfigRequiredFields(t *testing.T) {
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
		require.Contains(t, err.Error(), "validation error")
	})

	t.Run("Return error if only some required fields set", func(t *testing.T) {
		tests := []struct {
			name        string
			setFields   map[string]interface{}
			wantErr     bool
			errContains string
		}{
			{
				name:        "missing id",
				setFields:   map[string]interface{}{},
				wantErr:     true,
				errContains: "Config.ID",
			},
			{
				name:        "missing interval",
				setFields:   map[string]interface{}{"id": "test"},
				wantErr:     true,
				errContains: "Config.Interval",
			},
			{
				name:        "interval not int",
				setFields:   map[string]interface{}{"id": "test", "interval": "notanint"},
				wantErr:     true,
				errContains: "cannot parse 'interval' as int",
			},
			{
				name:        "missing postgres.address",
				setFields:   map[string]interface{}{"id": "test", "interval": 5},
				wantErr:     true,
				errContains: "Config.Postgres.Address",
			},
			{
				name:        "missing postgres.username",
				setFields:   map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a"},
				wantErr:     true,
				errContains: "Config.Postgres.Username",
			},
			{
				name:        "missing postgres.password",
				setFields:   map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u"},
				wantErr:     true,
				errContains: "Config.Postgres.Password",
			},
			{
				name:        "missing postgres.db_name",
				setFields:   map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p"},
				wantErr:     true,
				errContains: "Config.Postgres.DBName",
			},
			{
				name:        "missing vault.main_cluster.address",
				setFields:   map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d"},
				wantErr:     true,
				errContains: "Config.Vault.MainCluster.Address",
			},
			{
				name: "missing vault.main_cluster.app_role",
				setFields: map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a"},
				wantErr:     true,
				errContains: "Config.Vault.MainCluster.AppRole",
			},
			{
				name: "missing vault.main_cluster.app_secret",
				setFields: map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r"},
				wantErr:     true,
				errContains: "Config.Vault.MainCluster.AppSecret",
			},
			{
				name: "missing vault.main_cluster.tls_skip_verify",
				setFields: map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r", "vault.main_cluster.app_secret": "s"},
				wantErr:     true,
				errContains: "Config.Vault.MainCluster.TLSSkipVerify",
			},
			{
				name: "replica_clusters must not be empty",
				setFields: map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r", "vault.main_cluster.app_secret": "s", "vault.main_cluster.tls_skip_verify": true,
					"vault.replica_clusters": []interface{}{}},
				wantErr:     true,
				errContains: "Config.Vault.ReplicaClusters",
			},
			{
				name: "missing vault.replicat_cluster.name",
				setFields: map[string]interface{}{
					"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r", "vault.main_cluster.app_secret": "s", "vault.main_cluster.tls_skip_verify": true,
					"vault.replica_clusters": []map[string]interface{}{{}},
				},
				wantErr:     true,
				errContains: "Config.Vault.ReplicaClusters[0].Name",
			},
			{
				name: "missing vault.replicat_cluster.address	",
				setFields: map[string]interface{}{
					"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r", "vault.main_cluster.app_secret": "s", "vault.main_cluster.tls_skip_verify": true,
					"vault.replica_clusters": []map[string]interface{}{{"name": "r2"}},
				},
				wantErr:     true,
				errContains: "Config.Vault.ReplicaClusters[0].Address",
			},
			{
				name: "missing vault.replicat_cluster.app_role",
				setFields: map[string]interface{}{
					"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r", "vault.main_cluster.app_secret": "s", "vault.main_cluster.tls_skip_verify": true,
					"vault.replica_clusters": []map[string]interface{}{{"name": "r2", "address": "a"}},
				},
				wantErr:     true,
				errContains: "Config.Vault.ReplicaClusters[0].AppRole",
			},
			{
				name: "missing vault.replicat_cluster.app_secret",
				setFields: map[string]interface{}{
					"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
					"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r", "vault.main_cluster.app_secret": "s", "vault.main_cluster.tls_skip_verify": true,
					"vault.replica_clusters": []map[string]interface{}{{"name": "r2", "address": "a", "app_role": "r"}},
				},
				wantErr:     true,
				errContains: "Config.Vault.ReplicaClusters[0].AppSecret",
			},
			{
				name:        "missing vault.replicat_cluster.tls_skip_verify",
				setFields:   DeleteFromMap(validConfig, "vault.replica_cluster[0].tls_skip_verify"),
				wantErr:     true,
				errContains: "Config.Vault.ReplicaClusters[0].TLSSkipVerify",
			},
			{
				name:      "all required fields set",
				setFields: validConfig,
				wantErr:   false,
			},
			// {
			// 	name: "missing vault.replicat_cluster.tls_cert_file when tls_skip_verify is false",
			// 	setFields: map[string]interface{}{
			// 		"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
			// 		"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r", "vault.main_cluster.app_secret": "s", "vault.main_cluster.tls_skip_verify": true,
			// 		"vault.replica_clusters": []map[string]interface{}{{"name": "r2", "address": "a", "app_role": "r", "app_secret": "s", "tls_skip_verify": false}},
			// 	},
			// 	wantErr:     true,
			// 	errContains: "Config.Vault.ReplicaClusters[0].TLSCertFile",
			// },
			// {
			// 	name: "missing vault.main_cluster.tls_cert_file when tls_skip_verify is false",
			// 	setFields: map[string]interface{}{"id": "test", "interval": 5, "postgres.address": "a", "postgres.username": "u", "postgres.password": "p", "postgres.db_name": "d",
			// 		"vault.main_cluster.address": "a", "vault.main_cluster.app_role": "r",
			// 		"vault.main_cluster.app_secret": "s", "vault.main_cluster.tls_skip_verify": false},
			// 	wantErr:     true,
			// 	errContains: "Config.Vault.MainCluster.TLSCertFile",
			// },
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				viper.Reset()
				for k, v := range tt.setFields {
					viper.Set(k, v)
				}
				_, err := NewConfig()
				if tt.wantErr {
					require.Error(t, err)
					require.Contains(t, err.Error(), tt.errContains)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})
}

var validConfig = map[string]interface{}{
	"id":                                 "test",
	"interval":                           5,
	"postgres.address":                   "a",
	"postgres.username":                  "u",
	"postgres.password":                  "p",
	"postgres.db_name":                   "d",
	"vault.main_cluster.address":         "a",
	"vault.main_cluster.app_role":        "r",
	"vault.main_cluster.app_secret":      "s",
	"vault.main_cluster.tls_skip_verify": true,
	"vault.replica_clusters": []map[string]interface{}{
		{"name": "r2", "address": "a", "app_role": "r", "app_secret": "s", "tls_skip_verify": true},
	},
}

func DeleteFromMap(m map[string]interface{}, key string) map[string]interface{} {
	delete(m, key)
	return m
}
