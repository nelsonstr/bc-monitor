package main

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/monitors"
	"context"
	"github.com/joho/godotenv"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
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
			//bitcoinMonitor.GetChainName(): bitcoinMonitor,
			//solanaMonitor.GetChainName(): solanaMonitor,
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
				log.Printf("Error starting monitoring for %s: %v", m.GetChainName(), err)
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
	log.Println("Received shutdown signal. Initiating graceful shutdown...")

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
		log.Println("All monitors have shut down gracefully")
	case <-time.After(30 * time.Second):
		log.Println("Shutdown timed out after 30 seconds")
	}

	// Perform any necessary cleanup
	if err := kafkaEmitter.Close(); err != nil {
		log.Printf("Error closing Kafka emitter: %v", err)
	}

	log.Println("Shutdown complete")
}
