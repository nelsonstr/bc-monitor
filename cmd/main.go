package main

import (
	"blockchain-monitor/internal/config"
	"blockchain-monitor/internal/database"
	"blockchain-monitor/internal/logger"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Error().Interface("panic", r).Msg("Application panicked, recovering")
			// Could add cleanup here
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		logger.GetLogger().Fatal().Err(err).Msg("Failed to load configuration")
	}

	logger.Init(cfg.LogLevel)

	if err := database.InitDB(cfg.Database); err != nil {
		logger.GetLogger().Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer database.Close()

	if err := database.RunMigrations(cfg.Database); err != nil {
		logger.GetLogger().Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Keep the application running or start the server here
	// For now, just block or exit as it seems to be a monitor
	select {}
}
