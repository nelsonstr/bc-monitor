package main

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/monitors"
	"github.com/joho/godotenv"
	"log"
	"os"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}
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

	//for chainName, monitor := range printEmitter.Monitors {
	//	log.Printf("Starting monitoring for %s", chainName)
	//	time.After(time.Second * 5)
	//
	//	go func(m interfaces.BlockchainMonitor) {
	//		if err := m.Start(printEmitter); err != nil {
	//			log.Printf("Error starting monitoring for %s: %v", m.GetChainName(), err)
	//		}
	//	}(monitor)
	//}

	// Start monitoring
	//go ethereumMonitor.Start(printEmitter)
	//go solanaMonitor.Start(printEmitter)
	go bitcoinMonitor.Start(printEmitter)

	// Keep the main goroutine running
	select {}
}
