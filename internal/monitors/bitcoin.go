package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type BitcoinMonitor struct {
	client       *http.Client
	RpcEndpoint  string
	ApiKey       string
	latestBlock  string
	EventEmitter interfaces.EventEmitter
	Addresses    []string
	MaxRetries   int
	RetryDelay   time.Duration
}

func (b *BitcoinMonitor) Initialize() error {
	b.client = &http.Client{
		Timeout: time.Second * 10,
	}

	bestBlockHash, err := b.getBestBlockHash()
	if err != nil {
		return fmt.Errorf("failed to connect to Bitcoin node: %v\n", err)
	}

	b.latestBlock = bestBlockHash
	log.Printf("Connected to Bitcoin node. Latest block hash: %s\n", bestBlockHash)

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
	err = b.retry(func() error {
		res, err := b.client.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %v", err)
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}

		// Check if the response starts with '<', indicating HTML
		if strings.TrimSpace(string(body))[0] == '<' {
			return fmt.Errorf("received HTML response instead of JSON. First 100 characters: %s", string(body)[:100])
		}

		var response struct {
			Result string `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return fmt.Errorf("failed to parse JSON response: %v. Response body: %s", err, string(body))
		}

		if response.Error != nil {
			return fmt.Errorf("RPC error: %d - %s", response.Error.Code, response.Error.Message)
		}

		result = response.Result
		return nil
	})

	if err != nil {
		log.Printf("RPC call to method %s failed: %v\n", method, err)
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
	log.Printf("Starting %s monitoring from block %s\n", b.GetChainName(), b.latestBlock)

	go b.monitorBlocks()

	return nil
}

func (b *BitcoinMonitor) monitorBlocks() {
	for {
		currentBlockHash, err := b.getBestBlockHash()
		if err != nil {
			log.Printf("Failed to get current Bitcoin block hash: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if currentBlockHash != b.latestBlock {
			if err := b.processBlock(currentBlockHash); err != nil {
				log.Printf("BTC - Error processing block %s: %v\n", currentBlockHash, err)
			} else {
				b.latestBlock = currentBlockHash
			}
		}

		time.Sleep(10 * time.Second)
	}
}

func (b *BitcoinMonitor) processBlock(blockHash string) error {
	block, err := b.getBlock(blockHash)
	if err != nil {
		return fmt.Errorf("Bitcoin - failed to get block details: %v", err)
	}

	log.Printf("Processing Bitcoin block %s with %d transactions\n", blockHash, len(block.Tx))

	for _, tx := range block.Tx {
		if err := b.processTransaction(tx); err != nil {
			log.Printf("Error processing transaction %s: %v\n", tx, err)
		}
	}

	return nil
}

func (b *BitcoinMonitor) processTransaction(txHash string) error {
	txDetails, err := b.getTransaction(txHash)
	if err != nil {
		return fmt.Errorf("failed to get transaction details: %v\n", err)
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
	result, err := b.makeRPCCall("getrawtransaction", []interface{}{txHash, true})
	if err != nil {
		return nil, err
	}

	var tx TransactionDetails
	if err := json.Unmarshal([]byte(result), &tx); err != nil {
		return nil, err
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
	Hash string   `json:"hash"`
	Tx   []string `json:"tx"`
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
