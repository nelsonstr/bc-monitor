package events

import (
	"blockchain-monitor/internal/emitters"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"

	"github.com/rs/zerolog"
)

var _ interfaces.EventEmitter = (*GatewayEmitter)(nil)

// GatewayEmitter wraps another emitter and prints DB storage values
type GatewayEmitter struct {
	WrappedEmitter interfaces.EventEmitter
	logger         *zerolog.Logger
}

func EventsGateway(logger *zerolog.Logger, wrappedEmitter *emitters.KafkaEmitter) *GatewayEmitter {
	return &GatewayEmitter{
		WrappedEmitter: wrappedEmitter,
		logger:         logger,
	}
}

// EmitEvent prints DB storage values and forwards to the wrapped emitter
func (d *GatewayEmitter) EmitEvent(event models.TransactionEvent) error {
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

	if d.WrappedEmitter != nil {
		return d.WrappedEmitter.EmitEvent(event)
	}

	return nil
}

func (d *GatewayEmitter) Close() error {
	return d.WrappedEmitter.Close()
}
