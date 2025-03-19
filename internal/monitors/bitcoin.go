package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/time/rate"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type BitcoinMonitor struct {
	BaseMonitor
	client          *http.Client
	latestBlockHash string
	MaxRetries      int
	RetryDelay      time.Duration
	blockHead       int64
	rateLimiter     *rate.Limiter
}

func (b *BitcoinMonitor) Initialize() error {
	b.client = &http.Client{
		Timeout: time.Second * 10,
	}

	bestBlockHash, err := b.getBestBlockHash()
	if err != nil {
		return fmt.Errorf("failed to connect to Bitcoin node: %v\n", err)
	}

	blockHead, err := b.getBlockHead()
	if err != nil {
		return fmt.Errorf("failed to get latest block height: %v\n", err)
	}

	b.latestBlockHash = bestBlockHash
	logger.Log.Info().
		Str("blockHash", bestBlockHash).
		Msg("Connected to Bitcoin node")

	logger.Log.Info().
		Int64("blockNumber", blockHead).
		Msg("Connected to Bitcoin node")

	return nil
}

func (b *BitcoinMonitor) getBestBlockHash() (string, error) {
	return b.makeRPCCall("getbestblockhash", nil)
}

func (b *BitcoinMonitor) makeRPCCall(method string, params []interface{}) (string, error) {
	// Wait for rate limit
	if err := b.rateLimiter.Wait(context.Background()); err != nil {
		logger.Log.Error().Err(err).Msg("Rate limit error")
		return "", fmt.Errorf("rate limit error: %v", err)
	}

	payload, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

	logger.Log.Debug().
		Str("payload", string(payload)).
		Msg("Making RPC call")
	req, err := http.NewRequest("POST", b.RpcEndpoint, strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+b.ApiKey)
	req.Header.Add("Content-Type", "application/json")

	var result string
	var responseBody []byte
	err = b.retry(func() error {
		res, err := b.client.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %v", err)
		}
		defer res.Body.Close()

		responseBody, err = io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}

		// Check if the response starts with '<', indicating HTML
		if strings.TrimSpace(string(responseBody))[0] == '<' {
			return fmt.Errorf("received HTML response instead of JSON. Response body: %s", string(responseBody))
		}

		var response struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(responseBody, &response); err != nil {
			return fmt.Errorf("failed to parse JSON response: %v. Response body: %s", err, string(responseBody))
		}

		if response.Error != nil {
			return fmt.Errorf("RPC error: %d - %s", response.Error.Code, response.Error.Message)
		}

		// Handle the case when Result is a JSON string
		// Unmarshal the result into an interface{}
		var resultInterface interface{}
		if err := json.Unmarshal(response.Result, &resultInterface); err != nil {
			return fmt.Errorf("failed to unmarshal result: %v", err)
		}

		// Now we can use a type switch on the interface{}
		switch v := resultInterface.(type) {
		case string:
			result = v
		case float64:
			result = strconv.FormatFloat(v, 'f', -1, 64)
		case map[string]interface{}:
			result = string(response.Result)
		default:
			return fmt.Errorf("unexpected result type: %T", v)
		}

		return nil
	})

	if err != nil {
		logger.Log.Error().
			Err(err).
			Str("method", method).
			Str("responseBody", string(responseBody)).
			Msg("BITCOIN - RPC call failed")
	}

	return result, err
}

func (b *BitcoinMonitor) retry(fn func() error) error {
	var err error
	for i := 0; i < b.MaxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(b.RetryDelay)
	}
	return err
}

func (b *BitcoinMonitor) StartMonitoring() error {

	logger.Log.Info().
		Str("chain", b.GetChainName()).
		Str("rpcEndpoint", b.RpcEndpoint).
		Msg("Starting monitoring")
	logger.Log.Info().
		Str("chain", b.GetChainName()).
		Str("blockHash", b.latestBlockHash).
		Msg("Latest block hash")

	time.Sleep(5 * time.Second)

	go b.monitorBlocks()

	return nil
}

func (b *BitcoinMonitor) getBlockHead() (int64, error) {
	result, err := b.makeRPCCall("getblockcount", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get current block number: %v", err)
	}
	height, err := strconv.ParseInt(result, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block height: %v", err)
	}

	return height, nil
}

func (b *BitcoinMonitor) monitorBlocks() {
	for {
		currentBlockHash, err := b.getBestBlockHash()
		if err != nil {
			logger.Log.Error().
				Err(err).
				Msg("Failed to get current Bitcoin block hash")
			time.Sleep(5 * time.Second)
			continue
		}

		if currentBlockHash != b.latestBlockHash {
			if blockHeight, err := b.processBlock(currentBlockHash); err != nil {
				logger.Log.Error().
					Err(err).
					Str("blockHash", currentBlockHash).
					Msg("BTC - Error processing block")
			} else {
				b.latestBlockHash = currentBlockHash
				b.blockHead = blockHeight
				logger.Log.Info().
					Str("blockHash", currentBlockHash).
					Msg("BTC - Updated to block")
			}
		}

		time.Sleep(10 * time.Second)
	}
}

func (b *BitcoinMonitor) processBlock(blockHash string) (int64, error) {
	block, err := b.getBlock(blockHash)
	if err != nil {
		return 0, fmt.Errorf("Bitcoin - failed to get block details: %v", err)
	}

	logger.Log.Info().
		Str("blockHash", blockHash).
		Int("txCount", len(block.Tx)).
		Msg("Processing Bitcoin block")

	for _, tx := range block.Tx {
		if err := b.processTransaction(tx); err != nil {
			logger.Log.Error().
				Err(err).
				Str("txHash", tx).
				Msg("Error processing transaction")
		}
	}

	return block.Height, nil
}

