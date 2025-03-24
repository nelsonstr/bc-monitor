package solana

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
	"time"
)

var _ interfaces.BlockchainMonitor = (*SolanaMonitor)(nil)

// Update the SolanaMonitor struct
type SolanaMonitor struct {
	*monitors.BaseMonitor
	wsMu          sync.Mutex
	blockHead     uint64
	wsConn        *websocket.Conn
	subscriptions map[string]int // Changed from map[string]int to map[string]int
	balances      map[string]uint64
}
type AccountChange struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Result struct {
			Value struct {
				Lamports uint64 `json:"lamports"`
			} `json:"value"`
		} `json:"result"`
		Subscription uint64 `json:"subscription"`
	} `json:"params"`
}

// Update the NewSolanaMonitor function
func NewSolanaMonitor(baseMonitor *monitors.BaseMonitor) *SolanaMonitor {
	return &SolanaMonitor{
		BaseMonitor:   baseMonitor,
		subscriptions: make(map[string]int),
		balances:      make(map[string]uint64),
		wsMu:          sync.Mutex{},
	}
}

// Update the Start method
func (s *SolanaMonitor) Start(ctx context.Context) error {
	if err := s.Initialize(); err != nil {
		s.Logger.Fatal().Err(err).Msg("Failed to initialize Solana monitor")
		return err
	}

	return s.StartMonitoring(ctx)
}

func (s *SolanaMonitor) Initialize() error {
	wsURL := fmt.Sprintf("wss://solana-mainnet.core.chainstack.com/c4fd316535159fd103cc0dad2a971ab5")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	c, _, err := dialer.Dial(wsURL, nil)
	if err != nil {

		return fmt.Errorf("failed to connect to Solana WebSocket: %v", err)
	}

	s.wsConn = c

	s.Client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &monitors.CustomTransport{
			Base:   http.DefaultTransport,
			ApiKey: s.ApiKey,
		},
	}

	blockHead, err := s.GetBlockHead()
	if err != nil {

		return fmt.Errorf("failed to connect to Solana RPC: %v", err)
	}

	s.blockHead = blockHead

	s.Logger.Info().
		Uint64("blockHead", s.blockHead).
		Msg("Starting Solana monitoring")

	return nil
}

// Update the AddAddress method
func (s *SolanaMonitor) AddAddress(address string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Check if the address is already being monitored
	for _, addr := range s.Addresses {
		if addr == address {
			return nil // Address is already being monitored
		}
	}

	// Add the new address
	s.Addresses = append(s.Addresses, address)

	// Subscribe to the new address
	if err := s.subscribeToAccount(address); err != nil {
		s.Logger.Error().Err(err).Str("address", address).Msg("Failed to subscribe to new address")
		return err
	}

	s.Logger.Info().
		Str("address", address).
		Msg("Added new Solana address to monitoring")

	return nil
}

func (s *SolanaMonitor) StartMonitoring(ctx context.Context) error {
	s.Logger.Info().
		Int("addressCount", len(s.Addresses)).
		Msg("Starting Solana monitoring")

	for _, address := range s.Addresses {
		if err := s.subscribeToAccount(address); err != nil {
			s.Logger.Error().Err(err).Str("address", address).Msg("Failed to subscribe to account")
		}
	}

	go s.handleWebSocketMessages(ctx)

	return nil
}

func (s *SolanaMonitor) subscribeToAccount(address string) error {
	subscribeMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "accountSubscribe",
		"params": []interface{}{
			address,
			map[string]string{
				"encoding":   "jsonParsed",
				"commitment": "finalized",
			},
		},
	}

	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	if err := s.wsConn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to subscribe to account %s: %v", address, err)
	}

	// lê a resposta da subscrição
	_, message, err := s.wsConn.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read subscription response for %s: %v", address, err)
	}

	var resp struct {
		Jsonrpc string `json:"jsonrpc"`
		Result  int    `json:"result"` // <- subscriptionID
		ID      int    `json:"id"`
	}

	if err := json.Unmarshal(message, &resp); err != nil {
		return fmt.Errorf("failed to parse subscription response: %v", err)
	}

	s.subscriptions[address] = resp.Result

	s.Logger.Info().
		Str("address", address).
		Int("subscriptionID", resp.Result).
		Msg("Subscribed to account")

	return nil
}

