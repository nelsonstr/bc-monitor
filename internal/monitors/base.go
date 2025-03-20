package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
	"net/http"
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
	RateLimiter  *rate.Limiter

	Logger *zerolog.Logger

	Client *http.Client
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

func NewBaseMonitor(addresses []string, rpcClient *http.Client, logger *zerolog.Logger) *BaseMonitor {
	return &BaseMonitor{
		Logger:    logger,
		Addresses: addresses,
		Client:    rpcClient,
	}
}

func (s *BaseMonitor) MakeRPCCall(method string, params []interface{}) (*models.RPCResponse, error) {
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

	s.Logger.Debug().
		Str("method", method).
		Interface("params", params).
		Msg("Making RPC call")

	req, err := http.NewRequest("POST", s.RpcEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var response models.RPCResponse
	err = s.Retry(func() error {

		resp, err := s.Client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

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
			Str("method", method).
			Interface("params", params).
			Msg("RPC call failed")
		return nil, err
	}

	return &response, nil
}

func (b *BaseMonitor) Retry(fn func() error) error {
	var err error
	for i := 0; i < b.MaxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(b.RetryDelay)
	}
	return err
}
