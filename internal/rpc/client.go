package rpc

import (
	"blockchain-monitor/internal/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// Client provides a standardized RPC client with rate limiting, retries, and structured logging
type Client struct {
	Endpoint     string
	ApiKey       string
	RateLimiter  *rate.Limiter
	MaxRetries   int
	RetryDelay   time.Duration
	HTTPTimeout  time.Duration
	Logger       *zerolog.Logger
	HTTPClient   *http.Client
}

// NewClient creates a new RPC client with the given configuration
func NewClient(endpoint, apiKey string, rateLimit float64, maxRetries int, retryDelay, httpTimeout time.Duration, logger *zerolog.Logger) *Client {
	return &Client{
		Endpoint:    endpoint,
		ApiKey:      apiKey,
		RateLimiter: rate.NewLimiter(rate.Limit(rateLimit), 1),
		MaxRetries:  maxRetries,
		RetryDelay:  retryDelay,
		HTTPTimeout: httpTimeout,
		Logger:      logger,
		HTTPClient: &http.Client{
			Timeout: httpTimeout,
			Transport: &CustomTransport{
				Base:   http.DefaultTransport,
				ApiKey: apiKey,
			},
		},
	}
}

// CustomTransport adds API key authentication to HTTP requests
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

// Call performs an RPC call with rate limiting, retries, and error handling
func (c *Client) Call(method string, params []interface{}) (*models.RPCResponse, error) {
	c.Logger.Debug().
		Str("endpoint", c.Endpoint).
		Str("method", method).
		Interface("params", params).
		Msg("Making RPC call")

	// Wait for rate limit
	if err := c.RateLimiter.Wait(context.Background()); err != nil {
		c.Logger.Error().Err(err).Msg("Rate limit error")
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

	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var response models.RPCResponse
	err = c.retry(func() error {
		resp, err := c.HTTPClient.Do(req)
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
		c.Logger.Error().
			Err(err).
			Str("method", method).
			Interface("params", params).
			Msg("RPC call failed")
		return nil, err
	}

	return &response, nil
}

// retry executes a function with retry logic
func (c *Client) retry(fn func() error) error {
	var err error
	for i := 0; i < c.MaxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(c.RetryDelay)
	}
	return err
}

// Close closes the HTTP client connections
func (c *Client) Close() {
	if c.HTTPClient != nil {
		c.HTTPClient.CloseIdleConnections()
	}
}