package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// SolanaMonitor implements BlockchainMonitor for Solana
type SolanaMonitor struct {
	BaseMonitor
	client     *http.Client
	latestSlot uint64
}

type SolanaRpcRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type SolanaRpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
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

func (s *SolanaMonitor) Initialize() error {
	s.client = &http.Client{
		Timeout: 30 * time.Second,
	}

	// Test connection
	slot, err := s.getLatestSlot()
	if err != nil {
		return fmt.Errorf("failed to connect to Solana RPC: %v", err)
	}

	s.latestSlot = slot
	log.Printf("Successfully connected to Solana RPC, latest slot: %d", slot)
	return nil
}

func (s *SolanaMonitor) GetChainName() string {
	return "Solana"
}

func (s *SolanaMonitor) StartMonitoring() error {
	log.Printf("Starting %s monitoring for %d addresses", s.GetChainName(), len(s.Addresses))

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
		// Create request to get account info
		reqBody, err := json.Marshal(SolanaRpcRequest{
			Jsonrpc: "2.0",
			ID:      1,
			Method:  "getAccountInfo",
			Params: []interface{}{
				address,
				map[string]interface{}{
					"encoding":   "jsonParsed",
					"commitment": "confirmed",
				},
			},
		})

		if err != nil {
			log.Printf("Error creating account info request: %v", err)
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		// Create HTTP request
		req, err := http.NewRequest("POST", s.RpcEndpoint, bytes.NewBuffer(reqBody))
		if err != nil {
			log.Printf("Error creating HTTP request: %v", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		// Add headers
		req.Header.Set("Content-Type", "application/json")
		if s.ApiKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.ApiKey)
		}

		// Send request
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error sending HTTP request: %v", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		// Read response
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading response body: %v", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		// Parse response
		var response struct {
			Result struct {
				Value struct {
					Lamports uint64 `json:"lamports"`
				} `json:"value"`
			} `json:"result"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			log.Printf("Error parsing response: %v", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		// Check if balance changed
		currentBalance := response.Result.Value.Lamports
		if currentBalance != lastKnownBalance {
			if lastKnownBalance > 0 {
				balanceChange := int64(currentBalance) - int64(lastKnownBalance)
				solAmount := float64(balanceChange) / 1000000000 // Convert lamports to SOL

				// Fetch recent transactions to get details
				txDetails, err := s.getRecentTransactionDetails(address)
				if err != nil {
					log.Printf("Error fetching transaction details: %v", err)
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

				// Print DB storage values
				log.Printf("DB STORAGE VALUES:")
				log.Printf("  Chain:       %s", event.Chain)
				log.Printf("  From:      %s", event.From)
				log.Printf("  To: %s", event.To)
				log.Printf("  Amount:      %s SOL", event.Amount)
				log.Printf("  Fees:        %s SOL", event.Fees)
				log.Printf("  TxHash:      %s", event.TxHash)
				log.Printf("  Timestamp:   %s", event.Timestamp.Format(time.RFC3339))
				log.Printf("  Raw Balance: %d -> %d (change: %d lamports)",
					lastKnownBalance, currentBalance, balanceChange)

				s.EventEmitter.EmitEvent(event)
			} else {
				log.Printf("Initial balance for %s: %d lamports (%.9f SOL)",
					address, currentBalance, float64(currentBalance)/1000000000)
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
	reqBody, err := json.Marshal(SolanaRpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "getSlot",
		Params:  []interface{}{},
	})
	if err != nil {
		return 0, err
	}

	resp, err := s.makeRpcRequest(reqBody)
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
	reqBody, err := json.Marshal(SolanaRpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "getBlock",
		Params: []interface{}{
			slot,
			map[string]interface{}{
				"encoding":                       "json",
				"maxSupportedTransactionVersion": 0,
				"transactionDetails":             "full",
				"rewards":                        false,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.makeRpcRequest(reqBody)
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

func (s *SolanaMonitor) makeRpcRequest(reqBody []byte) (*SolanaRpcResponse, error) {
	req, err := http.NewRequest("POST", s.RpcEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Add Blockdaemon API key to the request header
	if s.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.ApiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d - %s\n", resp.StatusCode, resp.Status)
	}

	var rpcResp SolanaRpcResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&rpcResp); err != nil {
		// Try to read the raw response for debugging
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("JSON decode error: %v - Response: %s\n", err, string(bodyBytes))
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %d - %s\n", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, nil
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

				log.Printf("Detected outgoing transaction from watched address %s\n", account)
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

				log.Printf("Detected incoming transaction to watched address %s\n", account)
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
	reqBody, err := json.Marshal(SolanaRpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "getSignaturesForAddress",
		Params: []interface{}{
			address,
			map[string]interface{}{
				"limit": 1,
			},
		},
	})
	if err != nil {
		return TransactionDetails{}, err
	}

	resp, err := s.makeRpcRequest(reqBody)
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

	// Get transaction details
	txReqBody, err := json.Marshal(SolanaRpcRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "getTransaction",
		Params: []interface{}{
			signaturesResp[0].Signature,
			map[string]interface{}{
				"encoding": "jsonParsed",
			},
		},
	})
	if err != nil {
		return TransactionDetails{}, err
	}

	txResp, err := s.makeRpcRequest(txReqBody)
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

func NewSolanaMonitor() *SolanaMonitor {
	return &SolanaMonitor{
		BaseMonitor: BaseMonitor{
			RpcEndpoint: os.Getenv("SOLANA_RPC_ENDPOINT"),
			ApiKey:      os.Getenv("SOLANA_API_KEY"),
			Addresses:   []string{"oQPnhXAbLbMuKHESaGrbXT17CyvWCpLyERSJA9HCYd7"},
		},
	}
}

func (s *SolanaMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			log.Printf("%s monitor shutting down", s.GetChainName())
			return nil
		default:
			s.EventEmitter = emitter
			// Initialize Solana monitor
			if err := s.Initialize(); err != nil {
				log.Fatalf("Failed to initialize Solana monitor: %v", err)
			}
			// Start monitoring Solana blockchain
			if err := s.StartMonitoring(); err != nil {
				log.Fatalf("Failed to start Solana monitoring: %v", err)
				return err
			}
			return nil
		}
	}
}
