package evm

import (
	"blockchain-monitor/internal/database"
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var _ interfaces.BlockchainMonitor = (*EthereumMonitor)(nil)

type EthereumMonitor struct {
	*monitors.BaseMonitor
	latestBlockHeight uint64
}

func (e *EthereumMonitor) AddAddress(address string) error {
	address = strings.ToLower(address)
	return e.BaseMonitor.AddAddress(address)
}

func NewEthereumMonitor(baseMonitor *monitors.BaseMonitor) *EthereumMonitor {
	return &EthereumMonitor{
		BaseMonitor: baseMonitor,
	}
}

func (e *EthereumMonitor) Start(ctx context.Context) error {

	if err := e.Initialize(); err != nil {
		e.Logger.Error().Err(err).Msg("Failed to initialize Ethereum monitor")
		return err
	}

	if err := e.StartMonitoring(ctx); err != nil {
		e.Logger.Error().Err(err).Msg("Failed to start Ethereum monitoring")
		return err
	}

	return nil
}

func (e *EthereumMonitor) Initialize() error {
	e.Client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &monitors.CustomTransport{
			Base:   http.DefaultTransport,
			ApiKey: e.ApiKey,
		},
	}

	// Load last scanned block from database
	state, err := database.GetBlockchainState(e.GetChainName().String())
	if err != nil {
		e.Logger.Error().Err(err).Msg("Failed to get blockchain state from DB")
	}

	if state != nil {
		e.latestBlockHeight = state.LastBlockHeight
		e.Logger.Info().
			Uint64("blockNumber", e.latestBlockHeight).
			Msg("Resuming Ethereum monitoring from DB state")
	} else {
		// New chain, get current block head
		latestBlock, err := e.GetBlockHead()
		if err != nil {
			return fmt.Errorf("failed to get latest Ethereum block: %v", err)
		}
		e.latestBlockHeight = latestBlock
		e.Logger.Info().
			Uint64("blockNumber", latestBlock).
			Msg("Starting Ethereum monitoring from current head")
	}

	return nil
}

func (e *EthereumMonitor) StartMonitoring(ctx context.Context) error {

	e.Mu.Lock()
	defer e.Mu.Unlock()

	watchAddresses := make(map[string]bool, len(e.Addresses))
	for _, addr := range e.Addresses {
		watchAddresses[strings.ToLower(addr)] = true
	}

	e.Logger.Info().
		Int("addressCount", len(e.Addresses)).
		Uint64("blockNumber", e.latestBlockHeight).
		Msg("Starting Ethereum monitoring loop")

	go e.monitorBlocks(ctx)

	return nil
}

func (e *EthereumMonitor) GetBlockHead() (uint64, error) {
	result, err := e.MakeRPCCall("eth_blockNumber", nil)
	if err != nil {
		return 0, err
	}

	var blockNumberHex string
	if err := json.Unmarshal(result.Result, &blockNumberHex); err != nil {
		e.Logger.Error().Err(err).Msg("Error parsing response")
		return 0, err
	}

	return parseHexToUint64(blockNumberHex)
}


func (e *EthereumMonitor) monitorBlocks(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.Logger.Info().Msg("Ethereum monitor shutting down")
			return
		case <-ticker.C:
			currentBlock, err := e.GetBlockHead()
			if err != nil {
				e.Logger.Error().Err(err).Msg("Failed to get current Ethereum block")
				continue
			}

			for blockNum := e.latestBlockHeight + 1; blockNum <= currentBlock; blockNum++ {
				blockDetails, err := e.fetchBlockWithRetry(blockNum, 3)
				if err != nil {
					e.Logger.Error().Err(err).Uint64("blockNumber", blockNum).Msg("Failed to fetch Ethereum block after retries")
					break
				}

				if err := e.processFetchedBlock(blockNum, blockDetails); err != nil {
					e.Logger.Error().Err(err).Uint64("blockNumber", blockNum).Msg("Error processing Ethereum block")
				}

				// Update state in DB and cache
				if err := database.UpdateBlockchainState(e.GetChainName().String(), blockNum, blockDetails.Hash); err != nil {
					e.Logger.Error().Err(err).Uint64("blockNumber", blockNum).Msg("Failed to update blockchain state in DB")
				}

				e.Mu.Lock()
				e.latestBlockHeight = blockNum
				e.Mu.Unlock()
			}
		}
	}
}

func (e *EthereumMonitor) fetchBlockWithRetry(blockNum uint64, retries int) (*EthereumBlockDetails, error) {
	var blockDetails EthereumBlockDetails
	err := e.Retry(func() error {
		params := []interface{}{fmt.Sprintf("0x%x", blockNum), true}
		result, err := e.MakeRPCCall("eth_getBlockByNumber", params)
		if err != nil {
			return err
		}
		if result.Result == nil {
			return fmt.Errorf("block %d not found yet", blockNum)
		}
		return json.Unmarshal(result.Result, &blockDetails)
	})
	if err != nil {
		return nil, err
	}
	return &blockDetails, nil
}

