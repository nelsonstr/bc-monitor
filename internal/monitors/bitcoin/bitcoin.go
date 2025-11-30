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

type BitcoinMonitor struct {
	*monitors.BaseMonitor
	latestBlockHash   string
	latestBlockHeight uint64
}

var _ interfaces.BlockchainMonitor = (*BitcoinMonitor)(nil)

func NewBitcoinMonitor(baseMonitor *monitors.BaseMonitor) *BitcoinMonitor {
	return &BitcoinMonitor{
		BaseMonitor: baseMonitor,
	}
}

func (b *BitcoinMonitor) Start(ctx context.Context) error {

	if err := b.Initialize(); err != nil {
		b.Logger.Error().
			Err(err).
			Str("chain", b.GetChainName().String()).
			Msg("Failed to initialize monitor")
		return err
	}

	if err := b.StartMonitoring(ctx); err != nil {
		b.Logger.Error().
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
	b.latestBlockHeight = blockHead

	b.Logger.Info().
		Str("bestBlockHash", bestBlockHash).
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
				Str("chain", b.GetChainName().String()).
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
			b.Logger.Debug().Str("currentBlockHash", currentBlockHash).
				Str("bestBlockHash", b.latestBlockHash).
				Msg("BTC")

			if currentBlockHash != b.latestBlockHash {
				b.Logger.Debug().Str("currentBlockHash", currentBlockHash).
					Str("bestBlockHash", b.latestBlockHash).
					Msg("BTC new block")
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
				b.Logger.Info().
					Str("blockHash", currentBlockHash).
					Msg("BTC - Updated to block")
				b.latestBlockHeight = blockHeight
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

	// iterate over the transaction outputs to find the watched addresses
	for _, vout := range txDetails.Vout {

		if b.IsWatchedAddress(vout.ScriptPubKey.Address) {
			var fromAddrs []string
			var inputSum float64

			for _, vin := range txDetails.Vin {
				f, val, err := b.getPrevOutputValue(vin.TxID, vin.Vout)
				if err != nil {
					b.Logger.Err(err).Msg("Warning: failed to fetch input value")
					continue
				}
				fromAddrs = append(fromAddrs, f)
				inputSum += val
			}
			var outputSum float64
			for _, vout := range txDetails.Vout {
				outputSum += vout.Value
			}
			fees := inputSum - outputSum

			// send an event for each watched address involved in the transaction
			for _, addr := range fromAddrs {
				b.emitTransactionEvent(txDetails, addr, vout.ScriptPubKey.Address, vout.Value, fees)
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

func (b *BitcoinMonitor) getPrevOutputValue(txid string, voutIndex int) (string, float64, error) {
	prevTx, err := b.getTransaction(txid)

	if err != nil {
		return "", 0, err
	}
	for _, vout := range prevTx.Vout {
		if vout.N == voutIndex {
			return vout.ScriptPubKey.Address, vout.Value, nil // value in sats
		}
	}
	return "", 0, fmt.Errorf("vout %d not found in tx %s", voutIndex, txid)
}

func (b *BitcoinMonitor) emitTransactionEvent(tx *TransactionDetails, from, to string, amount, fees float64) {
	event := models.TransactionEvent{
		Chain:       b.GetChainName(),
		From:        from,
		To:          to,
		Amount:      fmt.Sprintf("%f", amount),
		Fees:        fmt.Sprintf("%f", fees),
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

func (b *BitcoinMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://blockchair.com/bitcoin/transaction/%s", txHash)
}

func (b *BitcoinMonitor) Stop(_ context.Context) error {
	b.Logger.Info().Msg("Stopping Bitcoin monitor")

	b.CloseHTTPClient()

	return nil
}
