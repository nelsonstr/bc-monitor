package emitters

import (
	"blockchain-monitor/internal/config"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaEmitter implements EventEmitter using Kafka
type KafkaEmitter struct {
	writer    *kafka.Writer
	eventChan chan models.TransactionEvent
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
}

// NewKafkaEmitter creates a new KafkaEmitter
func NewKafkaEmitter(kafkaCfg config.KafkaConfig) *KafkaEmitter {
	ctx, cancel := context.WithCancel(context.Background())
	emitter := &KafkaEmitter{
		writer: &kafka.Writer{
			Addr:     kafka.TCP(kafkaCfg.BrokerAddress),
			Topic:    kafkaCfg.Topic,
			Balancer: &kafka.LeastBytes{},
		},
		eventChan: make(chan models.TransactionEvent, 1000), // buffer size
		ctx:       ctx,
		cancel:    cancel,
	}

	emitter.wg.Add(1)
	go emitter.worker(kafkaCfg)

	return emitter
}

func (k *KafkaEmitter) worker(kafkaCfg config.KafkaConfig) {
	defer k.wg.Done()

	ticker := time.NewTicker(kafkaCfg.BatchTimeout)
	defer ticker.Stop()

	var batch []models.TransactionEvent

	for {
		select {
		case event := <-k.eventChan:
			batch = append(batch, event)
			if len(batch) >= kafkaCfg.BatchSize {
				k.flushBatch(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				k.flushBatch(batch)
				batch = nil
			}
		case <-k.ctx.Done():
			// Flush remaining events
			if len(batch) > 0 {
				k.flushBatch(batch)
			}
			return
		}
	}
}

func (k *KafkaEmitter) flushBatch(batch []models.TransactionEvent) {
	k.mu.Lock()
	defer k.mu.Unlock()

	messages := make([]kafka.Message, 0, len(batch))
	for _, event := range batch {
		value, err := json.Marshal(event)
		if err != nil {
			logger.GetLogger().Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to marshal event")
			continue
		}
		messages = append(messages, kafka.Message{
			Key:   []byte(event.TxHash),
			Value: value,
		})
	}

	if len(messages) > 0 {
		err := k.writer.WriteMessages(k.ctx, messages...)
		if err != nil {
			logger.GetLogger().Error().Err(err).Int("batchSize", len(messages)).Msg("Failed to write batch to Kafka")
		} else {
			logger.GetLogger().Info().Int("batchSize", len(messages)).Msg("Successfully emitted batch to Kafka")
		}
	}
}

func (k *KafkaEmitter) EmitEvent(event models.TransactionEvent) error {
	select {
	case k.eventChan <- event:
		return nil
	case <-k.ctx.Done():
		return fmt.Errorf("emitter is closed")
	default:
		// Channel is full, drop the event or handle differently
		logger.GetLogger().Warn().Str("txHash", event.TxHash).Msg("Event channel full, dropping event")
		return nil
	}
}

func (k *KafkaEmitter) Close() error {
	k.cancel()
	k.wg.Wait()

	k.mu.Lock()
	defer k.mu.Unlock()

	if k.writer != nil {
		err := k.writer.Close()
		k.writer = nil
		return err
	}
	return nil
}
