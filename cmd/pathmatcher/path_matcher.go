package pathmatcher

import (
	"bufio"
	"os"
	"strings"

	"vault-sync/internal/config"
	"vault-sync/internal/service/pathmatching"
	"vault-sync/pkg/log"

	"github.com/spf13/cobra"
)

var pathsFile string

var logger = log.Logger.With().Str("component", "path-matcher").Logger()

var PathMatcherCmd = &cobra.Command{
	Use:   "path-matcher",
	Short: "Test path matching patterns against a list of paths",
	Long: `Test your sync rules against a list of paths to see which ones would be synced.
    
The paths file should contain one path per line in the format: mount/path/to/secret
For example:
  production/app/database/password
  uat/api/keys/jwt
  stage/infra/certificates/ssl`,
	Run: runPathMatcher,
}

func init() {
	PathMatcherCmd.Flags().StringVarP(&pathsFile, "paths-file", "f", "", "File containing paths to test (one per line)")
	if err := PathMatcherCmd.MarkFlagRequired("paths-file"); err != nil {
		logger.Error().Err(err).Msg("Failed to mark paths-file as required")
		os.Exit(-1)
	}
}

func runPathMatcher(_ *cobra.Command, _ []string) {
	cfg, err := config.Load()

	if err != nil {
		logger.Error().Err(err).Msg("Failed to load config")
		os.Exit(-1)
	}

	matcher := pathmatching.NewCorePathMatcher(&cfg.SyncRule)

	// Read paths from file or stdin
	var paths []string
	paths, err = readPathsFromFile(pathsFile)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read paths file")
		os.Exit(-1)
	}

	if len(paths) == 0 {
		logger.Warn().Msg("No paths provided. Use --paths-file or pipe paths to stdin.")
		return
	}

	logger.Info().Int("path_count", len(paths)).Msg("Testing paths against sync rules")

	matched := 0
	ignored := 0

	for _, fullPath := range paths {
		fullPath = strings.TrimSpace(fullPath)
		if fullPath == "" {
			continue
		}

		pathNumOfParts := 2
		parts := strings.SplitN(fullPath, "/", pathNumOfParts)
		if len(parts) != pathNumOfParts {
			logger.Warn().Msgf("❌ INVALID (format should be mount/path): %s", fullPath)
			continue
		}

		mount := parts[0]
		keyPath := parts[1]

		shouldSync := matcher.ShouldSync(mount, keyPath)

		if shouldSync {
			logger.Info().Msgf("✅ MATCH:   %s", fullPath)
			matched++
		} else {
			logger.Info().Msgf("❌ IGNORE:  %s", fullPath)
			ignored++
		}
	}

	logger.Info().
		Int("matched", matched).
		Int("ignored", ignored).
		Int("total", matched+ignored).
		Msg("Process is completed")
}

func readPathsFromFile(filename string) ([]string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		logger.Error().Err(err).Str("file_name", filename).Msg("file does not exist")
		return nil, err
	}

	file, err := os.Open(filename)
	if err != nil {
		logger.Error().Err(err).Str("file_name", filename).Msg("failed to open file")
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logger.Error().Err(closeErr).Str("file_name", filename).Msg("failed to close file")
		}
	}()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			paths = append(paths, line)
		}
	}

	return paths, scanner.Err()
}
