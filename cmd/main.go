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
