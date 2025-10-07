package pathmatcher

import (
	"fmt"

	"github.com/spf13/cobra"
)

var PathMatcherCmd = &cobra.Command{
	Use:     "path-matcher",
	Short:   "start the path matching process",
	Long:    `it will start the path matching process`,
	Example: `vault-sync path-matcher --config /path/to/config.yaml`,
	Run:     run,
}

func run(cmd *cobra.Command, args []string) {
	// add option to check paths in a given file or from argument
	// add option to check paths directly from Vault instance using config
	// output should be in json format containing pathsToSync and PathsToIgnore root objects, and values as arrays of strings
	// the mount also must be included
	// Example:
	// [
	// 	{
	//    "mount": "secret"
	//    "pathsToSync": [
	//        "secret/data/team-a/app/config",
	//        "secret/data/team-b/app/config"
	//    ],
	//    "pathsToIgnore": [
	//        "secret/data/team-a/app/ignore",
	//        "secret/data/team-b/app/ignore"
	//    ],
	// 	}
	// ]

	fmt.Println("path-matcher called")
}
