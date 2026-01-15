package monitors

import (
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// BaseMonitor contains common fields and methods for all blockchain monitors
type BaseMonitor struct {
	Client          *http.Client
	RpcEndpoint     string
	ApiKey          string
	ExplorerBaseURL string
	EventEmitter    interfaces.EventEmitter
	Addresses       []string
	Mu              sync.RWMutex
	Logger          *zerolog.Logger
	BlockchainName  models.BlockchainName
	RateLimiter     *rate.Limiter
	MaxRetries      int
	RetryDelay      time.Duration
}

// NewBaseMonitor creates a new BaseMonitor with the given parameters
func NewBaseMonitor(blockchain models.BlockchainName, rateLimit float64, rpcEndpoint, apiKey, explorerBaseURL string, logger *zerolog.Logger, emitter *events.GatewayEmitter) *BaseMonitor {
	return &BaseMonitor{
		Logger:          logger,
		Addresses:       []string{},
		RpcEndpoint:     rpcEndpoint,
		ApiKey:          apiKey,
		ExplorerBaseURL: explorerBaseURL,
		RateLimiter:     rate.NewLimiter(rate.Limit(rateLimit), 1),
		MaxRetries:      3,
		RetryDelay:      time.Second,
		EventEmitter:    emitter,
		BlockchainName:  blockchain,
	}
}

// GetChainName returns the blockchain name
func (b *BaseMonitor) GetChainName() models.BlockchainName {
	return b.BlockchainName
}

type CustomTransport struct {
	Base   http.RoundTripper
	ApiKey string
}

func (t *CustomTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Content-Type", "application/json")
	if t.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.ApiKey)
	}
	return t.Base.RoundTrip(req)
}

// MakeRPCCall performs an RPC call to the blockchain node with rate limiting and retries
func (s *BaseMonitor) MakeRPCCall(method string, params []interface{}) (*models.RPCResponse, error) {
	s.Logger.Debug().
		Str("url", s.RpcEndpoint).
		Str("method", method).
		Interface("params", params).
		Msg("Making RPC call")

	// Wait for rate limit
	if err := s.RateLimiter.Wait(context.Background()); err != nil {
		s.Logger.Error().Err(err).Msg("Rate limit error")
		return nil, fmt.Errorf("rate limit error: %v", err)
	}

	request := models.RPCRequest{
		Jsonrpc: "2.0",
		ID:      "1",
		Method:  method,
		Params:  params,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	var response models.RPCResponse
	err = s.Retry(func() error {
		// Create a new request for each retry attempt
		// HTTP requests cannot be reused after first use
		req, err := http.NewRequest("POST", s.RpcEndpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}

		resp, err := s.Client.Do(req)
		if err != nil {
			return err
		}
		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(resp.Body)

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, resp.Status)
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return fmt.Errorf("failed to decode response: %v", err)
		}

		if response.Error != nil {
			return fmt.Errorf("RPC error: %d - %s", response.Error.Code, response.Error.Message)
		}
		return nil
	})
	if err != nil {
		s.Logger.Error().
			Err(err).
			Str("blockchain", s.BlockchainName.String()).
			Str("method", method).
			Interface("params", params).
			Msg("RPC call failed")
		return nil, err
	}

	return &response, nil
}

// Retry executes a function with exponential backoff retry logic up to MaxRetries times
func (b *BaseMonitor) Retry(fn func() error) error {
	var err error
	delay := b.RetryDelay
	for i := 0; i < b.MaxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < b.MaxRetries-1 { // Don't sleep after last attempt
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		}
	}
	return err
}

func (b *BaseMonitor) CloseHTTPClient() {
	if b.Client != nil {
		b.Client.CloseIdleConnections()
	}
}

// AddAddress adds an address to the list of watched addresses if not already present
func (b *BaseMonitor) AddAddress(address string) error {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	for _, watchedAddr := range b.Addresses {
		if watchedAddr == address {
			return nil // Address is already being monitored
		}
	}

	b.Addresses = append(b.Addresses, address)

	return nil
}

// IsWatchedAddress checks if an address is in the list of watched addresses
func (b *BaseMonitor) IsWatchedAddress(address string) bool {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	for _, watchedAddr := range b.Addresses {
		if watchedAddr == address {
			return true
		}
	}

	return false
}
