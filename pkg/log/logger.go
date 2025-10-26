package log

import (
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

//nolint:gochecknoglobals
var Logger zerolog.Logger

func Init(appID string, levelStr string) {
	var logLevel zerolog.Level

	switch strings.ToLower(levelStr) {
	case "trace":
		logLevel = zerolog.TraceLevel
	case "debug":
		logLevel = zerolog.DebugLevel
	case "info":
		logLevel = zerolog.InfoLevel
	case "warn":
		logLevel = zerolog.WarnLevel
	case "error":
		logLevel = zerolog.ErrorLevel
	case "fatal":
		logLevel = zerolog.FatalLevel
	case "panic":
		logLevel = zerolog.PanicLevel
	default:
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)

	Logger = zerolog.New(os.Stdout).With().Timestamp().Str("service", "vault-sync").Str("app_id", appID).Logger()
}

//nolint:gochecknoinits
func init() {
	if isTestSilentMode() {
		Logger = zerolog.New(io.Discard)
		zerolog.SetGlobalLevel(zerolog.Disabled)
	} else {
		Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		Logger = Logger.With().Str("service", "vault-sync").Logger()
		Logger.Info().Str("log_level", "info").Msg("Logger initialized")
	}
}

func isTestSilentMode() bool {
	if isTestMode() &&
		(os.Getenv("TEST_SILENT") == "1" || os.Getenv("TEST_SILENT") == "true") {
		return true
	}

	return false
}

func isTestMode() bool {
	for _, arg := range os.Args {
		if strings.Contains(arg, "test") || strings.HasSuffix(arg, ".test") {
			return true
		}
	}
	return false
}