func (e *EthereumMonitor) processFetchedBlock(blockNum uint64, blockDetails *EthereumBlockDetails) error {
	e.Logger.Info().
		Uint64("blockNumber", blockNum).
		Int("transactionCount", len(blockDetails.Transactions)).
		Msg("Processing Ethereum block")

	timestamp, err := parseHexToUint64(blockDetails.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to parse block timestamp: %w", err)
	}

	for _, tx := range blockDetails.Transactions {
		if err := e.processSingleTransaction(tx, timestamp, blockDetails.BaseFeePerGas); err != nil {
			e.Logger.Warn().
				Err(err).
				Str("txHash", tx.Hash).
				Msg("Error processing Ethereum transaction")
			continue
		}
	}

	return nil
}


func (e *EthereumMonitor) processSingleTransaction(tx EthereumTransaction, blockTime uint64, baseFeePerGas string) error {
	from := strings.ToLower(tx.From)
	to := strings.ToLower(tx.To)

	e.Logger.Debug().
		Str("from", from).
		Str("to", to).
		Str("e.Addresses", strings.Join(e.Addresses, ",")).
		Msg("Processing Ethereum transaction")

	found := false
	for _, addr := range e.Addresses {
		if addr == from || addr == to {
			found = true

			break
		}
	}

	if !found {
		return nil
	}
	event := e.processTransaction(tx, from, to, blockTime, baseFeePerGas)

	// Save transaction to DB
	if err := database.SaveTransaction(event); err != nil {
		e.Logger.Error().Err(err).Str("txHash", tx.Hash).Msg("Failed to save transaction to DB")
	}

	if e.EventEmitter != nil {
		e.Logger.Info().
			Str("from", from).
			Str("to", to).
			Str("amount", event.Amount).
			Str("fees", event.Fees).
			Str("txHash", tx.Hash).
			Msg("Emitted transaction event")
		if err := e.EventEmitter.EmitEvent(event); err != nil {
			e.Logger.Error().
				Err(err).
				Msg("Error emitting transaction event")
			return nil
		}

	} else {
		e.Logger.Warn().Msg("EventEmitter is nil, cannot emit event")
	}

	return nil
}

func CalculateTransactionFee(gasUsed, maxFeePerGas, maxPriorityFeePerGas, baseFeePerGas, gasPrice *big.Int) *big.Float {
	if gasPrice != nil {
		totalFee := new(big.Int).Mul(gasUsed, gasPrice)
		return new(big.Float).Quo(new(big.Float).SetInt(totalFee), big.NewFloat(1e18))
	}

	if maxFeePerGas == nil || maxPriorityFeePerGas == nil || baseFeePerGas == nil {
		return big.NewFloat(0)
	}

	priorityFee := new(big.Int).Sub(maxFeePerGas, baseFeePerGas)
	if priorityFee.Cmp(maxPriorityFeePerGas) > 0 {
		priorityFee.Set(maxPriorityFeePerGas)
	}

	totalFeePerGas := new(big.Int).Add(baseFeePerGas, priorityFee)
	totalFee := new(big.Int).Mul(gasUsed, totalFeePerGas)

	return new(big.Float).Quo(new(big.Float).SetInt(totalFee), big.NewFloat(1e18))
}

func (e *EthereumMonitor) processTransaction(tx EthereumTransaction, from, to string, blockTime uint64, baseFeePerGasRaw string) models.TransactionEvent {

	value := parseHexToBigInt(tx.Value)
	valueFloat := new(big.Float).Quo(new(big.Float).SetInt(value), big.NewFloat(1e18))
	valueStr := formatBigFloat(valueFloat)

	e.Logger.Debug().
		Str("from", from).
		Str("to", to).
		Str("value", valueStr).
		Str("gasUsed", tx.Gas).
		Str("gasPrice", tx.GasPrice).
		Msg("Processing transaction")

	gasUsed := parseHexToBigInt(tx.Gas)
	gasPrice := parseHexToBigInt(tx.GasPrice)
	maxFeePerGas := parseHexToBigInt(tx.MaxFeePerGas)
	maxPriorityFeePerGas := parseHexToBigInt(tx.MaxPriorityFeePerGas)
	baseFeePerGas := parseHexToBigInt(baseFeePerGasRaw)

	fees := CalculateTransactionFee(gasUsed, maxFeePerGas, maxPriorityFeePerGas, baseFeePerGas, gasPrice)
	feesStr := formatBigFloat(fees)

	return models.TransactionEvent{
		From:        from,
		To:          to,
		Amount:      valueStr,
		Fees:        feesStr,
		Chain:       e.GetChainName(),
		TxHash:      tx.Hash,
		Timestamp:   time.Unix(int64(blockTime), 0),
		ExplorerURL: e.GetExplorerURL(tx.Hash),
	}
}

func (e *EthereumMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("%s%s", e.ExplorerBaseURL, txHash)
}

func parseHexToUint64(hex string) (uint64, error) {
	return strconv.ParseUint(hex[2:], 16, 64)
}

func parseHexToBigInt(hex string) *big.Int {
	if len(hex) == 0 {
		return new(big.Int).SetInt64(0)
	}

	value, _ := new(big.Int).SetString(hex[2:], 16)
	return value
}

func formatBigFloat(f *big.Float) string {
	str := f.Text('f', 18)
	return strings.TrimRight(strings.TrimRight(str, "0"), ".")
}

func (e *EthereumMonitor) Stop(_ context.Context) error {
	e.Logger.Info().Msg("Stopping Ethereum monitor")

	e.CloseHTTPClient()

	return nil
}
