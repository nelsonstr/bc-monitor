package main

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/health"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"blockchain-monitor/internal/monitors/bitcoin"
	"blockchain-monitor/internal/monitors/evm"
	"blockchain-monitor/internal/monitors/solana"
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	logger.Init(os.Getenv("LOG_LEVEL"))

	if err := godotenv.Load(); err != nil {
		logger.GetLogger().Fatal().Err(err).Msg("Error loading .env file")
	}

	// Start HTTP server for metrics and health checks
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", health.LivenessHandler)
		mux.HandleFunc("/readyz", health.ReadinessHandler)

		server := &http.Server{
			Addr:    ":8888",
			Handler: mux,
		}

		logger.GetLogger().Info().Msg("Starting HTTP server on :8888")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.GetLogger().Error().Err(err).Msg("Error starting HTTP server")
		}
	}()

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Kafka emitter
	kafkaEmitter := emitters.NewKafkaEmitter(os.Getenv("KAFKA_BROKER_ADDRESS"), os.Getenv("KAFKA_TOPIC"))

	gatewayEmitter := events.EventsGateway(
		logger.GetLogger(),
		kafkaEmitter,
	)

	rateLimit := getRateLimit()

	btcBaseMonitor := monitors.NewBaseMonitor(
		models.Bitcoin,
		rateLimit,
		os.Getenv("BITCOIN_RPC_ENDPOINT"),
		os.Getenv("BITCOIN_API_KEY"),
		logger.GetLogger(),
		gatewayEmitter,
	)

	// Create monitors
	bitcoinMonitor := bitcoin.NewBitcoinMonitor(btcBaseMonitor)

	ethBaseMonitor := monitors.NewBaseMonitor(
		models.Ethereum,
		rateLimit, os.Getenv("ETHEREUM_RPC_ENDPOINT"),
		os.Getenv("ETHEREUM_API_KEY"),
		logger.GetLogger(),
		gatewayEmitter)
	ethereumMonitor := evm.NewEthereumMonitor(ethBaseMonitor)

	solBaseMonitor := monitors.NewBaseMonitor(
		models.Solana,
		rateLimit,
		os.Getenv("SOLANA_RPC_ENDPOINT"),
		os.Getenv("SOLANA_API_KEY"),
		logger.GetLogger(),
		gatewayEmitter)
	solanaMonitor := solana.NewSolanaMonitor(solBaseMonitor)

	bcMonitors := map[models.BlockchainName]interfaces.BlockchainMonitor{
		ethereumMonitor.GetChainName(): ethereumMonitor,
		bitcoinMonitor.GetChainName():  bitcoinMonitor,
		solanaMonitor.GetChainName():   solanaMonitor,
	}

	// WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Start monitoring for each blockchain
	for _, m := range bcMonitors {

		if err := m.Start(ctx); err != nil {
			logger.GetLogger().
				Error().
				Err(err).
				Str("chain", m.GetChainName().String()).
				Msg("Error starting monitoring")
		}
		health.RegisterMonitor(ctx, m)
	}

	_ = bitcoinMonitor
	_ = ethereumMonitor
	_ = solanaMonitor

	addAddressesToMonitor(bcMonitors)

	health.SetReady(true)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for interrupt signal
	<-sigChan
	logger.GetLogger().Info().
		Msg("Received shutdown signal. Initiating graceful shutdown...")

	// Cancel the context to signal all goroutines to stop
	cancel()

	// Stop all monitors
	for _, monitor := range bcMonitors {
		if err := monitor.Stop(ctx); err != nil {
			logger.GetLogger().Error().Err(err).Str("chain", monitor.GetChainName().String()).Msg("Error stopping monitor")
		}
	}

	waitChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		logger.GetLogger().Info().Msg("All monitors have shut down gracefully")
	case <-time.After(10 * time.Second):
		logger.GetLogger().Warn().Msg("Shutdown timed out after 30 seconds")
	}

	// Before shutting down, set the application as not ready
	health.SetReady(false)

	// Perform any necessary cleanup
	if err := gatewayEmitter.Close(); err != nil {
		logger.GetLogger().Error().Err(err).Msg("Error closing Kafka emitter")
	}

	logger.GetLogger().Info().Msg("Shutdown complete")
}

func getRateLimit() float64 {
	rlRaw := os.Getenv("RATE_LIMIT")
	rateLimit, err := strconv.Atoi(rlRaw)
	if err != nil || rateLimit <= 0 {
		rateLimit = 4
	}
	logger.GetLogger().Info().
		Int("rateLimit", rateLimit).
		Msg("Rate limit set")

	return float64(rateLimit)
}
