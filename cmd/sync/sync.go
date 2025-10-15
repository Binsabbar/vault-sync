package sync

import (
	"vault-sync/internal/config"
	"vault-sync/internal/core"
	"vault-sync/pkg/log"

	"github.com/spf13/cobra"
)

var SyncCmd = &cobra.Command{
	Use:     "sync",
	Short:   "start the sync process until it is stopped",
	Long:    `it will start the sync process and keep it running until it is stopped`,
	Example: `vault-sync sync --config /path/to/config.yaml`,
	Run:     run,
}

func run(cmd *cobra.Command, args []string) {
	logger := log.Logger.With().Str("component", "sync-cmd").Logger()
	logger.Info().Msg("Starting vault-sync")

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
	logger.Info().Msg("Sync process completed successfully")
}
