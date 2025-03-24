package bitcoin

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type TransactionDetails struct {
	Txid string  `json:"txid"`
	Vin  []Vin   `json:"vin"`
	Vout []Vout  `json:"vout"`
	Fees float64 `json:"fees"`
	Time int64   `json:"time"`
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

type BlockDetails struct {
	Hash   string   `json:"hash"`
	Tx     []string `json:"tx"`
	Height uint64   `json:"height"`
}

type BitcoinMonitor struct {
	monitors.BaseMonitor
	latestBlockHash string
	blockHead       uint64
}

var _ interfaces.BlockchainMonitor = (*BitcoinMonitor)(nil)

func NewBitcoinMonitor(baseMonitor monitors.BaseMonitor) *BitcoinMonitor {
	return &BitcoinMonitor{
		BaseMonitor: baseMonitor,
	}
}

func (b *BitcoinMonitor) Start(ctx context.Context) error {

	if err := b.Initialize(); err != nil {
		b.Logger.Fatal().
			Err(err).
			Str("chain", b.GetChainName().String()).
			Msg("Failed to initialize monitor")
		return err
	}

	if err := b.StartMonitoring(ctx); err != nil {
		b.Logger.Fatal().
			Err(err).
			Str("chain", b.GetChainName().String()).
			Msg("Failed to start monitoring")
		return err
	}

	return nil
}

func (b *BitcoinMonitor) Initialize() error {
	b.Client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &monitors.CustomTransport{
			Base:   http.DefaultTransport,
			ApiKey: b.ApiKey,
		},
	}

	bestBlockHash, err := b.getBestBlockHash()
	if err != nil {
		return fmt.Errorf("failed to connect to Bitcoin node: %v", err)
	}

	blockHead, err := b.GetBlockHead()
	if err != nil {
		return fmt.Errorf("failed to get latest block height: %v", err)
	}

	b.latestBlockHash = bestBlockHash
	b.blockHead = blockHead

	b.Logger.Info().
		//Str("blockHash", bestBlockHash).
		Uint64("blockNumber", blockHead).
		Msg("Connected to Bitcoin node")

	return nil
}

func (b *BitcoinMonitor) getBestBlockHash() (string, error) {
	resp, err := b.MakeRPCCall("getbestblockhash", nil)
	if err != nil {
		return "", err
	}

	var blockHash string
	if err := json.Unmarshal(resp.Result, &blockHash); err != nil {
		return "", err
	}

	return blockHash, nil
}

//func (b *BitcoinMonitor) GetChainName() models.BlockchainName {
//	return models.Bitcoin
//}

func (b *BitcoinMonitor) StartMonitoring(ctx context.Context) error {
	b.Logger.Info().
		Str("chain", b.GetChainName().String()).
		Msg("Starting monitoring")

	go b.monitorBlocks(ctx)

	return nil
}

func (b *BitcoinMonitor) GetBlockHead() (uint64, error) {
	result, err := b.MakeRPCCall("getblockcount", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get current block number: %v", err)
	}

	if result.Result == nil {
		return 0, fmt.Errorf("RPC response did not contain block count")
	}

	var height uint64
	if err := json.Unmarshal(result.Result, &height); err != nil {
		return 0, err
	}

	return height, nil
}

func (b *BitcoinMonitor) monitorBlocks(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.Logger.Info().
				Str("chain", (b.GetChainName().String())).
				Msg("Shutting down")
			return

		case <-ticker.C:
			currentBlockHash, err := b.getBestBlockHash()
			if err != nil {
				b.Logger.Error().
					Err(err).
					Msg("Failed to get current Bitcoin block hash")
				continue
			}

			if currentBlockHash != b.latestBlockHash {
				blockHeight, err := b.processBlock(currentBlockHash)
				if err != nil {
					b.Logger.Error().
						Err(err).
						Str("blockHash", currentBlockHash).
						Msg("BTC - Error processing block")
					continue
				}

				b.Mu.Lock()
				b.latestBlockHash = currentBlockHash
				b.blockHead = blockHeight
				b.Mu.Unlock()

				b.Logger.Info().
					Str("blockHash", currentBlockHash).
					Uint64("blockHeight", blockHeight).
					Msg("BTC - Updated to block")
			}
		}
	}
}

func (b *BitcoinMonitor) processBlock(blockHash string) (uint64, error) {
	block, err := b.getBlock(blockHash)
	if err != nil {
		return 0, fmt.Errorf("Bitcoin - failed to get block details: %v", err)
	}

	b.Logger.Info().
		Str("blockHash", blockHash).
		Int("txCount", len(block.Tx)).
		Msg("Processing Bitcoin block")

	for _, tx := range block.Tx {
		if err := b.processTransaction(tx); err != nil {
			b.Logger.Error().
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
	result, err := b.MakeRPCCall("getblock", []interface{}{blockHash})
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
	result, err := b.MakeRPCCall("getrawtransaction", []interface{}{txHash, true})
	if err != nil {
		return nil, fmt.Errorf("RPC call failed: %v", err)
	}

	var tx TransactionDetails
	if err := json.Unmarshal(result.Result, &tx); err != nil {
		b.Logger.Error().
			Err(err).
			Str("txHash", txHash).
			Str("result", string(result.Result)).
			Msg("Failed to unmarshal transaction")
		return nil, fmt.Errorf("failed to parse transaction details: %v", err)
	}

	if tx.Txid == "" {
		return nil, fmt.Errorf("parsed transaction is invalid (empty Txid)")
	}

	return &tx, nil
}

func (b *BitcoinMonitor) isWatchedAddress(address string) bool {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	for _, watchedAddr := range b.Addresses {
		if watchedAddr == address {
			return true
		}
	}
	return false
}

func (b *BitcoinMonitor) emitTransactionEvent(tx *TransactionDetails, address string, amount float64) {
	event := models.TransactionEvent{
		Chain:       b.GetChainName(),
		From:        tx.Vin[0].Txid, // Simplified; you might need to get the actual source address
		To:          address,
		Amount:      fmt.Sprintf("%f", amount),
		Fees:        fmt.Sprintf("%f", tx.Fees),
		TxHash:      tx.Txid,
		Timestamp:   time.Unix(tx.Time, 0),
		ExplorerURL: b.GetExplorerURL(tx.Txid),
	}

	if err := b.EventEmitter.EmitEvent(event); err != nil {
		b.Logger.Error().
			Err(err).
			Str("txid", tx.Txid).
			Msg("Failed to emit event for transaction")
	}
}

func (b *BitcoinMonitor) AddAddress(address string) error {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	// Check if the address is already being monitored
	for _, watchedAddr := range b.Addresses {
		if watchedAddr == address {
			return nil // Address is already being monitored
		}
	}

	b.Addresses = append(b.Addresses, address)

	return nil
}

func (b *BitcoinMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://blockchair.com/bitcoin/transaction/%s", txHash)
}
