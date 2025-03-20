package events

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
)

// PrintEmitter wraps another emitter and prints DB storage values
type PrintEmitter struct {
	WrappedEmitter interfaces.EventEmitter
	Monitors       map[string]interfaces.BlockchainMonitor
}

// EmitEvent prints DB storage values and forwards to the wrapped emitter
func (d *PrintEmitter) EmitEvent(event models.TransactionEvent) error {
	// Print  storage values
	logger.Log.Info().
		Str("chain", event.Chain).
		Msg("DB STORAGE VALUES")

	logger.Log.Info().
		Str("chain", event.Chain).
		Str("from", event.From).
		Str("to", event.To).
		Str("amount", event.Amount).
		Str("fees", event.Fees).
		Str("txHash", event.TxHash).
		Time("timestamp", event.Timestamp).
		Msg("Transaction details")

	// Print chain-specific information
	if monitor, ok := d.Monitors[event.Chain]; ok {
		logger.Log.Info().
			Str("chain", event.Chain).
			Str("network", "mainnet").
			Str("explorer", monitor.GetExplorerURL(event.TxHash)).
			Msg("Chain-specific information")
	}

	// Forward to wrapped emitter
	if d.WrappedEmitter != nil {
		return d.WrappedEmitter.EmitEvent(event)
	}
	return nil
}
