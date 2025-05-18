package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	ID       string   `mapstructure:"id"`
	Interval string   `mapstructure:"interval"`
	Postgres Postgres `mapstructure:"postgres"`
	Vault    Vault    `mapstructure:"vault"`
}

type Postgres struct {
	Address        string `mapstructure:"address"`
	Username       string `mapstructure:"username"`
	Password       string `mapstructure:"password"`
	DBName         string `mapstructure:"db_name"`
	SSLMode        string `mapstructure:"ssl_mode"`
	MaxConnections int    `mapstructure:"max_connections"`
}

type Vault struct {
	MainCluster     MainCluster      `mapstructure:"main_cluster"`
	ReplicaClusters []ReplicaCluster `mapstructure:"replica_clusters"`
}

type MainCluster struct {
	Address          string   `mapstructure:"address"`
	TLSSkipVerify    bool     `mapstructure:"tls_skip_verify"`
	TLSCertFile      string   `mapstructure:"tls_cert_file"`
	AppRole          string   `mapstructure:"app_role"`
	AppSecret        string   `mapstructure:"app_secret"`
	PathsToReplicate []string `mapstructure:"paths_to_replicate"`
	PathsToIgnore    []string `mapstructure:"paths_to_ignore"`
}

type ReplicaCluster struct {
	Name      string `mapstructure:"name"`
	Address   string `mapstructure:"address"`
	AppRole   string `mapstructure:"app_role"`
	AppSecret string `mapstructure:"app_secret"`
}

func NewConfig() (*Config, error) {
	// requiredFields := []string{"id", "interval", "postgres.address", "postgres.username", "postgres.password", "postgres.db_name", "vault.main_cluster.address"}
	// for _, field := range requiredFields {
	// 	if !viper.IsSet(field) || viper.GetString(field) == "" {
	// 		return nil, errors.New("missing required config field: " + field)
	// 	}
	// }

	// if viper.IsSet("vault.main_cluster.app_role") && viper.IsSet("vault.main_cluster.app_secret") {
	// 	if viper.GetString("vault.main_cluster.app_role") != "" && viper.GetString("vault.main_cluster.app_secret") != "" {
	// 		// Example: if both are set, you may want to error or warn
	// 		// return nil, errors.New("app_role and app_secret cannot both be set")
	// 		// For now, just allow both, but you can uncomment above to enforce exclusivity
	// 	}
	// }

	var cfg Config
	err := viper.Unmarshal(&cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
