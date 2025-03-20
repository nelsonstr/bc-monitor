package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type EthereumMonitor struct {
	BaseMonitor
	client      *http.Client
	latestBlock uint64
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
}

type EthereumTransaction struct {
	BlockNumber      string `json:"blockNumber"`
	From             string `json:"from"`
	Gas              string `json:"gas"`
	GasPrice         string `json:"gasPrice"`
	Hash             string `json:"hash"`
	Input            string `json:"input"`
	Nonce            string `json:"nonce"`
	To               string `json:"to"`
	TransactionIndex string `json:"transactionIndex"`
	Value            string `json:"value"`
	Type             string `json:"type"`
	ChainId          string `json:"chainId"`
}

var _ interfaces.BlockchainMonitor = (*EthereumMonitor)(nil)

func NewEthereumMonitor(log *zerolog.Logger) *EthereumMonitor {
	rlRaw := os.Getenv("RATE_LIMIT")
	rateLimit, err := strconv.Atoi(rlRaw)
	if err != nil || rateLimit <= 0 {
		rateLimit = 4
	}
	log.Info().
		Int("rateLimit", rateLimit).
		Msg("Rate limit set")

	return &EthereumMonitor{
		BaseMonitor: BaseMonitor{
			RpcEndpoint: os.Getenv("ETHEREUM_RPC_ENDPOINT"),
			ApiKey:      os.Getenv("ETHEREUM_API_KEY"),
			Addresses:   []string{"0x00000000219ab540356cBB839Cbe05303d7705Fa"},
			maxRetries:  1,
			retryDelay:  2 * time.Second,
			rateLimiter: rate.NewLimiter(rate.Limit(rateLimit), 1),
			logger:      log,
		},
	}
}

func (e *EthereumMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			e.logger.Info().Msg("Ethereum monitor shutting down")
			return nil
		default:
			e.EventEmitter = emitter

			if err := e.Initialize(); err != nil {
				e.logger.Fatal().Err(err).Msg("Failed to initialize Ethereum monitor")
				return err
			}
			if err := e.StartMonitoring(); err != nil {
				e.logger.Fatal().Err(err).Msg("Failed to start Ethereum monitoring")
				return err
			}
			return nil
		}
	}
}

func (e *EthereumMonitor) Initialize() error {
	e.BaseMonitor.client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &customTransport{
			base:   http.DefaultTransport,
			apiKey: e.ApiKey,
		},
	}
	return nil
}

func (e *EthereumMonitor) GetChainName() string {
	return "Ethereum"
}

func (e *EthereumMonitor) StartMonitoring() error {
	e.logger.Info().
		Int("addressCount", len(e.Addresses)).
		Msg("Starting Ethereum monitoring")

	for _, address := range e.Addresses {
		balance, blockNumber, err := e.getBalance(address)
		if err != nil {
			e.logger.Error().Err(err).Str("address", address).Msg("Error getting initial balance")
		} else {
			ethBalance := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
			e.logger.Info().
				Str("address", address).
				Str("balance", ethBalance.Text('f', 18)).
				Uint64("blockNumber", blockNumber).
				Msg("Initial balance")
		}
	}

	watchAddresses := make(map[string]bool)
	for _, addr := range e.Addresses {
		watchAddresses[strings.ToLower(addr)] = true
	}

	latestBlock, err := e.getLatestBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to get latest Ethereum block: %v", err)
	}

	e.latestBlock = latestBlock
	e.logger.Info().
		Uint64("blockNumber", latestBlock).
		Msg("Starting Ethereum monitoring from block")

	go e.monitorBlocks(watchAddresses)

	return nil
}

func (e *EthereumMonitor) getLatestBlockNumber() (uint64, error) {
	result, err := e.makeRPCCall("eth_blockNumber", nil)
	if err != nil {

		return 0, err
	}
	// Parse response
	var blockNumberHex string

	if err := json.Unmarshal(result.Result, &blockNumberHex); err != nil {
		e.logger.Error().
			Err(err).
			Msg("Error parsing response")

		return 0, err
	}

	blockNumber, err := strconv.ParseUint(blockNumberHex[2:], 16, 64)
	if err != nil {

		return 0, fmt.Errorf("failed to parse block number: %v", err)
	}

	return blockNumber, nil
}

