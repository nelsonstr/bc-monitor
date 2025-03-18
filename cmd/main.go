package main

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"github.com/joho/godotenv"
	"log"
	"os"
	"time"
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
	ethereumMonitor := createEthereumMonitor()
	bitcoinMonitor := createBitcoinMonitor()
	solanaMonitor := createSolanaMonitor()

	// Create a custom emitter that wraps the Kafka emitter and prints DB values
	dbPrintEmitter := &DbPrintEmitter{
		wrappedEmitter: kafkaEmitter,
		monitors: map[string]interfaces.BlockchainMonitor{
			"Ethereum": ethereumMonitor,
			"Bitcoin":  bitcoinMonitor,
			"Solana":   solanaMonitor,
		},
	}

	// Start monitoring
	go ethereum(dbPrintEmitter, ethereumMonitor)
	//go solana(dbPrintEmitter, solanaMonitor)
	//go bitcoin(dbPrintEmitter, bitcoinMonitor)
	// Keep the main goroutine running
	select {}
}

// DbPrintEmitter wraps another emitter and prints DB storage values
type DbPrintEmitter struct {
	wrappedEmitter interfaces.EventEmitter
	monitors       map[string]interfaces.BlockchainMonitor
}

// EmitEvent prints DB storage values and forwards to the wrapped emitter
func (d *DbPrintEmitter) EmitEvent(event models.TransactionEvent) error {
	// Print DB storage values
	log.Printf("DB STORAGE VALUES (%s):", event.Chain)
	log.Printf("  Chain:       %s", event.Chain)
	log.Printf("  From:      %s", event.From)
	log.Printf("  To: %s", event.To)
	log.Printf("  Amount:      %s", event.Amount)
	log.Printf("  Fees:        %s", event.Fees)
	log.Printf("  TxHash:      %s", event.TxHash)
	log.Printf("  Timestamp:   %s", event.Timestamp.Format(time.RFC3339))

	// Print Ethereum-specific information if applicable
	// Print chain-specific information
	if monitor, ok := d.monitors[event.Chain]; ok {
		log.Printf("  Network:     %s mainnet", event.Chain)
		log.Printf("  Explorer:    %s", monitor.GetExplorerURL(event.TxHash))
	}

	// Forward to wrapped emitter
	if d.wrappedEmitter != nil {
		return d.wrappedEmitter.EmitEvent(event)
	}
	return nil

}

func createSolanaMonitor() *monitors.SolanaMonitor {
	return &monitors.SolanaMonitor{
		RpcEndpoint: os.Getenv("SOLANA_RPC_ENDPOINT"),
		ApiKey:      os.Getenv("BLOCKDAEMON_API_KEY"),
		Addresses:   []string{"oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7"},
	}
}

func solana(emitter interfaces.EventEmitter, monitor *monitors.SolanaMonitor) {
	monitor.EventEmitter = emitter
	// Initialize Solana monitor
	if err := monitor.Initialize(); err != nil {
		log.Fatalf("Failed to initialize Solana monitor: %v", err)
	}
	// Start monitoring Solana blockchain
	if err := monitor.StartMonitoring(); err != nil {
		log.Fatalf("Failed to start Solana monitoring: %v", err)
	}
}

func createEthereumMonitor() *monitors.EthereumMonitor {
	return &monitors.EthereumMonitor{
		RpcEndpoint: os.Getenv("ETHEREUM_RPC_ENDPOINT"),
		ApiKey:      os.Getenv("BLOCKDAEMON_API_KEY"),
		Addresses:   []string{"0x00000000219ab540356cBB839Cbe05303d7705Fa"},
		MaxRetries:  5,
		RetryDelay:  5 * time.Second,
	}
}

func ethereum(emitter interfaces.EventEmitter, monitor *monitors.EthereumMonitor) {
	// Set the EventEmitter
	monitor.EventEmitter = emitter

	// Initialize Ethereum monitor
	if err := monitor.Initialize(); err != nil {
		log.Fatalf("Failed to initialize Ethereum monitor: %v", err)
	}
	// Start monitoring Ethereum blockchain
	if err := monitor.StartMonitoring(); err != nil {
		log.Fatalf("Failed to start Ethereum monitoring: %v", err)
	}
}

func createBitcoinMonitor() *monitors.BitcoinMonitor {
	return &monitors.BitcoinMonitor{
		RpcEndpoint: os.Getenv("BITCOIN_RPC_ENDPOINT"),
		ApiKey:      os.Getenv("BLOCKDAEMON_API_KEY"),
		Addresses:   []string{"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
		MaxRetries:  2,
		RetryDelay:  2 * time.Second,
	}
}

func bitcoin(emitter interfaces.EventEmitter, monitor *monitors.BitcoinMonitor) {
	monitor.EventEmitter = emitter
	// Initialize Bitcoin monitor
	if err := monitor.Initialize(); err != nil {
		log.Fatalf("Failed to initialize Bitcoin monitor: %v", err)
	}
	// Start monitoring Bitcoin blockchain
	if err := monitor.StartMonitoring(); err != nil {
		log.Fatalf("Failed to start Bitcoin monitoring: %v", err)
	}
}