// Update the handleWebSocketMessages method
func (s *SolanaMonitor) handleWebSocketMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.wsMu.Lock()

			_, message, err := s.wsConn.ReadMessage()
			if err != nil {
				s.Logger.Error().Err(err).Msg("Error reading WebSocket message")
				continue
			}
			s.wsMu.Unlock()

			var change AccountChange
			if err := json.Unmarshal(message, &change); err != nil {
				s.Logger.Error().Err(err).Msg("Decode error")
				continue
			}

			if change.Method == "accountNotification" {
				s.processAccountChange(change)
			} else {
				s.Logger.Error().Str("method", change.Method).Msg("Unknown method")
			}
		}
	}
}

// Add a new method to process account changes
func (s *SolanaMonitor) processAccountChange(change AccountChange) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	address := s.findAddressBySubscription(int(change.Params.Subscription))
	if address == "" {
		s.Logger.Error().Int("subscription", int(change.Params.Subscription)).Msg("Unknown subscription")
		return
	}

	newBalance := change.Params.Result.Value.Lamports
	oldBalance, exists := s.balances[address]

	if !exists {
		s.balances[address] = newBalance
		s.Logger.Info().
			Str("address", address).
			Uint64("balance", newBalance).
			Msg("Initial balance")
		return
	}

	if newBalance != oldBalance {
		balanceChange := int64(newBalance) - int64(oldBalance)
		s.balances[address] = newBalance

		event := models.TransactionEvent{
			From:   address,
			To:     "Unknown", // You might want to fetch this information
			Amount: fmt.Sprintf("%.9f", float64(balanceChange)/1e9),
			Chain:  s.GetChainName(),
			// Set other fields as needed
		}

		if err := s.EventEmitter.EmitEvent(event); err != nil {
			s.Logger.Error().Err(err).Msg("Error emitting event")
		}

		s.Logger.Info().
			Str("address", address).
			Int64("change", balanceChange).
			Uint64("newBalance", newBalance).
			Msg("Balance changed")
	}
}

// Add a helper method to find address by subscription ID
func (s *SolanaMonitor) findAddressBySubscription(subscriptionID int) string {
	for address, subscription := range s.subscriptions {
		if subscription == subscriptionID {
			return address
		}
	}
	return ""
}

// Update the Stop method
func (s *SolanaMonitor) Stop(_ context.Context) error {
	s.Logger.Info().Msg("Stopping Solana monitor")

	s.CloseHTTPClient()
	if s.wsConn != nil {
		if err := s.wsConn.Close(); err != nil {
			s.Logger.Error().Err(err).Msg("Error closing WebSocket connection")
			return err
		}
	}
	return nil
}

func (s *SolanaMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://solscan.io/tx/%s", txHash)
}

func (s *SolanaMonitor) getBlock(slot uint64) ([]SolanaTransaction, error) {
	resp, err := s.MakeRPCCall("getBlock", []interface{}{
		slot,
		map[string]interface{}{
			"encoding":                       "json",
			"maxSupportedTransactionVersion": 0,
			"transactionDetails":             "full",
			"rewards":                        false,
		},
	})
	if err != nil {
		return nil, err
	}

	var blockResp struct {
		Transactions []SolanaTransaction `json:"transactions"`
	}
	if err := json.Unmarshal(resp.Result, &blockResp); err != nil {
		return nil, err
	}

	return blockResp.Transactions, nil
}

func (s *SolanaMonitor) GetBlockHead() (uint64, error) {
	resp, err := s.MakeRPCCall("getSlot", nil)
	if err != nil {
		return 0, err
	}

	var block uint64
	if err := json.Unmarshal(resp.Result, &block); err != nil {
		return 0, err
	}

	return block, nil
}
