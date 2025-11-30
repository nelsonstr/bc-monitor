package solana

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var _ interfaces.BlockchainMonitor = (*SolanaMonitor)(nil)

// Update the SolanaMonitor struct
type SolanaMonitor struct {
	*monitors.BaseMonitor
	wsMu              *sync.Mutex
	latestBlockHeight uint64
	wsConn            *websocket.Conn
	subscriptions     map[string]int
	balances          map[string]*big.Int
}

// Update the NewSolanaMonitor function
func NewSolanaMonitor(baseMonitor *monitors.BaseMonitor) *SolanaMonitor {
	return &SolanaMonitor{
		BaseMonitor:   baseMonitor,
		subscriptions: make(map[string]int),
		balances:      make(map[string]*big.Int),
		wsMu:          &sync.Mutex{},
	}
}

// Update the Start method
func (s *SolanaMonitor) Start(ctx context.Context) error {
	if err := s.Initialize(); err != nil {
		s.Logger.Error().Err(err).Msg("Failed to initialize Solana monitor")
		return err
	}

	return s.StartMonitoring(ctx)
}

func (s *SolanaMonitor) Initialize() error {
	wsURL := os.Getenv("SOLANA_WSS_ENDPOINT")

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

	s.latestBlockHeight = blockHead

	s.Logger.Info().
		Uint64("latestBlockHeight", s.latestBlockHeight).
		Msg("Starting Solana monitoring")

	return nil
}

func (s *SolanaMonitor) AddAddress(address string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Check if the address is already being monitored
	for _, addr := range s.Addresses {
		if addr == address {
			return nil
		}
	}

	// Add the new address
	s.Addresses = append(s.Addresses, address)

	// set current balance for the new address
	balance, err := s.GetAccountBalance(address)
	if err != nil {
		s.Logger.Error().Err(err).Str("address", address).Msg("Failed to get initial balance for address")
		return err
	}

	s.balances[address] = new(big.Int).SetUint64(balance)

	// Subscribe to the new address
	if err := s.subscribeToAccount(address); err != nil {
		s.Logger.Error().Err(err).Str("address", address).Msg("Failed to subscribe to new address")
		return err
	}

	s.Logger.Info().
		Str("address", address).
		Uint64("balance", balance).
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

	_, message, err := s.wsConn.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read subscription response for %s: %v", address, err)
	}

	var resp struct {
		Jsonrpc string `json:"jsonrpc"`
		Result  int    `json:"result"`
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
			time.Sleep(time.Second)
			message, err := s.readMessage()
			if err != nil {
				s.Logger.Error().Err(err).Msg("Error reading WebSocket message")
				continue
			}

			var change AccountChange
			if err := json.Unmarshal(message, &change); err != nil {
				s.Logger.Error().Err(err).Msg("Decode error")
				continue
			}

			if change.Method == "accountNotification" {
				s.Logger.Debug().
					Uint64("Lamports", change.Params.Result.Value.Lamports).
					Msg("Received account notification")
				s.processAccountChange(change)
			} else {
				s.Logger.Error().
					Str("method", change.Method).
					Msg("Unknown method")
			}
		}
	}
}

func (s *SolanaMonitor) readMessage() ([]byte, error) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()

	_, message, err := s.wsConn.ReadMessage()

	return message, err
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

	newBalance := new(big.Int).SetUint64(change.Params.Result.Value.Lamports)
	oldBalance, exists := s.balances[address]

	if !exists {
		s.balances[address] = newBalance
		s.Logger.Info().
			Str("address", address).
			Str("balance", newBalance.String()).
			Msg("Initial balance")

	}

	if newBalance.Cmp(oldBalance) != 0 {
		balanceChange := new(big.Int).Sub(newBalance, oldBalance)
		s.balances[address] = newBalance
		txDetails, err := s.getRecentTransactionDetails(address, change.Params.Context.Slot)
		if err != nil {
			s.Logger.Error().Err(err).Msg("Error fetching transaction details")
		}

		solAmount := new(big.Float).SetInt(balanceChange)
		solAmount.Quo(solAmount, big.NewFloat(1e9)) // Convert lamports to SOL

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
		}
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

func (s *SolanaMonitor) getRecentTransactionDetails(address string, slot uint64) (SolanaTransactionDetails, error) {
	resp, err := s.MakeRPCCall("getSignaturesForAddress", []interface{}{
		address,
		map[string]interface{}{
			"limit":          10,
			"minContextSlot": slot,
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

	// filter signaturesResp where slot
	var slotSignaturesIndex int
	for i, s := range signaturesResp {
		if s.Slot == slot {
			slotSignaturesIndex = i
			break
		}
	}

	txResp, err := s.MakeRPCCall("getTransaction", []interface{}{
		signaturesResp[slotSignaturesIndex].Signature,
		map[string]interface{}{
			"encoding":                       "jsonParsed",
			"maxSupportedTransactionVersion": 0,
		},
	})
	if err != nil {
		return SolanaTransactionDetails{}, err
	}

	var txDetails struct {
		Meta        Meta `json:"meta"`
		Transaction struct {
			Message struct {
				AccountKeys []struct {
					Pubkey   string `json:"pubkey"`
					Signer   bool   `json:"signer"`
					Source   string `json:"source"`
					Writable bool   `json:"writable"`
				} `json:"accountKeys"`
				Instructions []struct {
					ProgramIdIndex int      `json:"programIdIndex"`
					Accounts       []string `json:"accounts"`
					Data           string   `json:"data"`
				} `json:"instructions"`
			} `json:"message"`
		} `json:"transaction"`
	}

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

func (s *SolanaMonitor) GetAccountBalance(address string) (uint64, error) {
	rpcResponse, err := s.MakeRPCCall("getAccountInfo", []interface{}{
		address,
		map[string]interface{}{
			"encoding":   "jsonParsed",
			"commitment": "finalized",
		},
	})
	if err != nil {
		s.Logger.Error().Err(err).Msg("Error making RPC call")
		return 0, err
	}

	var response struct {
		Value struct {
			Lamports uint64 `json:"lamports"`
		} `json:"value"`
	}
	if err := json.Unmarshal(rpcResponse.Result, &response); err != nil {
		return 0, fmt.Errorf("failed to parse account balance: %w", err)
	}

	return response.Value.Lamports, nil
}
