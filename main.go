package main

import (
	"fmt"
	"os"

	"github.com/Binsabbar/vault-sync/internal/config"
	"github.com/Binsabbar/vault-sync/internal/sync"
	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "vault-sync",
		Short: "A tool to synchronize secrets with HashiCorp Vault",
		Long:  "vault-sync is a CLI tool that helps synchronize secrets between different Vault instances or backup/restore Vault secrets.",
		Run:   runSync,
	}

	rootCmd.Flags().StringP("config", "c", "", "Path to configuration file")
	rootCmd.Flags().StringP("source", "s", "", "Source Vault address")
	rootCmd.Flags().StringP("target", "t", "", "Target Vault address")
	rootCmd.Flags().StringP("token", "", "", "Vault token")
	rootCmd.Flags().BoolP("dry-run", "d", false, "Perform a dry run without making changes")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runSync(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	source, _ := cmd.Flags().GetString("source")
	target, _ := cmd.Flags().GetString("target")
	token, _ := cmd.Flags().GetString("token")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cfg, err := config.Load(configPath, source, target, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	syncer, err := sync.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create syncer: %v\n", err)
		os.Exit(1)
	}

	if err := syncer.Sync(dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "Sync failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Sync completed successfully")
}