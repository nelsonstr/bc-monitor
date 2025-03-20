package interfaces

import "blockchain-monitor/internal/models"

// EventEmitter defines the interface for emitting events
type EventEmitter interface {
	EmitEvent(event models.TransactionEvent) error
}
