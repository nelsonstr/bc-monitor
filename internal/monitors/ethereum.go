package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"blockchain-monitor/internal/models"
	"context"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type EthereumMonitor struct {
	BaseMonitor
	client      *ethclient.Client
	latestBlock uint64
	MaxRetries  int
	RetryDelay  time.Duration
}

func (e *EthereumMonitor) Initialize() error {
	if e.ApiKey == "" {
		return fmt.Errorf("API key not provided")
	}

	httpClient := &http.Client{
		Transport: &customTransport{
			base:   http.DefaultTransport,
			apiKey: e.ApiKey,
		},
	}

	rpcClient, err := rpc.DialHTTPWithClient(e.RpcEndpoint, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create RPC client: %v", err)
	}

	e.client = ethclient.NewClient(rpcClient)
	return nil
}

func (e *EthereumMonitor) GetChainName() string {
	return "Ethereum"
}

func (e *EthereumMonitor) StartMonitoring() error {
	logger.Log.Info().
		Int("addressCount", len(e.Addresses)).
		Msg("Starting Ethereum monitoring")

	for _, address := range e.Addresses {
		balance, blockNumber, err := e.getBalance(address)
		if err != nil {
			logger.Log.Error().Err(err).Str("address", address).Msg("Error getting initial balance")
		} else {
			ethBalance := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
			logger.Log.Info().
				Str("address", address).
				Str("balance", ethBalance.Text('f', 18)).
				Uint64("blockNumber", blockNumber).
				Msg("Initial balance")
		}
	}

	watchAddresses := make(map[common.Address]bool)
	for _, addr := range e.Addresses {
		watchAddresses[common.HexToAddress(addr)] = true
	}

	latestBlock, err := e.client.BlockNumber(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get latest Ethereum block: %v", err)
	}

	e.latestBlock = latestBlock
	logger.Log.Info().
		Uint64("blockNumber", latestBlock).
		Msg("Starting Ethereum monitoring from block")

	go e.monitorBlocks(watchAddresses)

	return nil
}

func (e *EthereumMonitor) monitorBlocks(watchAddresses map[common.Address]bool) {
	for {
		currentBlock, err := e.client.BlockNumber(context.Background())
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to get current Ethereum block")
			time.Sleep(5 * time.Second)
			continue
		}

		for blockNum := e.latestBlock + 1; blockNum <= currentBlock; blockNum++ {
			if err := e.processBlock(blockNum, watchAddresses); err != nil {
				logger.Log.Error().Err(err).Uint64("blockNumber", blockNum).Msg("Error processing Ethereum block")
				continue
			}
			e.latestBlock = blockNum
		}

		time.Sleep(15 * time.Second)
	}
}

