package logger

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"os"
	"time"
)

var logger zerolog.Logger

func Init(level string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	// Set the global logger
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Set the log level
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	logger = log.With().Caller().Logger()
}

func GetLogger() *zerolog.Logger {
	return &logger
}
