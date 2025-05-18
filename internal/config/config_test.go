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
	require.Equal(t, "5s", cfg.Interval)

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
	require.Equal(t, "my_app_secret_replica_3", replica3.AppSecret)
}
