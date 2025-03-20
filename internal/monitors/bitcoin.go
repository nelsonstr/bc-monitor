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

type BitcoinMonitor struct {
	BaseMonitor
	latestBlockHash string
	blockHead       int64
}

var _ interfaces.BlockchainMonitor = (*BitcoinMonitor)(nil)

func NewBitcoinMonitor(log *zerolog.Logger) *BitcoinMonitor {
	rlRaw := os.Getenv("RATE_LIMIT")
	rateLimit, err := strconv.Atoi(rlRaw)
	if err != nil || rateLimit <= 0 {
		rateLimit = 4
	}
	log.Info().
		Int("rateLimit", rateLimit).
		Msg("Rate limit set")

	return &BitcoinMonitor{
		BaseMonitor: BaseMonitor{
			RpcEndpoint: os.Getenv("BITCOIN_RPC_ENDPOINT"),
			ApiKey:      os.Getenv("BITCOIN_API_KEY"),
			Addresses:   []string{"bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"},
			maxRetries:  1,
			retryDelay:  2 * time.Second,
			rateLimiter: rate.NewLimiter(rate.Limit(rateLimit), 1),
			logger:      log,
		},
	}
}

func (b *BitcoinMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			b.logger.Info().
				Str("chain", b.GetChainName()).
				Msg("Shutting down")
			return nil
		default:
			b.EventEmitter = emitter
			// Initialize Bitcoin monitor
			if err := b.Initialize(); err != nil {
				b.logger.Fatal().
					Err(err).
					Str("chain", b.GetChainName()).
					Msg("Failed to initialize monitor")
				return err
			}
			// Start monitoring Bitcoin blockchain
			if err := b.StartMonitoring(); err != nil {
				b.logger.Fatal().
					Err(err).
					Str("chain", b.GetChainName()).
					Msg("Failed to start monitoring")
				return err
			}
			return nil
		}
	}
}

func (b *BitcoinMonitor) Initialize() error {
	b.BaseMonitor.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &customTransport{
			base:   http.DefaultTransport,
			apiKey: b.ApiKey,
		},
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
	b.logger.Info().
		Str("blockHash", bestBlockHash).
		Msg("Connected to Bitcoin node")
	b.blockHead = blockHead
	b.logger.Info().
		Int64("blockNumber", blockHead).
		Msg("Connected to Bitcoin node")

	return nil
}

func (b *BitcoinMonitor) getBestBlockHash() (string, error) {
	resp, err := b.makeRPCCall("getbestblockhash", nil)
	if err != nil {
		return "", err
	}

	var blockHash string
	if err := json.Unmarshal(resp.Result, &blockHash); err != nil {
		return "", err
	}

	return blockHash, nil
}

func (b *BitcoinMonitor) GetChainName() string {
	return "Bitcoin"
}

func (b *BitcoinMonitor) StartMonitoring() error {

	b.logger.Info().
		Str("chain", b.GetChainName()).
		Str("rpcEndpoint", b.RpcEndpoint).
		Msg("Starting monitoring")

	b.logger.Info().
		Str("chain", b.GetChainName()).
		Str("blockHash", b.latestBlockHash).
		Msg("Latest block hash")

	go b.monitorBlocks()

	return nil
}

func (b *BitcoinMonitor) getBlockHead() (int64, error) {
	result, err := b.makeRPCCall("getblockcount", nil)
	if err != nil {

		return 0, fmt.Errorf("failed to get current block number: %v", err)
	}

	if result.Result == nil {

		return 0, fmt.Errorf("RPC response did not contain block count")
	}

	var height int64
	if err := json.Unmarshal(result.Result, &height); err != nil {

		return 0, err
	}

	return height, nil
}

func (b *BitcoinMonitor) monitorBlocks() {
	for {
		currentBlockHash, err := b.getBestBlockHash()
		if err != nil {
			b.logger.Error().
				Err(err).
				Msg("Failed to get current Bitcoin block hash")

			time.Sleep(5 * time.Second)

			continue
		}

		if currentBlockHash != b.latestBlockHash {
			if blockHeight, err := b.processBlock(currentBlockHash); err != nil {
				b.logger.Error().
					Err(err).
					Str("blockHash", currentBlockHash).
					Msg("BTC - Error processing block")
			} else {
				b.latestBlockHash = currentBlockHash
				b.blockHead = blockHeight

				b.logger.Info().
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

	b.logger.Info().
		Str("blockHash", blockHash).
		Int("txCount", len(block.Tx)).
		Msg("Processing Bitcoin block")

	for _, tx := range block.Tx {
		if err := b.processTransaction(tx); err != nil {
			b.logger.Error().
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
	if err := json.Unmarshal(result.Result, &block); err != nil {
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

	var tx TransactionDetails
	if err := json.Unmarshal(result.Result, &tx); err != nil {
		// GetLogger the error and the result that caused it
		b.logger.Error().
			Err(err).
			Str("txHash", txHash).
			Str("result", tx.TxHash).
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
		b.logger.Error().
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

func (b *BitcoinMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://blockchair.com/bitcoin/transaction/%s", txHash)
}
