package interfaces

import (
	"blockchain-monitor/internal/models"
	"context"
)

// BlockchainMonitor defines the interface for blockchain monitoring
type BlockchainMonitor interface {
	Start(ctx context.Context, emitter EventEmitter) error
	// Initialize sets up the blockchain client
	Initialize() error

	// StartMonitoring begins monitoring for transactions involving specified addresses
	StartMonitoring() error

	// GetChainName returns the name of the blockchain
	GetChainName() string

	GetExplorerURL(txHash string) string
}

// EventEmitter defines the interface for emitting events
type EventEmitter interface {
	EmitEvent(event models.TransactionEvent) error
}
