package sync

import (
	"vault-sync/internal/config"
	"vault-sync/internal/core"
	"vault-sync/pkg/log"

	"github.com/spf13/cobra"
)

var SyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize secrets between Vault clusters",
	Long:  `Synchronize secrets between Vault clusters with various execution modes.`,
}

var onceCmd = &cobra.Command{
	Use:     "once",
	Short:   "Run sync operation once and exit",
	Long:    `Perform a one-time synchronization of secrets and exit.`,
	Example: `vault-sync sync once --config /path/to/config.yaml`,
	Run:     runOnce,
}

// var daemonCmd = &cobra.Command{
// 	Use:     "daemon",
// 	Short:   "Run sync as a scheduled daemon",
// 	Long:    `Run sync operations continuously based on configured schedule.`,
// 	Example: `vault-sync sync daemon --config /path/to/config.yaml`,
// 	Run:     runDaemon,
// }

var dryRunCmd = &cobra.Command{
	Use:     "dry-run",
	Short:   "Show what would be synced without actually syncing",
	Long:    `Discover and display all secrets that would be synchronized without performing actual sync.`,
	Example: `vault-sync sync dry-run --config /path/to/config.yaml`,
	Run:     runDryRun,
}

func init() {
	SyncCmd.AddCommand(onceCmd)
	SyncCmd.AddCommand(dryRunCmd)
	// SyncCmd.Run = runOnce
}

func runOnce(cmd *cobra.Command, _ []string) {
	logger := log.Logger.With().Str("component", "sync-once").Logger()
	logger.Info().Msg("Starting one-time vault-sync")

	appConfig, err := config.NewConfig()
	if err != nil {
		logger.Error().Err(err).Msg("Error creating config")
		return
	}

	wiring := core.NewWiring(appConfig)
	ctx := cmd.Context()

	orchestrator := wiring.InitOrchestrator(ctx)
	_, err = orchestrator.StartSync(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Error during sync")
		return
	}
	logger.Info().Msg("One-time sync completed successfully")
}

// func runDaemon(cmd *cobra.Command, args []string) {
// 	logger := log.Logger.With().Str("component", "sync-daemon").Logger()
// 	logger.Info().Msg("Starting vault-sync daemon")

// 	appConfig, err := config.NewConfig()
// 	if err != nil {
// 		logger.Error().Err(err).Msg("Error creating config")
// 		return
// 	}

// 	wiring := core.NewWiring(appConfig)
// 	ctx := cmd.Context()

// 	// TODO: Implement scheduler/daemon mode
// 	// This will use the interval from config.SyncRule.Interval
// 	// and run sync operations continuously

// 	logger.Info().Msg("Daemon mode - TODO: Implement scheduler")

// 	// For now, run once
// 	orchestrator := wiring.InitOrchestrator(ctx)
// 	_, err = orchestrator.StartSync(ctx)
// 	if err != nil {
// 		logger.Error().Err(err).Msg("Error during sync")
// 		return
// 	}
// }

func runDryRun(cmd *cobra.Command, _ []string) {
	logger := log.Logger.With().Str("component", "sync-dry-run").Logger()
	logger.Info().Msg("Starting vault-sync dry-run")

	appConfig, err := config.NewConfig()
	if err != nil {
		logger.Error().Err(err).Msg("Error creating config")
		return
	}

	wiring := core.NewWiring(appConfig)
	ctx := cmd.Context()

	pathMatcher := wiring.InitPathMatcher()
	secrets, err := pathMatcher.DiscoverSecretsForSync(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Error discovering secrets for dry-run")
		return
	}

	logger.Info().Msg("=== DRY RUN: Secrets that would be synced ===")

	mountGroups := make(map[string][]string)
	for _, secret := range secrets {
		mountGroups[secret.Mount] = append(mountGroups[secret.Mount], secret.KeyPath)
	}

	for mount, paths := range mountGroups {
		logger.Info().Str("mount", mount).Int("count", len(paths)).Msg("Mount summary")
		for _, path := range paths {
			logger.Info().Str("mount", mount).Str("path", path).Msg(" → Would sync")
		}
	}

	logger.Info().Int("total_secrets", len(secrets)).Msg("=== DRY RUN COMPLETE ===")
}
