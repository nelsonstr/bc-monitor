package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"time"
)

// BaseMonitor contains common fields and methods for all blockchain monitors
type BaseMonitor struct {
	ApiKey       string
	RpcEndpoint  string
	EventEmitter interfaces.EventEmitter
	Addresses    []string
	MaxRetries   int
	RetryDelay   time.Duration
}
