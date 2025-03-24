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
	"time"
)

var _ interfaces.BlockchainMonitor = (*SolanaMonitor)(nil)

type SolanaMonitor struct {
	*monitors.BaseMonitor
	latestSlot    uint64
	wsConn        *websocket.Conn
	subscriptions map[string]int
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

func NewSolanaMonitor(baseMonitor *monitors.BaseMonitor) *SolanaMonitor {
	return &SolanaMonitor{
		BaseMonitor:   baseMonitor,
		subscriptions: make(map[string]int),
	}
}

func (s *SolanaMonitor) Start(ctx context.Context) error {

	if err := s.Initialize(); err != nil {
		s.Logger.Fatal().Err(err).Msg("Failed to initialize Solana monitor")

		return err
	}

	if err := s.StartMonitoring(ctx); err != nil {
		s.Logger.Fatal().Err(err).Msg("Failed to start Solana monitoring")

		return err
	}

	return nil
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

	slot, err := s.getLatestSlot()
	if err != nil {

		return fmt.Errorf("failed to connect to Solana RPC: %v", err)
	}

	s.latestSlot = slot

	s.Logger.Info().
		Uint64("lastestSlot", slot).
		Msg("Starting Solana monitoring")

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

	//go s.handleWebSocketMessages(ctx)
	return nil
}

func (s *SolanaMonitor) subscribeToAccount(address string) error {
	//s.Mu.Lock()
	//defer s.Mu.Unlock()

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

	if err := s.wsConn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to subscribe to account %s: %v", address, err)
	}

	s.Logger.Debug().Str("address", address).Msg("Subscribed to account")

	for {
		_, message, err := s.wsConn.ReadMessage()
		if err != nil {
			s.Logger.Printf("Read error: %v", err)
			continue
		}

		var change AccountChange
		if err := json.Unmarshal(message, &change); err != nil {
			s.Logger.Err(err).Msg("Decode error")
			continue
		}

		if change.Method == "accountNotification" {
			balance := float64(change.Params.Result.Value.Lamports) / 1e9

			s.Logger.Info().
				Any("value", balance).
				Msg("Address balance changed")
		}
	}

	return nil
}

func (s *SolanaMonitor) handleWebSocketMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var msg map[string]interface{}
			err := s.wsConn.ReadJSON(&msg)
			if err != nil {
				s.Logger.Error().Err(err).Msg("Error reading WebSocket message")
				continue
			}

			if msg["method"] == "accountNotification" {
				s.Logger.Info().Any("msg", msg).Msg("Received account notification")
				s.processAccountNotification2(msg)
			}
			s.Logger.Info().
				Any("method", msg["method"]).
				Msg("Processed WebSocket message")
		}
	}
}

func (s *SolanaMonitor) processAccountNotification2(msg map[string]interface{}) {
	params, ok := msg["params"].(map[string]interface{})
	if !ok {
		s.Logger.Error().Msg("Invalid account notification format")
		return
	}

	result, ok := params["result"].(map[string]interface{})
	if !ok {
		s.Logger.Error().Msg("Invalid result format in account notification")
		return
	}

	value, ok := result["value"].(map[string]interface{})
	if !ok {
		s.Logger.Error().Msg("Invalid value format in account notification")
		return
	}

	lamports, ok := value["lamports"].(float64)
	if !ok {
		s.Logger.Error().Msg("Invalid lamports format in account notification")
		return
	}

	address, ok := params["subscription"].(float64)
	if !ok {
		s.Logger.Error().Msg("Invalid subscription format in account notification")
		return
	}

	s.Logger.Info().
		Float64("lamports", lamports).
		Float64("balanceSOL", lamports/1e9).
		Float64("subscriptionID", address).
		Msg("Account balance update")

}

func (s *SolanaMonitor) processAccountNotification(msg string) {
	address := "string"
	var lastKnownBalance uint64

	var response struct {
		Value struct {
			Lamports uint64 `json:"lamports"`
		} `json:"value"`
	}

	if err := json.Unmarshal([]byte(msg), &response); err != nil {
		s.Logger.Error().Err(err).Msg("Error parsing response")
		return
	}

	currentBalance := response.Value.Lamports
	if currentBalance != lastKnownBalance {
		if lastKnownBalance > 0 {
			balanceChange := int64(currentBalance) - int64(lastKnownBalance)
			solAmount := float64(balanceChange) / 1e9

			txDetails, err := s.getRecentTransactionDetails(address)
			if err != nil {
				s.Logger.Error().Err(err).Msg("Error fetching transaction details")
			}

			if balanceChange > 0 {
				solAmount = -solAmount
			}

			event := models.TransactionEvent{
				From:        txDetails.From,
				To:          txDetails.To,
				Amount:      fmt.Sprintf("%.9f", solAmount),
				Fees:        fmt.Sprintf("%.9f", txDetails.Fees),
				Chain:       s.GetChainName(),
				TxHash:      txDetails.TxHash,
				Timestamp:   txDetails.Timestamp,
				ExplorerURL: s.GetExplorerURL(txDetails.TxHash),
			}

			s.Logger.Info().
				Str("chain", string(event.Chain)).
				Str("from", event.From).
				Str("to", event.To).
				Str("amount", event.Amount).
				Str("fees", event.Fees).
				Str("txHash", event.TxHash).
				Time("timestamp", event.Timestamp).
				Msg("STORAGE VALUES")

			if err := s.EventEmitter.EmitEvent(event); err != nil {
				s.Logger.Error().Err(err).Msg("Error emitting event")
				return
			}
		} else {
			s.Logger.Info().
				Str("address", address).
				Uint64("balance", currentBalance).
				Float64("balanceSOL", float64(currentBalance)/1e9).
				Msg("Initial balance")
		}

		lastKnownBalance = currentBalance
	}

}

