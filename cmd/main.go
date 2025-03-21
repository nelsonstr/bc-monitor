package main

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/monitors"
	"blockchain-monitor/internal/monitors/bitcoin"
	"blockchain-monitor/internal/monitors/evm"
	"blockchain-monitor/internal/monitors/solana"
	"context"
	"github.com/joho/godotenv"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func main() {
	logger.Init("info") // Initialize zerolog

	if err := godotenv.Load(); err != nil {
		logger.GetLogger().Fatal().Err(err).Msg("Error loading .env file")
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Kafka emitter
	kafkaEmitter := &emitters.KafkaEmitter{
		BrokerAddress: os.Getenv("KAFKA_BROKER_ADDRESS"),
		Topic:         os.Getenv("KAFKA_TOPIC"),
	}
	printEmitter := events.NewPrintEmitter(
		logger.GetLogger(),
		kafkaEmitter,
	)

	rateLimit := getRateLimit()

	btcBaseMonitor := monitors.NewBaseMonitor(
		[]string{"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
		rateLimit,
		os.Getenv("BITCOIN_RPC_ENDPOINT"),
		os.Getenv("BITCOIN_API_KEY"),
		logger.GetLogger(),
		printEmitter,
	)

	// Create monitors
	bitcoinMonitor := bitcoin.NewBitcoinMonitor(*btcBaseMonitor)

	ethBaseMonitor := monitors.NewBaseMonitor(
		[]string{"0x00000000219ab540356cBB839Cbe05303d7705Fa"},
		rateLimit, os.Getenv("ETHEREUM_RPC_ENDPOINT"),
		os.Getenv("ETHEREUM_API_KEY"),
		logger.GetLogger(),
		printEmitter)
	ethereumMonitor := evm.NewEthereumMonitor(*ethBaseMonitor)

	solBaseMonitor := monitors.NewBaseMonitor(
		[]string{"5guD4Uz462GT4Y4gEuqyGsHZ59JGxFN4a3rF6KWguMcJ",
			"oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7"},
		rateLimit,
		os.Getenv("SOLANA_RPC_ENDPOINT"),
		os.Getenv("SOLANA_API_KEY"),
		logger.GetLogger(),
		printEmitter)
	solanaMonitor := solana.NewSolanaMonitor(*solBaseMonitor)

	monitors := map[string]interfaces.BlockchainMonitor{
		ethereumMonitor.GetChainName(): ethereumMonitor,
		bitcoinMonitor.GetChainName():  bitcoinMonitor,
		solanaMonitor.GetChainName():   solanaMonitor,
	}

	// WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Start monitoring for each blockchain
	for _, monitor := range monitors {
		wg.Add(1)

		go func(m interfaces.BlockchainMonitor) {
			defer wg.Done()
			if err := m.Start(ctx); err != nil {
				logger.GetLogger().Error().Err(err).Str("chain", m.GetChainName()).Msg("Error starting monitoring")
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
	logger.GetLogger().Info().Msg("Received shutdown signal. Initiating graceful shutdown...")

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
		logger.GetLogger().Info().Msg("All monitors have shut down gracefully")
	case <-time.After(30 * time.Second):
		logger.GetLogger().Warn().Msg("Shutdown timed out after 30 seconds")
	}

	// Perform any necessary cleanup
	if err := kafkaEmitter.Close(); err != nil {
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
