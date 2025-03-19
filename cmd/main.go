package main

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/monitors"
	"context"
	"github.com/joho/godotenv"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	logger.Init("info") // Initialize zerolog

	if err := godotenv.Load(); err != nil {
		logger.Log.Fatal().Err(err).Msg("Error loading .env file")
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Kafka emitter
	kafkaEmitter := &emitters.KafkaEmitter{
		BrokerAddress: os.Getenv("KAFKA_BROKER_ADDRESS"),
		Topic:         os.Getenv("KAFKA_TOPIC"),
	}

	// Create monitors
	ethereumMonitor := monitors.NewEthereumMonitor()
	bitcoinMonitor := monitors.NewBitcoinMonitor()
	solanaMonitor := monitors.NewSolanaMonitor()

	// Create a custom emitter that wraps the Kafka emitter and prints DB values
	printEmitter := &events.PrintEmitter{
		WrappedEmitter: kafkaEmitter,
		Monitors: map[string]interfaces.BlockchainMonitor{
			ethereumMonitor.GetChainName(): ethereumMonitor,
			bitcoinMonitor.GetChainName():  bitcoinMonitor,
			solanaMonitor.GetChainName():   solanaMonitor,
		},
	}

	// WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Start monitoring for each blockchain
	for _, monitor := range printEmitter.Monitors {
		wg.Add(1)

		go func(m interfaces.BlockchainMonitor) {
			defer wg.Done()
			if err := m.Start(ctx, printEmitter); err != nil {
				logger.Log.Error().Err(err).Str("chain", m.GetChainName()).Msg("Error starting monitoring")
			}
		}(monitor)
	}

	_ = bitcoinMonitor
	_ = ethereumMonitor
	_ = solanaMonitor

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for interrupt signal
	<-sigChan
	logger.Log.Info().Msg("Received shutdown signal. Initiating graceful shutdown...")

	// Cancel the context to signal all goroutines to stop
	cancel()

	// Wait for all goroutines to finish with a timeout
	waitChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		logger.Log.Info().Msg("All monitors have shut down gracefully")
	case <-time.After(30 * time.Second):
		logger.Log.Warn().Msg("Shutdown timed out after 30 seconds")
	}

	// Perform any necessary cleanup
	if err := kafkaEmitter.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("Error closing Kafka emitter")
	}

	logger.Log.Info().Msg("Shutdown complete")
}
