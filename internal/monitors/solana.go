package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	"net/http"
	"os"
	"strconv"
	"time"
)

// SolanaMonitor implements BlockchainMonitor for Solana
type SolanaMonitor struct {
	BaseMonitor
	latestSlot uint64
}

type SolanaRpcRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type SolanaSlotResponse struct {
	Context struct {
		Slot uint64 `json:"slot"`
	} `json:"context"`
	Value uint64 `json:"value"`
}

type SolanaTransaction struct {
	BlockTime   int64  `json:"blockTime"`
	Meta        Meta   `json:"meta"`
	Slot        uint64 `json:"slot"`
	Transaction struct {
		Message struct {
			AccountKeys  []string `json:"accountKeys"`
			Instructions []struct {
				ProgramIdIndex int    `json:"programIdIndex"`
				Accounts       []int  `json:"accounts"`
				Data           string `json:"data"`
			} `json:"instructions"`
		} `json:"message"`
		Signatures []string `json:"signatures"`
	} `json:"transaction"`
}

type Meta struct {
	Fee          uint64   `json:"fee"`
	PreBalances  []uint64 `json:"preBalances"`
	PostBalances []uint64 `json:"postBalances"`
}

var _ interfaces.BlockchainMonitor = (*SolanaMonitor)(nil)

func NewSolanaMonitor(log *zerolog.Logger) *SolanaMonitor {
	rlRaw := os.Getenv("RATE_LIMIT")
	rateLimit, err := strconv.Atoi(rlRaw)
	if err != nil || rateLimit <= 0 {
		rateLimit = 4
	}
	log.Info().
		Int("rateLimit", rateLimit).
		Msg("Rate limit set")

	return &SolanaMonitor{
		BaseMonitor: BaseMonitor{
			RpcEndpoint: os.Getenv("SOLANA_RPC_ENDPOINT"),
			ApiKey:      os.Getenv("SOLANA_API_KEY"),
			Addresses:   []string{"oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7"},
			maxRetries:  1,
			retryDelay:  2 * time.Second,
			rateLimiter: rate.NewLimiter(rate.Limit(rateLimit), 1),
			logger:      log,
		},
	}
}

func (s *SolanaMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info().Msg("Solana monitor shutting down")
			return nil
		default:
			s.EventEmitter = emitter

			if err := s.Initialize(); err != nil {
				s.logger.Fatal().Err(err).Msg("Failed to initialize Solana monitor")
				return err
			}
			if err := s.StartMonitoring(); err != nil {
				s.logger.Fatal().Err(err).Msg("Failed to start Solana monitoring")
				return err
			}
			return nil
		}
	}
}

func (s *SolanaMonitor) Initialize() error {
	s.BaseMonitor.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &customTransport{
			base:   http.DefaultTransport,
			apiKey: s.ApiKey,
		},
	}

	// todo get blockhead

	// Test connection
	slot, err := s.getLatestSlot()
	if err != nil {
		return fmt.Errorf("failed to connect to Solana RPC: %v", err)
	}

	s.latestSlot = slot
	s.logger.Info().
		Int("addressCount", len(s.Addresses)).
		Msg("Starting Solana monitoring")
	return nil
}

func (s *SolanaMonitor) GetChainName() string {
	return "Solana"
}

func (s *SolanaMonitor) StartMonitoring() error {
	s.logger.Info().
		Int("addressCount", len(s.Addresses)).
		Msg("Starting Solana monitoring")

	// Start polling for each address
	for _, address := range s.Addresses {
		go s.pollAccountChanges(address)
	}

	return nil
}

func (s *SolanaMonitor) pollAccountChanges(address string) {
	// Initial backoff time
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	// Keep track of last known balance to detect changes
	var lastKnownBalance uint64

	for {
		var rpcResponse *models.RPCResponse
		var err error
		rpcResponse, err = s.makeRPCCall("getAccountInfo", []interface{}{
			address,
			map[string]interface{}{
				"encoding":   "jsonParsed",
				"commitment": "confirmed",
			},
		})

		if err != nil {
			s.logger.Error().
				Err(err).
				Msg("Error making RPC call")

			time.Sleep(backoff)
			backoff = minDuration(backoff*2, maxBackoff)

			continue
		}

		// Parse response
		var response struct {
			Value struct {
				Lamports uint64 `json:"lamports"`
			} `json:"value"`
		}

		if err := json.Unmarshal(rpcResponse.Result, &response); err != nil {
			s.logger.Error().
				Err(err).
				Msg("Error parsing response")

			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)

			continue
		}

		// Check if balance changed
		currentBalance := response.Value.Lamports
		if currentBalance != lastKnownBalance {
			if lastKnownBalance > 0 {
				balanceChange := int64(currentBalance) - int64(lastKnownBalance)
				solAmount := float64(balanceChange) / 1000000000 // Convert lamports to SOL

				// Fetch recent transactions to get details
				txDetails, err := s.getRecentTransactionDetails(address)
				if err != nil {
					s.logger.Error().
						Err(err).
						Msg("Error fetching transaction details")
				}

				// Determine if it's incoming or outgoing
				var source, destination string
				if balanceChange > 0 {
					source = "unknown"
					destination = address
				} else {
					source = address
					destination = "unknown"
					solAmount = -solAmount // Make amount positive for display
				}

				// Create transaction event
				event := models.TransactionEvent{
					From:      source,
					To:        destination,
					Amount:    fmt.Sprintf("%.9f", solAmount),
					Fees:      fmt.Sprintf("%.9f", txDetails.Fees),
					Chain:     s.GetChainName(),
					TxHash:    txDetails.TxHash,
					Timestamp: txDetails.Timestamp,
				}

				// Print storage values
				s.logger.Info().
					Str("chain", event.Chain).
					Str("from", event.From).
					Str("to", event.To).
					Str("amount", event.Amount).
					Str("fees", event.Fees).
					Str("txHash", event.TxHash).
					Time("timestamp", event.Timestamp).
					Msg("STORAGE VALUES")

				s.EventEmitter.EmitEvent(event)
			} else {
				s.logger.Info().
					Str("address", address).
					Uint64("balance", currentBalance).
					Float64("balanceSOL", float64(currentBalance)/1000000000).
					Msg("Initial balance")
			}

			lastKnownBalance = currentBalance
		}

		// Reset backoff on successful request
		backoff = 1 * time.Second

		// Wait before polling again
		time.Sleep(5 * time.Second)
	}
}

