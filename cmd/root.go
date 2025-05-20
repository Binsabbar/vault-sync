/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"
	"strings"
	"vault-sync/cmd/sync"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

const (
	CFG_FLAG_NAME = "config"
)

var RootCmd = &cobra.Command{
	Use:   "vault-sync",
	Short: "Vault Sync will sync two Vault instances together",
	Long: `Vault Sync is a tool that will sync Vault secrets from one instance to multiple other instances.
It is useful for keeping secrets in sync across multiple regions, such as main site and disater recovery site.`,
}

func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {

	RootCmd.PersistentFlags().StringVarP(&cfgFile, CFG_FLAG_NAME, "c", "", "path to config file")

	viper.BindPFlag(CFG_FLAG_NAME, RootCmd.PersistentFlags().Lookup(CFG_FLAG_NAME))
	viper.SetConfigName(cfgFile)
	viper.SetConfigType("yaml")
	viper.SetEnvPrefix("vault_sync")
	viper.AddConfigPath(".")                 // For running from project root
	viper.AddConfigPath("/etc/vault-sync/")  // For production
	viper.AddConfigPath("$HOME/.vault-sync") // For user-specific config

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	RootCmd.AddCommand(sync.SyncCmd)
}
