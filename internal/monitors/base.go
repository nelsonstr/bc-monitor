package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	maxRetries   int
	retryDelay   time.Duration
	rateLimiter  *rate.Limiter

	client *http.Client
}

func (s *BaseMonitor) makeRPCCall(method string, params []interface{}) (*models.RPCResponse, error) {
	// Wait for rate limit
	if err := s.rateLimiter.Wait(context.Background()); err != nil {
		logger.Log.Error().Err(err).Msg("Rate limit error")
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

	logger.Log.Debug().
		Str("method", method).
		Interface("params", params).
		Msg("Making RPC call")

	req, err := http.NewRequest("POST", s.RpcEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if s.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.ApiKey)
	}

	var response models.RPCResponse
	err = s.Retry(func() error {

		resp, err := s.client.Do(req)
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
		logger.Log.Error().
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
	for i := 0; i < b.maxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(b.retryDelay)
	}
	return err
}
