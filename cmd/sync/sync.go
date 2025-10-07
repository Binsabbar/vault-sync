package sync

import (
	"fmt"

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
	fmt.Println("sync called")
}