func (b *BitcoinMonitor) processTransaction(txHash string) error {

	txDetails, err := b.getTransaction(txHash)
	if err != nil {
		return fmt.Errorf("failed to get transaction %s details: %v", txHash, err)
	}

	for _, vout := range txDetails.Vout {
		for _, addr := range vout.ScriptPubKey.Addresses {
			if b.isWatchedAddress(addr) {
				b.emitTransactionEvent(txDetails, addr, vout.Value)
				break
			}
		}
	}

	return nil
}

func (b *BitcoinMonitor) getBlock(blockHash string) (*BlockDetails, error) {
	result, err := b.makeRPCCall("getblock", []interface{}{blockHash})
	if err != nil {
		return nil, err
	}

	var block BlockDetails
	if err := json.Unmarshal([]byte(result), &block); err != nil {
		return nil, err
	}

	return &block, nil
}

func (b *BitcoinMonitor) getTransaction(txHash string) (*TransactionDetails, error) {
	time.After(time.Second * 1)
	result, err := b.makeRPCCall("getrawtransaction", []interface{}{txHash, true})
	if err != nil {
		return nil, fmt.Errorf("RPC call failed: %v", err)
	}

	// Log the raw result for debugging
	//log.Printf("Raw transaction result for %s: %s", txHash, result)

	var tx TransactionDetails
	if err := json.Unmarshal([]byte(result), &tx); err != nil {
		// Log the error and the result that caused it
		logger.Log.Error().
			Err(err).
			Str("txHash", txHash).
			Str("result", result).
			Msg("Failed to unmarshal transaction")
		return nil, fmt.Errorf("failed to parse transaction details: %v", err)
	}

	// Validate the parsed transaction
	if tx.Txid == "" {
		return nil, fmt.Errorf("parsed transaction is invalid (empty Txid)")
	}

	return &tx, nil
}

func (b *BitcoinMonitor) isWatchedAddress(address string) bool {
	for _, watchedAddr := range b.Addresses {
		if watchedAddr == address {
			return true
		}
	}
	return false
}

func (b *BitcoinMonitor) emitTransactionEvent(tx *TransactionDetails, address string, amount float64) {
	event := models.TransactionEvent{
		Chain:     b.GetChainName(),
		From:      tx.Vin[0].Txid, // This is simplified; you might need to get the actual source address
		To:        address,
		Amount:    fmt.Sprintf("%f", amount),
		Fees:      fmt.Sprintf("%f", tx.Fees),
		TxHash:    tx.Txid,
		Timestamp: time.Unix(tx.Time, 0),
	}

	if err := b.EventEmitter.EmitEvent(event); err != nil {
		logger.Log.Error().
			Err(err).
			Str("txid", tx.Txid).
			Msg("Failed to emit event for transaction")
	}
}

type BlockDetails struct {
	Hash   string   `json:"hash"`
	Tx     []string `json:"tx"`
	Height int64    `json:"height"`
}

type TransactionDetails struct {
	Txid string  `json:"txid"`
	Vin  []Vin   `json:"vin"`
	Vout []Vout  `json:"vout"`
	Fees float64 `json:"fees"`
	Time int64   `json:"time"`

	TxHash    string
	Timestamp time.Time
}

type Vin struct {
	Txid string `json:"txid"`
}

type Vout struct {
	Value        float64      `json:"value"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
}

type ScriptPubKey struct {
	Addresses []string `json:"addresses"`
}

func (b *BitcoinMonitor) GetChainName() string {
	return "Bitcoin"
}

func (b *BitcoinMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://blockchair.com/bitcoin/transaction/%s", txHash)
}

func NewBitcoinMonitor() *BitcoinMonitor {
	rlRaw := os.Getenv("RATE_LIMIT")
	rateLimit, err := strconv.Atoi(rlRaw)
	if err != nil || rateLimit <= 0 {
		rateLimit = 4
	}
	logger.Log.Info().
		Int("rateLimit", rateLimit).
		Msg("Rate limit set")
	return &BitcoinMonitor{
		BaseMonitor: BaseMonitor{
			RpcEndpoint: os.Getenv("BITCOIN_RPC_ENDPOINT"),
			ApiKey:      os.Getenv("BITCOIN_API_KEY"),
			Addresses:   []string{"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
		},
		MaxRetries: 1,
		RetryDelay: 2 * time.Second,
		// Create a rate limiter that allows 4 operations per second with a burst of 1. - max 5 RPS
		rateLimiter: rate.NewLimiter(rate.Limit(rateLimit), 1),
	}
}

func (b *BitcoinMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			logger.Log.Info().
				Str("chain", b.GetChainName()).
				Msg("Shutting down")
			return nil
		default:
			b.EventEmitter = emitter
			// Initialize Bitcoin monitor
			if err := b.Initialize(); err != nil {
				logger.Log.Fatal().
					Err(err).
					Str("chain", b.GetChainName()).
					Msg("Failed to initialize monitor")
				return err
			}
			// Start monitoring Bitcoin blockchain
			if err := b.StartMonitoring(); err != nil {
				logger.Log.Fatal().
					Err(err).
					Str("chain", b.GetChainName()).
					Msg("Failed to start monitoring")
				return err
			}
			return nil
		}
	}
}
