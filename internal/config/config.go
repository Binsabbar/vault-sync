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
	Postgres Postgres `mapstructure:"postgres" validate:"required"`
	Vault    Vault    `mapstructure:"vault" validate:"required"`
}

type Postgres struct {
	Address        string `mapstructure:"address" validate:"required,hostname|ip"`
	Port           int    `mapstructure:"port" validate:"required,gt=0,lt=65536"`
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
	PathsToReplicate []string `mapstructure:"paths_to_replicate" validate:"unique"`
	PathsToIgnore    []string `mapstructure:"paths_to_ignore" validate:"unique"`
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

func init() {
	validate.RegisterStructValidation(MainClusterPathsOverlapValidation, MainCluster{})
}

func MainClusterPathsOverlapValidation(sl validator.StructLevel) {
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
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
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
		case "gt":
			msg = fmt.Sprintf("%s must be greater than %s", namespace, param)
		case "lt":
			msg = fmt.Sprintf("%s must be less than %s", namespace, param)
		case "unique":
			msg = fmt.Sprintf("%s must contain unique items", namespace)
		case "min":
			msg = fmt.Sprintf("%s must have at least %s items/characters", namespace, param)
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
