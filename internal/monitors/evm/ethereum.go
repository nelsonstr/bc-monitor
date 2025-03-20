package evm

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EthereumMonitor struct {
	monitors.BaseMonitor
	latestBlock uint64
	mu          sync.Mutex
}

type EthereumBlockDetails struct {
	Hash          string                `json:"hash"`
	Transactions  []EthereumTransaction `json:"transactions"`
	Number        string                `json:"number"`
	Timestamp     string                `json:"timestamp"`
	BlobGasUsed   string                `json:"blobGasUsed"`
	Difficulty    string                `json:"difficulty"`
	ExcessBlobGas string                `json:"excessBlobGas"`
	GasUsed       string                `json:"gasUsed"`
	Nonce         string                `json:"nonce"`
	Size          string                `json:"size"`
	StateRoot     string                `json:"stateRoot"`
	BaseFeePerGas string                `json:"baseFeePerGas"`
}

type EthereumTransaction struct {
	BlockNumber          string `json:"blockNumber"`
	From                 string `json:"from"`
	Gas                  string `json:"gas"`
	GasPrice             string `json:"gasPrice"`
	Hash                 string `json:"hash"`
	Input                string `json:"input"`
	Nonce                string `json:"nonce"`
	To                   string `json:"to"`
	TransactionIndex     string `json:"transactionIndex"`
	Value                string `json:"value"`
	Type                 string `json:"type"`
	ChainId              string `json:"chainId"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
}

var _ interfaces.BlockchainMonitor = (*EthereumMonitor)(nil)

func NewEthereumMonitor(endpoint string, apiKey string, rateLimit float64, log *zerolog.Logger) *EthereumMonitor {
	return &EthereumMonitor{
		BaseMonitor: monitors.BaseMonitor{
			RpcEndpoint: endpoint,
			ApiKey:      apiKey,
			Addresses:   []string{"0x00000000219ab540356cBB839Cbe05303d7705Fa"},
			MaxRetries:  1,
			RetryDelay:  2 * time.Second,
			RateLimiter: rate.NewLimiter(rate.Limit(rateLimit), 1),
			Logger:      log,
		},
	}
}

func (e *EthereumMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	e.EventEmitter = emitter

	if err := e.Initialize(); err != nil {
		e.Logger.Fatal().Err(err).Msg("Failed to initialize Ethereum monitor")
		return err
	}

	if err := e.StartMonitoring(ctx); err != nil {
		e.Logger.Fatal().Err(err).Msg("Failed to start Ethereum monitoring")
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
	return nil
}

func (e *EthereumMonitor) GetChainName() string {
	return "Ethereum"
}

func (e *EthereumMonitor) StartMonitoring(ctx context.Context) error {
	e.Logger.Info().
		Int("addressCount", len(e.Addresses)).
		Msg("Starting Ethereum monitoring")

	watchAddresses := make(map[string]bool)
	for _, addr := range e.Addresses {
		watchAddresses[strings.ToLower(addr)] = true
	}

	latestBlock, err := e.getLatestBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to get latest Ethereum block: %v", err)
	}

	e.latestBlock = latestBlock
	e.Logger.Info().
		Uint64("blockNumber", latestBlock).
		Msg("Starting Ethereum monitoring from block")

	go e.monitorBlocks(ctx, watchAddresses)

	return nil
}

func (e *EthereumMonitor) getLatestBlockNumber() (uint64, error) {
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

func (e *EthereumMonitor) getBalance(address string) (*big.Int, uint64, error) {
	params := []interface{}{address, "latest"}
	result, err := e.MakeRPCCall("eth_getBalance", params)
	if err != nil {
		return nil, 0, err
	}

	var balanceHex string
	if err := json.Unmarshal(result.Result, &balanceHex); err != nil {
		e.Logger.Error().Err(err).Msg("Error parsing response")
		return nil, 0, err
	}

	balance, ok := new(big.Int).SetString(balanceHex[2:], 16)
	if !ok {
		return nil, 0, fmt.Errorf("failed to parse balance")
	}

	blockNumber, err := e.getLatestBlockNumber()
	if err != nil {
		return nil, 0, err
	}

	return balance, blockNumber, nil
}

func (e *EthereumMonitor) monitorBlocks(ctx context.Context, watchAddresses map[string]bool) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.Logger.Info().Msg("Ethereum monitor shutting down")
			return
		case <-ticker.C:
			currentBlock, err := e.getLatestBlockNumber()
			if err != nil {
				e.Logger.Error().Err(err).Msg("Failed to get current Ethereum block")
				continue
			}

			for blockNum := e.latestBlock + 1; blockNum <= currentBlock; blockNum++ {
				if err := e.processBlock(blockNum, watchAddresses); err != nil {
					e.Logger.Error().Err(err).Uint64("blockNumber", blockNum).Msg("Error processing Ethereum block")
					continue
				}
				e.mu.Lock()
				e.latestBlock = blockNum
				e.mu.Unlock()
			}
		}
	}
}

func (e *EthereumMonitor) processBlock(blockNum uint64, watchAddresses map[string]bool) error {
	params := []interface{}{fmt.Sprintf("0x%x", blockNum), true}
	result, err := e.MakeRPCCall("eth_getBlockByNumber", params)
	if err != nil {
		return err
	}

	var blockDetails EthereumBlockDetails
	if err := json.Unmarshal(result.Result, &blockDetails); err != nil {
		return fmt.Errorf("failed to parse block details: %w", err)
	}

	e.Logger.Info().
		Uint64("blockNumber", blockNum).
		Int("transactionCount", len(blockDetails.Transactions)).
		Msg("Processing Ethereum block")

	timestamp, err := parseHexToUint64(blockDetails.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to parse block timestamp: %w", err)
	}

	for i, tx := range blockDetails.Transactions {
		if err := e.processSingleTransaction(tx, watchAddresses, timestamp, blockDetails.BaseFeePerGas); err != nil {
			e.Logger.Warn().
				Err(err).
				Int("index", i).
				Str("txHash", tx.Hash).
				Msg("Error processing Ethereum transaction")
			continue
		}

		e.Logger.Debug().
			Int("index", i).
			Str("txHash", tx.Hash).
			Msg("Successfully processed transaction")
	}

	e.Logger.Info().
		Uint64("blockNumber", blockNum).
		Msg("Finished processing Ethereum block")

	return nil
}

func (e *EthereumMonitor) processSingleTransaction(tx EthereumTransaction, watchAddresses map[string]bool, blockTime uint64, baseFeePerGas string) error {
	from := strings.ToLower(tx.From)
	to := strings.ToLower(tx.To)

	if watchAddresses[from] || watchAddresses[to] {
		event := e.processTransaction(tx, from, to, blockTime, baseFeePerGas)
		if e.EventEmitter != nil {
			e.EventEmitter.EmitEvent(event)
			e.Logger.Info().
				Str("from", from).
				Str("to", to).
				Str("amount", event.Amount).
				Str("fees", event.Fees).
				Str("txHash", tx.Hash).
				Msg("Emitted transaction event")
		} else {
			e.Logger.Warn().Msg("EventEmitter is nil, cannot emit event")
		}
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

	gasUsed := parseHexToBigInt(tx.Gas)
	gasPrice := parseHexToBigInt(tx.GasPrice)
	maxFeePerGas := parseHexToBigInt(tx.MaxFeePerGas)
	maxPriorityFeePerGas := parseHexToBigInt(tx.MaxPriorityFeePerGas)
	baseFeePerGas := parseHexToBigInt(baseFeePerGasRaw)

	fees := CalculateTransactionFee(gasUsed, maxFeePerGas, maxPriorityFeePerGas, baseFeePerGas, gasPrice)
	feesStr := formatBigFloat(fees)

	return models.TransactionEvent{
		From:      from,
		To:        to,
		Amount:    valueStr,
		Fees:      feesStr,
		Chain:     e.GetChainName(),
		TxHash:    tx.Hash,
		Timestamp: time.Unix(int64(blockTime), 0),
	}
}

func (e *EthereumMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://etherscan.io/tx/%s", txHash)
}

func parseHexToUint64(hex string) (uint64, error) {
	return strconv.ParseUint(hex[2:], 16, 64)
}

func parseHexToBigInt(hex string) *big.Int {
	value, _ := new(big.Int).SetString(hex[2:], 16)
	return value
}

func formatBigFloat(f *big.Float) string {
	str := f.Text('f', 18)
	return strings.TrimRight(strings.TrimRight(str, "0"), ".")
}
