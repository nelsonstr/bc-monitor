package main

import (
	"blockchain-monitor/internal/config"
	"blockchain-monitor/internal/database"
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"blockchain-monitor/internal/monitors/bitcoin"
	"blockchain-monitor/internal/monitors/evm"
	"blockchain-monitor/internal/monitors/solana"
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger().Error().Interface("panic", r).Msg("Application panicked, recovering")
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		logger.GetLogger().Fatal().Err(err).Msg("Failed to load configuration")
	}

	logger.Init(cfg.LogLevel)
	log := logger.GetLogger()

	if err := database.InitDB(cfg.Database); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer database.Close()

	if err := database.RunMigrations(cfg.Database); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Initialize Kafka Emitter
	emitter, err := events.NewGatewayEmitter(cfg.Kafka)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Kafka emitter")
	}
	defer emitter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Monitors
	monitorMap := make(map[models.BlockchainName]interfaces.BlockchainMonitor)

	for bcName, bcfg := range cfg.Chains {
		base := monitors.NewBaseMonitor(
			bcName,
			bcfg.RateLimit,
			bcfg.RpcEndpoint,
			bcfg.ApiKey,
			bcfg.ExplorerBaseURL,
			log,
			emitter,
		)

		var monitor interfaces.BlockchainMonitor
		switch bcName {
		case models.Bitcoin:
			monitor = bitcoin.NewBitcoinMonitor(base)
		case models.Ethereum:
			monitor = evm.NewEthereumMonitor(base)
		case models.Solana:
			monitor = solana.NewSolanaMonitor(base)
		default:
			log.Warn().Str("blockchain", bcName.String()).Msg("Unsupported blockchain, skipping")
			continue
		}

		monitorMap[bcName] = monitor
		if err := monitor.Start(ctx); err != nil {
			log.Error().Err(err).Str("blockchain", bcName.String()).Msg("Failed to start monitor")
		}
	}

	// Add addresses to monitor (initially from hardcoded list, later from DB)
	addAddressesToMonitor(monitorMap)

	log.Info().Msg("Blockchain monitor service started")

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	case <-ctx.Done():
		log.Info().Msg("Context cancelled")
	}

	log.Info().Msg("Shutting down...")
	cancel()

	// Gracefully stop all monitors
	for bcName, monitor := range monitorMap {
		if err := monitor.Stop(context.Background()); err != nil {
			log.Error().Err(err).Str("blockchain", bcName.String()).Msg("Error stopping monitor")
		}
	}

	log.Info().Msg("Shutdown complete")
}
