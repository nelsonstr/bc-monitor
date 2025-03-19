package emitters

import (
	"blockchain-monitor/internal/models"
	"context"
	"fmt"
	"log"

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
	defer w.Close()

	err := w.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(event.TxHash),
		Value: []byte(fmt.Sprintf("%+v", event)),
	})
	if err != nil {
		return fmt.Errorf("failed to write message to Kafka: %v", err)
	}

	log.Printf("Successfully emitted %s event to Kafka: %s", event.Chain, event.TxHash)
	return nil
}

func (k *KafkaEmitter) Close() error {
	return nil
}
