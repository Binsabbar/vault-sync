package main

import "vault-sync/cmd"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

func main() {
	// Set version info before executing commands
	cmd.SetVersionInfo(version, commit, date, builtBy)
	cmd.Execute()
}