func (e *EthereumMonitor) getBalance(address string) (*big.Int, uint64, error) {
	params := []interface{}{address, "latest"}
	result, err := e.makeRPCCall("eth_getBalance", params)
	if err != nil {

		return nil, 0, err
	}
	// Parse response
	var balanceHex string

	if err := json.Unmarshal(result.Result, &balanceHex); err != nil {
		e.logger.Error().
			Err(err).
			Msg("Error parsing response")

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

func (e *EthereumMonitor) monitorBlocks(watchAddresses map[string]bool) {
	for {
		currentBlock, err := e.getLatestBlockNumber()
		if err != nil {
			e.logger.Error().Err(err).Msg("Failed to get current Ethereum block")
			time.Sleep(5 * time.Second)

			continue
		}

		for blockNum := e.latestBlock + 1; blockNum <= currentBlock; blockNum++ {
			if err := e.processBlock(blockNum, watchAddresses); err != nil {
				e.logger.Error().Err(err).Uint64("blockNumber", blockNum).Msg("Error processing Ethereum block")

				continue
			}
			e.latestBlock = blockNum
		}

		time.Sleep(15 * time.Second)
	}
}

func (e *EthereumMonitor) processBlock(blockNum uint64, watchAddresses map[string]bool) error {
	params := []interface{}{fmt.Sprintf("0x%x", blockNum), true}
	result, err := e.makeRPCCall("eth_getBlockByNumber", params)
	if err != nil {
		return err
	}

	var blockDetails EthereumBlockDetails
	if err := json.Unmarshal(result.Result, &blockDetails); err != nil {
		return fmt.Errorf("failed to parse block details: %w", err)
	}

	e.logger.Info().
		Uint64("blockNumber", blockNum).
		Int("transactionCount", len(blockDetails.Transactions)).
		Msg("Processing Ethereum block")
	timestamp, err := strconv.ParseUint(blockDetails.Timestamp[2:], 16, 64)
	if err != nil {

		return fmt.Errorf("failed to parse block timestamp: %w", err)
	}

	for i, tx := range blockDetails.Transactions {

		if err := e.processSingleTransaction(tx, watchAddresses, timestamp); err != nil {
			e.logger.Warn().
				Err(err).
				Int("index", i).
				Str("txHash", tx.Hash).
				Msg("Error processing Ethereum transaction")
			continue
		}

		e.logger.Debug().
			Int("index", i).
			Str("txHash", tx.Hash).
			Msg("Successfully processed transaction")
	}

	e.logger.Info().
		Uint64("blockNumber", blockNum).
		Msg("Finished processing Ethereum block")

	return nil
}

func (e *EthereumMonitor) processSingleTransaction(tx EthereumTransaction, watchAddresses map[string]bool, blockTime uint64) error {
	from := strings.ToLower(tx.From)
	to := strings.ToLower(tx.To)

	if watchAddresses[from] || watchAddresses[to] {
		event := e.processTransaction(tx, from, to, blockTime)
		if e.EventEmitter != nil {
			e.EventEmitter.EmitEvent(event)
			e.logger.Info().
				Str("from", from).
				Str("to", to).
				Str("txHash", tx.Hash).
				Msg("Emitted transaction event")
		} else {
			e.logger.Warn().Msg("EventEmitter is nil, cannot emit event")
		}
	}

	return nil
}

func (e *EthereumMonitor) processTransaction(tx EthereumTransaction, from, to string, blockTime uint64) models.TransactionEvent {
	value, _ := new(big.Int).SetString(tx.Value[2:], 16)
	valueFloat := new(big.Float).Quo(new(big.Float).SetInt(value), big.NewFloat(1e18))
	valueStr := valueFloat.Text('f', 18)
	valueStr = strings.TrimRight(strings.TrimRight(valueStr, "0"), ".")

	gasPrice, _ := new(big.Int).SetString(tx.GasPrice[2:], 16)
	gasUsed, _ := new(big.Int).SetString(tx.Gas[2:], 16)
	fees := new(big.Float).Quo(
		new(big.Float).SetInt(new(big.Int).Mul(gasPrice, gasUsed)),
		big.NewFloat(1e18),
	)
	feesStr := fees.Text('f', 18)
	feesStr = strings.TrimRight(strings.TrimRight(feesStr, "0"), ".")

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

type customTransport struct {
	base   http.RoundTripper
	apiKey string
}

func (t *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Content-Type", "application/json")
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}
	return t.base.RoundTrip(req)
}

func (e *EthereumMonitor) GetExplorerURL(txHash string) string {
	return fmt.Sprintf("https://etherscan.io/tx/%s", txHash)
}
