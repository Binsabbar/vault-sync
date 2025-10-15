package log

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
)

var Logger zerolog.Logger

func Init(app_id string, levelStr string) {
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

	Logger = zerolog.New(os.Stdout).With().Timestamp().Str("service", "vault-sync").Str("app_id", app_id).Logger()
}

func init() {
	Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	Logger = Logger.With().Str("service", "vault-sync").Logger()
	Logger.Info().Str("log_level", "info").Msg("Logger initialized")

}
