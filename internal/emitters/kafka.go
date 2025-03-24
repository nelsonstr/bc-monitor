package emitters

import (
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/segmentio/kafka-go"
)

// KafkaEmitter implements EventEmitter using Kafka
type KafkaEmitter struct {
	writer *kafka.Writer
	mu     sync.Mutex
}

// NewKafkaEmitter creates a new KafkaEmitter
func NewKafkaEmitter(brokerAddress, topic string) *KafkaEmitter {
	return &KafkaEmitter{
		writer: &kafka.Writer{
			Addr:     kafka.TCP(brokerAddress),
			Topic:    topic,
			Balancer: &kafka.LeastBytes{},
		},
	}
}

func (k *KafkaEmitter) EmitEvent(event models.TransactionEvent) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %v", err)
	}

	err = k.writer.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(event.TxHash),
		Value: value,
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
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.writer != nil {
		err := k.writer.Close()
		k.writer = nil
		return err
	}
	return nil
}
