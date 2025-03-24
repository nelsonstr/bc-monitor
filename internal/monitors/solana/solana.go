package solana

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var _ interfaces.BlockchainMonitor = (*SolanaMonitor)(nil)

type SolanaMonitor struct {
	*monitors.BaseMonitor
	latestSlot uint64
}

func NewSolanaMonitor(baseMonitor *monitors.BaseMonitor) *SolanaMonitor {
	return &SolanaMonitor{
		BaseMonitor: baseMonitor,
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

	var wg sync.WaitGroup
	for _, address := range s.Addresses {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			s.pollAccountChanges(ctx, addr)
		}(address)
	}

	wg.Wait()
	return nil
}

func (s *SolanaMonitor) pollAccountChanges(ctx context.Context, address string) {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	var lastKnownBalance uint64

	for {
		select {
		case <-ctx.Done():
			s.Logger.Info().Msg("Stopping account polling")
			return
		default:
			rpcResponse, err := s.MakeRPCCall("getAccountInfo", []interface{}{
				address,
				map[string]interface{}{
					"encoding":   "jsonParsed",
					"commitment": "confirmed",
				},
			})

			if err != nil {
				s.Logger.Error().Err(err).Msg("Error making RPC call")
				time.Sleep(backoff)
				backoff = minDuration(backoff*2, maxBackoff)
				continue
			}

			var response struct {
				Value struct {
					Lamports uint64 `json:"lamports"`
				} `json:"value"`
			}

			if err := json.Unmarshal(rpcResponse.Result, &response); err != nil {
				s.Logger.Error().Err(err).Msg("Error parsing response")
				time.Sleep(backoff)
				backoff = minDuration(backoff*2, maxBackoff)
				continue
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

			backoff = 1 * time.Second
			time.Sleep(5 * time.Second)
		}
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
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

func (s *SolanaMonitor) processTransaction(tx SolanaTransaction, watchAddresses map[string]bool) {
	accounts := tx.Transaction.Message.AccountKeys

	for i, account := range accounts {
		if watchAddresses[account] {
			var source, destination string
			var amount string

			if i == 0 {
				source = account
				if len(accounts) > 1 {
					destination = accounts[1]
				}

				if len(tx.Meta.PreBalances) > 0 && len(tx.Meta.PostBalances) > 0 {
					preBalance := tx.Meta.PreBalances[i]
					postBalance := tx.Meta.PostBalances[i]

					if preBalance > postBalance {
						amountLamports := preBalance - postBalance - tx.Meta.Fee
						amount = convertLaports2Sol(amountLamports)
					}
				}

				s.Logger.Info().
					Str("address", account).
					Msg("Detected outgoing transaction from watched address")
			} else {
				destination = account
				source = accounts[0]

				if len(tx.Meta.PreBalances) > i && len(tx.Meta.PostBalances) > i {
					preBalance := tx.Meta.PreBalances[i]
					postBalance := tx.Meta.PostBalances[i]
					if postBalance > preBalance {
						amountLamports := postBalance - preBalance
						amount = convertLaports2Sol(amountLamports)
					}
				}

				s.Logger.Info().
					Str("address", account).
					Msg("Detected incoming transaction to watched address")
			}

			event := models.TransactionEvent{
				From:        source,
				To:          destination,
				Amount:      amount,
				Fees:        convertLaports2Sol(tx.Meta.Fee),
				Chain:       s.GetChainName(),
				TxHash:      tx.Transaction.Signatures[0],
				Timestamp:   time.Unix(tx.BlockTime, 0),
				ExplorerURL: s.GetExplorerURL(tx.Transaction.Signatures[0]),
			}

			if err := s.EventEmitter.EmitEvent(event); err != nil {
				s.Logger.Error().Err(err).Msg("Error emitting event")
			}
			break
		}
	}
}

func convertLaports2Sol(amountLamports uint64) string {
	return fmt.Sprintf("%.9f", float64(amountLamports)/1e9)
}

func (s *SolanaMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://solscan.io/tx/%s", txHash)
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

	// Start monitoring the new address
	go s.pollAccountChanges(context.Background(), address)

	s.Logger.Info().
		Str("address", address).
		Msg("Added new Solana address to monitoring")

	return nil
}

func (s *SolanaMonitor) Stop(_ context.Context) error {
	s.Logger.Info().Msg("Stopping solana monitor")

	// Close the HTTP client
	s.CloseHTTPClient()

	return nil
}
