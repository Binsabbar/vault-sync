package config

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	ID       string   `mapstructure:"id" validate:"required"`
	Interval int      `mapstructure:"interval" validate:"required"`
	Postgres Postgres `mapstructure:"postgres" validate:"required"`
	Vault    Vault    `mapstructure:"vault" validate:"required"`
}

type Postgres struct {
	Address        string `mapstructure:"address" validate:"required"`
	Username       string `mapstructure:"username" validate:"required"`
	Password       string `mapstructure:"password" validate:"required"`
	DBName         string `mapstructure:"db_name" validate:"required"`
	SSLMode        string `mapstructure:"ssl_mode"`
	MaxConnections int    `mapstructure:"max_connections"`
}

type Vault struct {
	MainCluster     MainCluster      `mapstructure:"main_cluster" validate:"required"`
	ReplicaClusters []ReplicaCluster `mapstructure:"replica_clusters" validate:"required,min=1,dive"`
}

type MainCluster struct {
	Address          string   `mapstructure:"address" validate:"required"`
	AppRole          string   `mapstructure:"app_role" validate:"required"`
	AppSecret        string   `mapstructure:"app_secret" validate:"required"`
	TLSSkipVerify    bool     `mapstructure:"tls_skip_verify" validate:"required"`
	TLSCertFile      string   `mapstructure:"tls_cert_file"`
	PathsToReplicate []string `mapstructure:"paths_to_replicate"`
	PathsToIgnore    []string `mapstructure:"paths_to_ignore"`
}

type ReplicaCluster struct {
	Name          string `mapstructure:"name" validate:"required"`
	Address       string `mapstructure:"address" validate:"required"`
	AppRole       string `mapstructure:"app_role" validate:"required" `
	AppSecret     string `mapstructure:"app_secret"  validate:"required"`
	TLSSkipVerify bool   `mapstructure:"tls_skip_verify" validate:"required"`
	TLSCertFile   string `mapstructure:"tls_cert_file"`
}

var validate = validator.New()

// func TLSCertFileStructLevelValidation(sl validator.StructLevel) {
// 	switch cluster := sl.Current().Interface().(type) {
// 	case MainCluster:
// 		if !cluster.TLSSkipVerify && cluster.TLSCertFile == "" {
// 			sl.ReportError(cluster.TLSCertFile, "TLSCertFile", "tls_cert_file", "required_with_tls", "")
// 		}
// 	case ReplicaCluster:
// 		if !cluster.TLSSkipVerify && cluster.TLSCertFile == "" {
// 			sl.ReportError(cluster.TLSCertFile, "TLSCertFile", "tls_cert_file", "required_with_tls", "")
// 		}
// 	}
// }

// func init() {
// 	validate.RegisterStructValidation(TLSCertFileStructLevelValidation, MainCluster{})
// 	validate.RegisterStructValidation(TLSCertFileStructLevelValidation, ReplicaCluster{})
// }

func NewConfig() (*Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := validate.Struct(cfg); err != nil {
		validationError := err.(validator.ValidationErrors)
		validationErrors := make([]string, 0)
		for _, fieldError := range validationError {
			fieldName := fieldError.Namespace()
			validationErrors = append(validationErrors, fieldName)
		}
		return nil, errors.New(fmt.Sprintf("validation error: the following keys are missing or invalid: %s", validationErrors))
	}
	return &cfg, nil
}
