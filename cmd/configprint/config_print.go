package configprint

import (
	"fmt"
	"vault-sync/internal/config"
	"vault-sync/pkg/log"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	sectionFlag string
	formatFlag  string
)

var ConfigPrintCmd = &cobra.Command{
	Use:   "config-print",
	Short: "Print the current configuration",
	Long: `Print the loaded configuration or a specific section of it.
Supports YAML and JSON output formats.`,
	Example: `  # Print entire config
  vault-sync config-print

  # Print specific section
  vault-sync config-print --section main_cluster
  vault-sync config-print --section database
  vault-sync config-print --section sync_rule

  # Print in JSON format
  vault-sync config-print --section replica_clusters --format json`,
	Run: run,
}

func init() {
	ConfigPrintCmd.Flags().StringVarP(&sectionFlag, "section", "s", "",
		"print only a specific section (main_cluster, replica_clusters, database, sync_rule)")
	ConfigPrintCmd.Flags().StringVarP(&formatFlag, "format", "f", "json",
		"output format (yaml|json)")
}

func run(_ *cobra.Command, _ []string) {
	logger := log.Logger.With().Str("component", "config_print").Logger()

	cfg, err := config.Load()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load configuration")
		return
	}

	var output interface{}

	if sectionFlag == "" {
		output = cfg
		logger.Info().Msg("Printing entire configuration")
	} else {
		output, err = getSection(cfg, sectionFlag)
		if err != nil {
			logger.Error().Err(err).Str("section", sectionFlag).Msg("Invalid section")
			return
		}
		logger.Info().Str("section", sectionFlag).Msg("Printing configuration section")
	}

	switch formatFlag {
	case "yaml":
		printYAML(logger, output)
	case "json":
		printJSON(logger, output)
	default:
		logger.Error().Msgf("unsupported format: %s (use 'yaml' or 'json')", formatFlag)
	}
}

func getSection(cfg *config.Config, section string) (interface{}, error) {
	switch section {
	case "main_cluster":
		return cfg.Vault.MainCluster, nil
	case "replica_clusters":
		return cfg.Vault.ReplicaClusters, nil
	case "postgres":
		return cfg.Postgres, nil
	case "sync_rule":
		return cfg.SyncRule, nil
	case "concurrency":
		return map[string]int{"concurrency": cfg.Concurrency}, nil
	case "log_level":
		return map[string]string{"log_level": cfg.LogLevel}, nil
	case "id":
		return map[string]string{"id": cfg.ID}, nil
	default:
		return nil,
			fmt.Errorf(
				"unknown section: %s (valid: main_cluster, replica_clusters, postgres, sync_rule, "+
					"concurrency, id, log_level)",
				section,
			)
	}
}

func printYAML(logger zerolog.Logger, data interface{}) {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		logger.Error().Err(err).Msg("failed to encode YAML")
	}
	content := string(bytes)
	logger.Info().
		Str("format", "yaml").
		Str("config", "\n"+content).
		Msg("Printing Configuration")
}

func printJSON(logger zerolog.Logger, data interface{}) {
	logger.Info().Stack().Interface("config", data).Msg("Printing Configuration")
}
