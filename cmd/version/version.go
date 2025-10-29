package version

import (
	"fmt"

	"vault-sync/pkg/log"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func SetVersionInfo(v, c, d, b string) {
	version = v
	commit = c
	date = d
	builtBy = b
}

func GetVersion() string {
	return fmt.Sprintf("%s (commit: %s, date: %s)", version, commit, date)
}

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(_ *cobra.Command, _ []string) {
		logger := log.Logger.With().Str("component", "version").Logger()
		logger.Info().
			Str("commit", commit).
			Str("built_at", date).
			Str("built_by", builtBy).
			Msg("vault-sync version information")
	},
}
