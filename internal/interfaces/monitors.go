package interfaces

import (
	"blockchain-monitor/internal/models"
	"context"
)

// BlockchainMonitor defines the interface for blockchain monitoring
type BlockchainMonitor interface {
	Start(ctx context.Context) error
	// Initialize sets up the blockchain client
	Initialize() error

	// StartMonitoring begins monitoring for transactions involving specified addresses
	StartMonitoring(ctx context.Context) error

	// GetChainName returns the name of the blockchain
	GetChainName() models.BlockchainName

	GetExplorerURL(txHash string) string
	GetBlockHead() (uint64, error)
	AddAddress(addr string) error
	Stop(ctx context.Context) error
}