func (s *SolanaMonitor) getRecentTransactionDetails(address string) (SolanaTransactionDetails, error) {
	resp, err := s.MakeRPCCall("getSignaturesForAddress", []interface{}{
		address,
		map[string]interface{}{
			"limit": 1,
		},
	})
	if err != nil {
		return SolanaTransactionDetails{}, err
	}

	var signaturesResp []struct {
		Signature string `json:"signature"`
		Slot      uint64 `json:"slot"`
		BlockTime int64  `json:"blockTime"`
	}
	if err := json.Unmarshal(resp.Result, &signaturesResp); err != nil {
		return SolanaTransactionDetails{}, err
	}

	if len(signaturesResp) == 0 {
		return SolanaTransactionDetails{}, fmt.Errorf("no recent transactions found")
	}

	txResp, err := s.MakeRPCCall("getTransaction", []interface{}{
		signaturesResp[0].Signature,
		map[string]interface{}{
			"encoding":                       "jsonParsed",
			"maxSupportedTransactionVersion": 0,
		},
	})
	if err != nil {
		return SolanaTransactionDetails{}, err
	}

	var txDetails TransactionDetailsRaw

	if err := json.Unmarshal(txResp.Result, &txDetails); err != nil {
		return SolanaTransactionDetails{}, fmt.Errorf("failed to parse transaction details: %w", err)
	}

	var from, to string
	var amount uint64

	actualFee := txDetails.Meta.Fee
	accountKeys := txDetails.Transaction.Message.AccountKeys
	preBalances := txDetails.Meta.PreBalances
	postBalances := txDetails.Meta.PostBalances

	if len(accountKeys) == 0 || len(preBalances) != len(accountKeys) || len(postBalances) != len(accountKeys) {
		return SolanaTransactionDetails{}, fmt.Errorf("invalid transaction details: account keys and balances mismatch")
	}

	// Extract sender (from)
	var senderIndex int
	for i, acc := range txDetails.Transaction.Message.AccountKeys {
		if acc.Signer && acc.Writable {
			from = acc.Pubkey
			senderIndex = i
			break
		}
	}
	if from == "" {
		return SolanaTransactionDetails{}, fmt.Errorf("sender not found in transaction")
	}
	var receiverIndex int
	for i, acc := range txDetails.Transaction.Message.AccountKeys {
		if acc.Writable && acc.Pubkey != from {
			to = acc.Pubkey
			receiverIndex = i
			break
		}
	}
	_ = receiverIndex

	// Calculate amount
	if len(txDetails.Meta.PreBalances) <= senderIndex || len(txDetails.Meta.PostBalances) <= senderIndex {
		return SolanaTransactionDetails{}, fmt.Errorf("invalid pre/post balances for sender")
	}

	preBalance := txDetails.Meta.PreBalances[senderIndex]
	postBalance := txDetails.Meta.PostBalances[senderIndex]
	amountLamports := preBalance - postBalance - txDetails.Meta.Fee // Subtract fee from the amount
	amount = amountLamports

	return SolanaTransactionDetails{
		From:      from,
		To:        to,
		Amount:    float64(amount) / 1e9,
		Fees:      float64(actualFee) / 1e9,
		TxHash:    signaturesResp[0].Signature,
		Timestamp: time.Unix(signaturesResp[0].BlockTime, 0),
	}, nil
}

func (s *SolanaMonitor) getLatestSlot() (uint64, error) {
	resp, err := s.MakeRPCCall("getSlot", nil)
	if err != nil {
		return 0, err
	}

	var slotResp uint64
	if err := json.Unmarshal(resp.Result, &slotResp); err != nil {
		return 0, err
	}

	return slotResp, nil
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

func (s *SolanaMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://solscan.io/tx/%s", txHash)
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

func (s *SolanaMonitor) Stop(_ context.Context) error {
	s.Logger.Info().Msg("Stopping solana monitor")

	s.CloseHTTPClient()

	if s.wsConn != nil {
		s.wsConn.Close()
	}

	return nil
}
