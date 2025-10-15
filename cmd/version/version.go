package version

import (
	"fmt"

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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("vault-sync version %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built at: %s\n", date)
		fmt.Printf("  built by: %s\n", builtBy)
	},
}
