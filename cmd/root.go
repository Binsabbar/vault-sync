package cmd

import (
	"os"
	"strings"
	"vault-sync/cmd/configprint"
	"vault-sync/cmd/pathmatcher"
	"vault-sync/cmd/sync"
	"vault-sync/cmd/version"
	"vault-sync/pkg/log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

const (
	CFG_FLAG_NAME = "config"
)

var RootCmd = &cobra.Command{
	Use:   "vault-sync",
	Short: "Vault Sync will sync one Vault instance with multiple other Vault instances",
	Long: `Vault Sync is a tool that will sync Vault secrets from one instance to multiple
	other instances. It is useful for keeping secrets in sync across multiple regions, such 
	as main site and disaster recovery site.`,
}

func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func SetVersionInfo(v, c, d, b string) {
	version.SetVersionInfo(v, c, d, b)
}

func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVarP(&cfgFile, CFG_FLAG_NAME, "c", "", "path to config file")

	RootCmd.AddCommand(version.VersionCmd)
	RootCmd.Version = version.GetVersion()

	RootCmd.AddCommand(sync.SyncCmd)
	RootCmd.AddCommand(pathmatcher.PathMatcherCmd)
	RootCmd.AddCommand(configprint.ConfigPrintCmd)
}

func initConfig() {
	logger := log.Logger.With().Str("component", "rootCmd").Logger()
	logger.Info().Str("cfgFile", cfgFile).Msg("Initializing configuration")

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/vault-sync/")
		viper.AddConfigPath("$HOME/.vault-sync")
	}

	// Environment variable support
	viper.SetEnvPrefix("vault_sync")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	if err := viper.ReadInConfig(); err != nil {
		logger.Error().Err(err).Msg("Error loading config")
	}
}
