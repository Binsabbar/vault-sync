package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"vault-sync/pkg/log"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	ID          string   `mapstructure:"id" validate:"required"`
	Concurrency int      `mapstructure:"concurrency" validate:"omitempty,gt=0,lt=101"`
	SyncRule    SyncRule `mapstructure:"sync_rule" validate:"required"`
	LogLevel    string   `mapstructure:"log_level" validate:"required,oneof=trace debug info warn error fatal panic"`
	Postgres    Postgres `mapstructure:"postgres" validate:"required"`
	Vault       Vault    `mapstructure:"vault" validate:"required"`
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
	MainCluster     VaultClusterConfig   `mapstructure:"main_cluster" validate:"required"`
	ReplicaClusters []VaultClusterConfig `mapstructure:"replica_clusters" validate:"required,min=1,dive"`
}

type VaultClusterConfig struct {
	Name          string `mapstructure:"name" validate:"required"`
	Address       string `mapstructure:"address" validate:"required,url"`
	AppRoleID     string `mapstructure:"app_role_id" validate:"required"`
	AppRoleSecret string `mapstructure:"app_role_secret" validate:"required"`
	AppRoleMount  string `mapstructure:"app_role_mount"`
	TLSSkipVerify bool   `mapstructure:"tls_skip_verify" validate:"boolean"`
	TLSCertFile   string `mapstructure:"tls_cert_file" validate:"omitempty,filepath"`
}

type SyncRule struct {
	Interval         string   `mapstructure:"interval" validate:"required,period_regex,period_limit_max=24h,period_limit_min=60s"`
	KvMounts         []string `mapstructure:"kv_mounts" validate:"required,min=1,unique"`
	PathsToReplicate []string `mapstructure:"paths_to_replicate" validate:"omitempty,min=0,unique"`
	PathsToIgnore    []string `mapstructure:"paths_to_ignore" validate:"omitempty,unique,min=0"`
}

func (syncRule *SyncRule) GetInterval() time.Duration {
	fmt.Println("Calculating interval for sync rule:", syncRule.Interval)
	duration, _ := time.ParseDuration(syncRule.Interval)
	return duration
}

var validate = validator.New()

func init() {
	validate.RegisterStructValidation(syncRuleValidation, SyncRule{})
	validate.RegisterValidation("period_regex", periodRegexValidator)
	validate.RegisterValidation("period_limit_max", periodLimitMaxValidator)
	validate.RegisterValidation("period_limit_min", periodLimitMinValidator)
}

var periodRegex = regexp.MustCompile(`^([0-9]+(s|m|h))$`)

func periodRegexValidator(fl validator.FieldLevel) bool {
	return periodRegex.MatchString(fl.Field().String())
}

func periodLimitMaxValidator(fl validator.FieldLevel) bool {
	fieldValue := fl.Field().String()
	fieldParam := fl.Param()
	maxDuration, err := time.ParseDuration(fieldParam)
	if err != nil {
		return false
	}
	duration, err := time.ParseDuration(fieldValue)
	if err != nil {
		return false
	}
	return duration <= maxDuration
}

func periodLimitMinValidator(fl validator.FieldLevel) bool {
	fieldValue := fl.Field().String()
	fieldParam := fl.Param()
	minimumDuration, err := time.ParseDuration(fieldParam)
	if err != nil {
		return false
	}
	duration, err := time.ParseDuration(fieldValue)
	if err != nil {
		fmt.Println("Error parsing field value as duration:", err)
		return false
	}
	return duration >= minimumDuration
}

func syncRuleValidation(sl validator.StructLevel) {
	syncRule := sl.Current().Interface().(SyncRule)

	set := make(map[string]struct{}, len(syncRule.PathsToReplicate))
	for _, p := range syncRule.PathsToReplicate {
		set[p] = struct{}{}
	}

	for _, p := range syncRule.PathsToIgnore {
		if _, exists := set[p]; exists {
			sl.ReportError(syncRule.PathsToIgnore, "PathsToIgnore", "paths_to_ignore", "no_overlap", "")
			sl.ReportError(syncRule.PathsToReplicate, "PathsToReplicate", "paths_to_replicate", "no_overlap", "")
			break
		}
	}
}

func NewConfig() (*Config, error) {
	logger := log.Logger.With().Str("component", "config").Logger()
	var cfg Config
	viper.SetDefault("log_level", "info")
	viper.SetDefault("postgres.ssl_mode", "disable")
	viper.SetDefault("vault.main_cluster.app_role_mount", "approle")

	if err := viper.Unmarshal(&cfg); err != nil {
		logger.Err(err).Msg("Failed to unmarshal config")
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	logger.Info().Msg("Configuration loaded successfully")

	for index, r := range cfg.Vault.ReplicaClusters {
		if r.AppRoleMount == "" {
			r.AppRoleMount = "approle"
		}
		cfg.Vault.ReplicaClusters[index] = r
	}

	if err := validateConfig(cfg); err != nil {
		logger.Err(err).Msg("Failed to validate config")
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
		case "period_limit_min":
			msg = fmt.Sprintf("%s must be greater than or equal to %s", namespace, param)
		case "period_limit_max":
			msg = fmt.Sprintf("%s must be less than or equal to %s", namespace, param)
		case "period_regex":
			msg = fmt.Sprintf("%s must match the format of a valid duration (e.g., 1s, 5m, 2h)", namespace)
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
