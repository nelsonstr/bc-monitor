package events

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"github.com/rs/zerolog"
)

// PrintEmitter wraps another emitter and prints DB storage values
type PrintEmitter struct {
	WrappedEmitter interfaces.EventEmitter
	logger         *zerolog.Logger
}

// create a new PrintEmitter
func EventsGateway(logger *zerolog.Logger, wrappedEmitter *emitters.KafkaEmitter) *PrintEmitter {

	return &PrintEmitter{
		WrappedEmitter: wrappedEmitter,
		logger:         logger,
	}
}

// EmitEvent prints DB storage values and forwards to the wrapped emitter
func (d *PrintEmitter) EmitEvent(event models.TransactionEvent) error {
	// Print  storage values
	d.logger.Info().
		Str("chain", string(event.Chain)).
		Msg("STORAGE VALUES")

	d.logger.Info().
		Str("chain", string(event.Chain)).
		Str("source", event.From).
		Str("destination", event.To).
		Str("amount", event.Amount).
		Str("fees", event.Fees).
		Time("timestamp", event.Timestamp).
		Str("explorer", event.ExplorerURL).
		Msg("Transaction details -->")

	// Forward to wrapped emitter
	if d.WrappedEmitter != nil {
		return d.WrappedEmitter.EmitEvent(event)
	}
	return nil
}
