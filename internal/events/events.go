package events

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"log"
	"time"
)

// PrintEmitter wraps another emitter and prints DB storage values
type PrintEmitter struct {
	WrappedEmitter interfaces.EventEmitter
	Monitors       map[string]interfaces.BlockchainMonitor
}

// EmitEvent prints DB storage values and forwards to the wrapped emitter
func (d *PrintEmitter) EmitEvent(event models.TransactionEvent) error {
	// Print DB storage values
	log.Printf("DB STORAGE VALUES (%s):", event.Chain)
	log.Printf("  Chain:       %s", event.Chain)
	log.Printf("  From:      %s", event.From)
	log.Printf("  To: %s", event.To)
	log.Printf("  Amount:      %s", event.Amount)
	log.Printf("  Fees:        %s", event.Fees)
	log.Printf("  TxHash:      %s", event.TxHash)
	log.Printf("  Timestamp:   %s", event.Timestamp.Format(time.RFC3339))

	// Print Start-specific information if applicable
	// Print chain-specific information
	if monitor, ok := d.Monitors[event.Chain]; ok {
		log.Printf("  Network:     %s mainnet", event.Chain)
		log.Printf("  Explorer:    %s", monitor.GetExplorerURL(event.TxHash))
	}

	// Forward to wrapped emitter
	if d.WrappedEmitter != nil {
		return d.WrappedEmitter.EmitEvent(event)
	}
	return nil
}
