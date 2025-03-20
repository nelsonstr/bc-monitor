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

	// Open a file for logging
	logFile, err := os.OpenFile("log.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open log file")
	}

	// Create a MultiLevelWriter that writes to both console and file
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	multi := zerolog.MultiLevelWriter(consoleWriter, logFile)

	// Set the global logger to write to both console and file
	log.Logger = zerolog.New(multi).With().Timestamp().Logger()

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