// Helper function for backoff calculation
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (s *SolanaMonitor) getLatestSlot() (uint64, error) {
	resp, err := s.makeRPCCall("getSlot", nil)
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

	resp, err := s.makeRPCCall("getBlock", []interface{}{
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
	// Extract accounts involved in the transaction
	accounts := tx.Transaction.Message.AccountKeys

	// Check if any of the accounts are in our watch list
	for i, account := range accounts {
		if watchAddresses[account] {
			// Determine if this is a sender or receiver
			// In Solana, typically the first account is the fee payer/sender
			var source, destination string
			var amount string

			// Simplified logic - in reality, you'd need to analyze the instruction data
			// to determine the actual transfer details
			if i == 0 {
				// This account is likely the sender
				source = account

				// Find potential destination (this is simplified)
				if len(accounts) > 1 {
					destination = accounts[1]
				}

				// Calculate amount from balance changes
				if len(tx.Meta.PreBalances) > 0 && len(tx.Meta.PostBalances) > 0 {
					preBalance := tx.Meta.PreBalances[i]
					postBalance := tx.Meta.PostBalances[i]

					if preBalance > postBalance {
						// Convert from lamports to SOL (1 SOL = 10^9 lamports)
						amountLamports := preBalance - postBalance - tx.Meta.Fee
						amount = convertLaports2Sol(amountLamports)
					}
				}

				s.logger.Info().
					Str("address", account).
					Msg("Detected outgoing transaction from watched address")
			} else {
				// This account is likely a receiver
				destination = account
				source = accounts[0]

				// Calculate amount from balance changes
				if len(tx.Meta.PreBalances) > i && len(tx.Meta.PostBalances) > i {
					preBalance := tx.Meta.PreBalances[i]
					postBalance := tx.Meta.PostBalances[i]
					if postBalance > preBalance {
						// Convert from lamports to SOL
						amountLamports := postBalance - preBalance
						amount = convertLaports2Sol(amountLamports)
					}
				}

				s.logger.Info().
					Str("address", account).
					Msg("Detected incoming transaction to watched address")
			}

			// Create and emit event
			event := models.TransactionEvent{
				From:      source,
				To:        destination,
				Amount:    amount,
				Fees:      convertLaports2Sol(tx.Meta.Fee),
				Chain:     s.GetChainName(),
				TxHash:    tx.Transaction.Signatures[0],
				Timestamp: time.Unix(tx.BlockTime, 0),
			}

			s.EventEmitter.EmitEvent(event)
			break
		}
	}
}

func convertLaports2Sol(amountLamports uint64) string {
	return fmt.Sprintf("%.9f", float64(amountLamports)/1000000000)
}

func (s *SolanaMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://explorer.solana.com/tx/%s", txHash)
}

func (s *SolanaMonitor) getRecentTransactionDetails(address string) (TransactionDetails, error) {

	resp, err := s.makeRPCCall("getSignaturesForAddress", []interface{}{
		address,
		map[string]interface{}{
			"limit": 1,
		},
	})
	if err != nil {
		return TransactionDetails{}, err
	}

	var signaturesResp []struct {
		Signature string `json:"signature"`
		Slot      uint64 `json:"slot"`
		BlockTime int64  `json:"blockTime"`
	}
	if err := json.Unmarshal(resp.Result, &signaturesResp); err != nil {
		return TransactionDetails{}, err
	}

	if len(signaturesResp) == 0 {
		return TransactionDetails{}, fmt.Errorf("no recent transactions found")
	}

	txResp, err := s.makeRPCCall("getTransaction", []interface{}{
		signaturesResp[0].Signature,
		map[string]interface{}{
			"encoding": "jsonParsed",
		},
	})
	if err != nil {
		return TransactionDetails{}, err
	}

	var txDetails struct {
		Result struct {
			Meta struct {
				Fee uint64 `json:"fee"`
			} `json:"meta"`
		} `json:"result"`
	}
	if err := json.Unmarshal(txResp.Result, &txDetails); err != nil {
		return TransactionDetails{}, err
	}

	return TransactionDetails{
		Fees:      float64(txDetails.Result.Meta.Fee) / 1000000000,
		TxHash:    signaturesResp[0].Signature,
		Timestamp: time.Unix(signaturesResp[0].BlockTime, 0),
	}, nil
}
