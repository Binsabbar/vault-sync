package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	ID       string   `mapstructure:"id" validate:"required"`
	Interval int      `mapstructure:"interval" validate:"required"`
	LogLevel string   `mapstructure:"log_level" validate:"required,oneof=trace debug info warn error fatal panic"`
	Postgres Postgres `mapstructure:"postgres" validate:"required"`
	Vault    Vault    `mapstructure:"vault" validate:"required"`
}

type Postgres struct {
	Address         string `mapstructure:"address" validate:"required,hostname|ip"`
	Port            int    `mapstructure:"port" validate:"required,gt=0,lt=65536"`
	Username        string `mapstructure:"username" validate:"required"`
	Password        string `mapstructure:"password" validate:"required"`
	DBName          string `mapstructure:"db_name" validate:"required"`
	SSLMode         string `mapstructure:"ssl_mode" validate:"omitempty,oneof=disable allow prefer require verify-ca verify-full"`
	SSLRootCertFile string `mapstructure:"ssl_root_cert_file" validate:"omitempty,filepath"`
	MaxConnections  int    `mapstructure:"max_connections"`
}

type Vault struct {
	MainCluster     MainCluster      `mapstructure:"main_cluster" validate:"required"`
	ReplicaClusters []ReplicaCluster `mapstructure:"replica_clusters" validate:"required,min=1,dive"`
}

type MainCluster struct {
	Address          string   `mapstructure:"address" validate:"required,url"`
	AppRoleID        string   `mapstructure:"app_role_id" validate:"required"`
	AppRoleSecret    string   `mapstructure:"app_role_secret" validate:"required"`
	AppRoleMount     string   `mapstructure:"app_role_mount"`
	TLSSkipVerify    bool     `mapstructure:"tls_skip_verify" validate:"boolean"`
	TLSCertFile      string   `mapstructure:"tls_cert_file" validate:"omitempty,filepath"`
	PathsToReplicate []string `mapstructure:"paths_to_replicate" validate:"unique"`
	PathsToIgnore    []string `mapstructure:"paths_to_ignore" validate:"unique"`
}

type ReplicaCluster struct {
	Name          string `mapstructure:"name" validate:"required"`
	Address       string `mapstructure:"address" validate:"required,url"`
	AppRoleID     string `mapstructure:"app_role_id" validate:"required"`
	AppRoleSecret string `mapstructure:"app_role_secret" validate:"required"`
	AppRoleMount  string `mapstructure:"app_role_mount"`
	TLSSkipVerify bool   `mapstructure:"tls_skip_verify" validate:"boolean"`
	TLSCertFile   string `mapstructure:"tls_cert_file" validate:"omitempty,filepath"`
}

type VaultConfig struct {
	Address       string `mapstructure:"address" validate:"required,url"`
	AppRoleID     string `mapstructure:"app_role_id" validate:"required"`
	AppRoleSecret string `mapstructure:"app_role_secret" validate:"required"`
	AppRoleMount  string `mapstructure:"app_role_mount"`
	TLSSkipVerify bool   `mapstructure:"tls_skip_verify" validate:"boolean"`
	TLSCertFile   string `mapstructure:"tls_cert_file" validate:"omitempty,filepath"`
}

var validate = validator.New()

func init() {
	validate.RegisterStructValidation(MainClusterValidation, MainCluster{})
}

func MainClusterValidation(sl validator.StructLevel) {
	cluster := sl.Current().Interface().(MainCluster)

	set := make(map[string]struct{}, len(cluster.PathsToReplicate))
	for _, p := range cluster.PathsToReplicate {
		set[p] = struct{}{}
	}

	for _, p := range cluster.PathsToIgnore {
		if _, exists := set[p]; exists {
			sl.ReportError(cluster.PathsToIgnore, "PathsToIgnore", "paths_to_ignore", "no_overlap", "")
			sl.ReportError(cluster.PathsToReplicate, "PathsToReplicate", "paths_to_replicate", "no_overlap", "")
			break
		}
	}
}

func NewConfig() (*Config, error) {
	var cfg Config
	viper.SetDefault("log_level", "info")
	viper.SetDefault("postgres.ssl_mode", "disable")
	viper.SetDefault("vault.main_cluster.app_role_mount", "approle")

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	for index, r := range cfg.Vault.ReplicaClusters {
		if r.AppRoleMount == "" {
			r.AppRoleMount = "approle"
		}
		cfg.Vault.ReplicaClusters[index] = r
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *MainCluster) MapToVaultConfig() *VaultConfig {
	return &VaultConfig{
		Address:       c.Address,
		AppRoleID:     c.AppRoleID,
		AppRoleSecret: c.AppRoleSecret,
		AppRoleMount:  c.AppRoleMount,
		TLSSkipVerify: c.TLSSkipVerify,
		TLSCertFile:   c.TLSCertFile,
	}
}

func (c *ReplicaCluster) MapToVaultConfig() *VaultConfig {
	return &VaultConfig{
		Address:       c.Address,
		AppRoleID:     c.AppRoleID,
		AppRoleSecret: c.AppRoleSecret,
		AppRoleMount:  c.AppRoleMount,
		TLSSkipVerify: c.TLSSkipVerify,
		TLSCertFile:   c.TLSCertFile,
	}
}

func validateConfig(cfg Config) error {
	err := validate.Struct(cfg)
	if err == nil {
		return nil
	}

	validationErrs, ok := err.(validator.ValidationErrors)
	if !ok {
		return fmt.Errorf("validation error (unexpected type): %w", err)
	}

	var errorMessages []string
	for _, fieldError := range validationErrs {
		namespace := fieldError.Namespace()
		param := fieldError.Param()
		var msg string = fmt.Sprintf("%s is invalid (rule: %s)", namespace, fieldError.Tag())

		switch fieldError.Tag() {
		case "required":
			msg = fmt.Sprintf("%s is required", namespace)
		case "hostname|ip":
			msg = fmt.Sprintf("%s must be a valid hostname or IP address", namespace)
		case "url":
			msg = fmt.Sprintf("%s must be a valid URL", namespace)
		case "gt":
			msg = fmt.Sprintf("%s must be greater than %s", namespace, param)
		case "lt":
			msg = fmt.Sprintf("%s must be less than %s", namespace, param)
		case "unique":
			msg = fmt.Sprintf("%s must contain unique items", namespace)
		case "min":
			msg = fmt.Sprintf("%s must have at least %s items/characters", namespace, param)
		case "oneof":
			msg = fmt.Sprintf("%s must be one of [%s]", namespace, param)
		case "filepath":
			msg = fmt.Sprintf("%s must be a valid file path", namespace)
		case "no_overlap":
			otherField := "PathsToReplicate"
			if fieldError.StructField() == otherField {
				otherField = "PathsToIgnore"
			}
			msg = fmt.Sprintf("%s must not contain items that are also in %s", namespace, otherField)
		}
		errorMessages = append(errorMessages, msg)
	}

	return errors.New(strings.Join(errorMessages, ", "))
}
