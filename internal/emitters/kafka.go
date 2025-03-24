package emitters

import (
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"context"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// KafkaEmitter implements EventEmitter using Kafka
type KafkaEmitter struct {
	BrokerAddress string
	Topic         string
}

func (k *KafkaEmitter) EmitEvent(event models.TransactionEvent) error {
	w := &kafka.Writer{
		Addr:     kafka.TCP(k.BrokerAddress),
		Topic:    k.Topic,
		Balancer: &kafka.LeastBytes{},
	}
	defer func(w *kafka.Writer) {
		_ = w.Close()
	}(w)

	err := w.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(event.TxHash),
		Value: []byte(fmt.Sprintf("%+v", event)),
	})
	if err != nil {
		return fmt.Errorf("failed to write message to Kafka: %v", err)
	}

	logger.GetLogger().Info().
		Str("chain", event.Chain.String()).
		Str("txHash", event.TxHash).
		Msg("Successfully emitted event to Kafka")
	return nil
}

func (k *KafkaEmitter) Close() error {
	return nil
}
