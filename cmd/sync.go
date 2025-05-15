/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// syncCmd represents the sync command
var syncCmd = &cobra.Command{
	Use:     "sync",
	Short:   "start the sync process until it is stopped",
	Long:    `it will start the sync process and keep it running until it is stopped`,
	Example: `vault-sync sync --config /path/to/config.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("sync called")
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
