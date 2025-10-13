package sync

import (
	"fmt"
	"vault-sync/internal/config"
	"vault-sync/internal/core"

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

	appConfig, err := config.NewConfig()
	if err != nil {
		fmt.Printf("Error creating config: %v\n", err)
		return
	}

	wiring := core.NewWiring(appConfig)
	ctx := cmd.Context()
	orchestrator := wiring.InitOrchestrator(ctx)
	_, err = orchestrator.StartSync(ctx)
	if err != nil {
		fmt.Printf("Error during sync: %v\n", err)
		return
	}
	fmt.Println("Sync process completed successfully")
}
