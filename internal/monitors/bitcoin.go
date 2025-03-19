package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"context"
	"encoding/json"
	"fmt"

	rate "golang.org/x/time/rate"
	"io"
	"log"
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
	log.Printf("Connected to Bitcoin node. Latest block hash: %s\n", bestBlockHash)
	log.Printf("Connected to Bitcoin node. Latest block number: %d\n", blockHead)

	return nil
}

func (b *BitcoinMonitor) getBestBlockHash() (string, error) {
	return b.makeRPCCall("getbestblockhash", nil)
}

func (b *BitcoinMonitor) makeRPCCall(method string, params []interface{}) (string, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON payload: %v", err)
	}

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
		log.Printf("RPC call to method %s failed: %v\nResponse body: %s\n", method, err, string(responseBody))
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

	log.Printf("Starting %s monitoring using RPC endpoint: %s\n", b.GetChainName(), b.RpcEndpoint)
	log.Printf("Latest %s block hash: %s\n", b.GetChainName(), b.latestBlockHash)

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
			log.Printf("Failed to get current Bitcoin block hash: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if currentBlockHash != b.latestBlockHash {
			if blockHeight, err := b.processBlock(currentBlockHash); err != nil {
				log.Printf("BTC - Error processing block %s: %v\n", currentBlockHash, err)
			} else {

				b.latestBlockHash = currentBlockHash
				b.blockHead = blockHeight
				log.Printf("BTC - Updated to block %s\n", currentBlockHash)
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

	log.Printf("Processing Bitcoin block %s with %d transactions\n", blockHash, len(block.Tx))

	for _, tx := range block.Tx {
		if err := b.processTransaction(tx); err != nil {
			log.Printf("Error processing transaction %s: %v\n", tx, err)
		}
	}

	return block.Height, nil
}

func (b *BitcoinMonitor) processTransaction(txHash string) error {
	//Wait for rate limit
	if err := b.rateLimiter.Wait(context.Background()); err != nil {
		return fmt.Errorf("rate limit error: %v", err)
	}

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
		log.Printf("Failed to unmarshal transaction %s: %v\nRaw result: %s", txHash, err, result)
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
		log.Printf("Failed to emit event for transaction %s: %v\n", tx.Txid, err)
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
	return &BitcoinMonitor{
		BaseMonitor: BaseMonitor{
			RpcEndpoint: os.Getenv("BITCOIN_RPC_ENDPOINT"),
			ApiKey:      os.Getenv("BLOCKDAEMON_API_KEY"),
			Addresses:   []string{"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
		},
		MaxRetries:  2,
		RetryDelay:  2 * time.Second,
		rateLimiter: rate.NewLimiter(rate.Every(time.Second), 100), // 100 requests per second
	}
}

func (b *BitcoinMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			log.Printf("%s monitor shutting down", b.GetChainName())
			return nil
		default:
			b.EventEmitter = emitter
			// Initialize Bitcoin monitor
			if err := b.Initialize(); err != nil {
				log.Fatalf("Failed to initialize Bitcoin monitor: %v", err)
				return err
			}
			// Start monitoring Bitcoin blockchain
			if err := b.StartMonitoring(); err != nil {
				log.Fatalf("Failed to start Bitcoin monitoring: %v", err)
				return err
			}
			return nil
		}
	}
}