func (e *EthereumMonitor) processBlock(blockNum uint64, watchAddresses map[common.Address]bool) error {
	var block *types.Block
	var err error

	for attempt := 0; attempt <= e.MaxRetries; attempt++ {
		block, err = e.client.BlockByNumber(context.Background(), big.NewInt(int64(blockNum)))
		if err == nil {
			break
		}

		if attempt < e.MaxRetries {
			logger.Log.Warn().
				Err(err).
				Int("attempt", attempt+1).
				Uint64("blockNumber", blockNum).
				Msg("Failed to get Ethereum block. Retrying...")
			time.Sleep(e.RetryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to get Ethereum block %d after %d attempts: %v", blockNum, e.MaxRetries+1, err)
	}

	logger.Log.Info().
		Uint64("blockNumber", blockNum).
		Int("transactionCount", len(block.Transactions())).
		Msg("Processing Ethereum block")

	for i, tx := range block.Transactions() {
		logger.Log.Debug().
			Int("index", i).
			Str("txHash", tx.Hash().Hex()).
			Msg("Processing transaction")

		if err := e.processSingleTransaction(tx, watchAddresses, block.Time()); err != nil {
			logger.Log.Warn().
				Err(err).
				Int("index", i).
				Str("txHash", tx.Hash().Hex()).
				Msg("Error processing Ethereum transaction")
			continue
		}

		logger.Log.Debug().
			Int("index", i).
			Str("txHash", tx.Hash().Hex()).
			Msg("Successfully processed transaction")
	}

	logger.Log.Info().
		Uint64("blockNumber", blockNum).
		Msg("Finished processing Ethereum block")

	return nil
}

func (e *EthereumMonitor) processSingleTransaction(tx *types.Transaction, watchAddresses map[common.Address]bool, blockTime uint64) error {
	if tx == nil {
		return fmt.Errorf("received nil transaction")
	}

	from, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return fmt.Errorf("failed to get sender for transaction %s: %v", tx.Hash().Hex(), err)
	}

	to := tx.To()

	if watchAddresses[from] {
		if to != nil {
			event := e.processTransaction(tx, from.Hex(), to.Hex(), blockTime)
			if e.EventEmitter != nil {
				e.EventEmitter.EmitEvent(event)
				logger.Log.Info().
					Str("from", from.Hex()).
					Str("to", to.Hex()).
					Str("txHash", tx.Hash().Hex()).
					Msg("Emitted outgoing transaction event")
			} else {
				logger.Log.Warn().Msg("EventEmitter is nil, cannot emit event")
			}
		}
	}

	if to != nil && watchAddresses[*to] {
		event := e.processTransaction(tx, from.Hex(), to.Hex(), blockTime)
		if e.EventEmitter != nil {
			e.EventEmitter.EmitEvent(event)
			logger.Log.Info().
				Str("from", from.Hex()).
				Str("to", to.Hex()).
				Str("txHash", tx.Hash().Hex()).
				Msg("Emitted incoming transaction event")
		} else {
			logger.Log.Warn().Msg("EventEmitter is nil, cannot emit event")
		}
	}

	return nil
}

func (e *EthereumMonitor) processTransaction(tx *types.Transaction, from, to string, blockTime uint64) models.TransactionEvent {
	value := new(big.Float).Quo(
		new(big.Float).SetInt(tx.Value()),
		new(big.Float).SetFloat64(1e18),
	)
	valueStr := value.Text('f', 18)
	valueStr = strings.TrimRight(strings.TrimRight(valueStr, "0"), ".")

	var gasPrice *big.Int
	if tx.Type() == types.DynamicFeeTxType {
		gasPrice = tx.GasFeeCap()
	} else {
		gasPrice = tx.GasPrice()
	}

	gasPriceFloat := new(big.Float).Quo(
		new(big.Float).SetInt(gasPrice),
		new(big.Float).SetFloat64(1e18),
	)
	gasUsed := new(big.Float).SetUint64(tx.Gas())
	fees := new(big.Float).Mul(gasPriceFloat, gasUsed)
	feesStr := fees.Text('f', 18)
	feesStr = strings.TrimRight(strings.TrimRight(feesStr, "0"), ".")

	return models.TransactionEvent{
		From:      from,
		To:        to,
		Amount:    valueStr,
		Fees:      feesStr,
		Chain:     e.GetChainName(),
		TxHash:    tx.Hash().Hex(),
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

func (e *EthereumMonitor) getBalance(address string) (*big.Int, uint64, error) {
	blockNumber, err := e.client.BlockNumber(context.Background())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get latest block number: %v", err)
	}

	account := common.HexToAddress(address)
	balance, err := e.client.BalanceAt(context.Background(), account, big.NewInt(int64(blockNumber)))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get balance: %v", err)
	}

	return balance, blockNumber, nil
}

func NewEthereumMonitor() *EthereumMonitor {
	return &EthereumMonitor{
		BaseMonitor: BaseMonitor{
			RpcEndpoint: os.Getenv("ETHEREUM_RPC_ENDPOINT"),
			ApiKey:      os.Getenv("ETHEREUM_API_KEY"),
			Addresses:   []string{"0x00000000219ab540356cBB839Cbe05303d7705Fa"},
		},
		MaxRetries: 1,
		RetryDelay: 2 * time.Second,
	}
}

func (e *EthereumMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			logger.Log.Info().Msg("Ethereum monitor shutting down")
			return nil
		default:
			e.EventEmitter = emitter

			if err := e.Initialize(); err != nil {
				logger.Log.Fatal().Err(err).Msg("Failed to initialize Ethereum monitor")
				return err
			}
			if err := e.StartMonitoring(); err != nil {
				logger.Log.Fatal().Err(err).Msg("Failed to start Ethereum monitoring")
				return err
			}
			return nil
		}
	}
}
