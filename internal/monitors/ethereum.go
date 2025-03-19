package monitors

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/models"
	"context"
	"fmt"
	"log"
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

// EthereumMonitor implements BlockchainMonitor for Start
// EthereumMonitor implements BlockchainMonitor for Ethereum
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

	// Create a custom HTTP client
	httpClient := &http.Client{
		Transport: &http.Transport{},
	}

	// Create a custom RoundTripper to add headers
	customTransport := &customTransport{
		base:   http.DefaultTransport,
		apiKey: e.ApiKey,
	}

	httpClient.Transport = customTransport

	// Create a custom rpc.Client
	rpcClient, err := rpc.DialHTTPWithClient(e.RpcEndpoint, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create RPC client: %v\n", err)
	}

	// Create ethclient.Client using the custom rpc.Client
	client := ethclient.NewClient(rpcClient)

	e.client = client
	return nil
}

func (e *EthereumMonitor) GetChainName() string {
	return "Ethereum"
}

func (e *EthereumMonitor) StartMonitoring() error {
	log.Printf("Starting %s monitoring for %d addresses", e.GetChainName(), len(e.Addresses))

	for _, address := range e.Addresses {
		balance, blockNumber, err := e.getBalance(address)
		if err != nil {
			log.Printf("Error getting initial balance for %s: %v", address, err)
		} else {
			ethBalance := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
			log.Printf("Initial balance for %s: %s ETH at block %d", address, ethBalance.Text('f', 18), blockNumber)
		}
	}

	// Convert string addresses to common.Address
	watchAddresses := make(map[common.Address]bool)
	for _, addr := range e.Addresses {
		watchAddresses[common.HexToAddress(addr)] = true
	}

	// Get the latest block number
	latestBlock, err := e.client.BlockNumber(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get latest Start block: %v\n", err)
	}

	e.latestBlock = latestBlock
	log.Printf("Starting %s monitoring from block %d", e.GetChainName(), latestBlock)

	go func() {
		for {
			// Get the current block number
			currentBlock, err := e.client.BlockNumber(context.Background())
			if err != nil {
				log.Printf("Failed to get current Start block: %v\n", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Process new blocks
			for blockNum := e.latestBlock + 1; blockNum <= currentBlock; blockNum++ {
				if err := e.processBlock(blockNum, watchAddresses); err != nil {
					log.Printf("Ethereum - Error processing block %d: %v\n", blockNum, err)
					// Continue to next block if this one fails
					continue
				}
				// Update latest processed block
				e.latestBlock = blockNum
			}

			// Wait before checking for new blocks
			time.Sleep(15 * time.Second)
		}
	}()

	return nil
}

func (e *EthereumMonitor) processBlock(blockNum uint64, watchAddresses map[common.Address]bool) error {
	var block *types.Block
	var err error

	// Try to get the block with retries
	for attempt := 0; attempt <= e.MaxRetries; attempt++ {
		block, err = e.client.BlockByNumber(context.Background(), big.NewInt(int64(blockNum)))
		if err == nil {
			break
		}

		if attempt < e.MaxRetries {
			log.Printf("Ethereum - Attempt %d: Failed to get block %d: %v. Retrying...\n", attempt+1, blockNum, err)
			time.Sleep(e.RetryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("Ethereum - failed to get block %d after %d attempts: %v\n", blockNum, e.MaxRetries+1, err)
	}

	log.Printf("Processing %s block %d with %d transactions\n", e.GetChainName(), blockNum, len(block.Transactions()))

	// Process each transaction in the block
	for _, tx := range block.Transactions() {
		// Try to process the transaction, but don't fail the whole block if one tx fails
		// try process with go routines
		if err := e.processSingleTransaction(tx, watchAddresses, block.Time()); err != nil {
			log.Printf("Warning: %v\n", err)
			continue
		}
	}

	return nil
}

func (e *EthereumMonitor) processSingleTransaction(tx *types.Transaction, watchAddresses map[common.Address]bool, blockTime uint64) error {
	if tx == nil {
		return fmt.Errorf("received nil transaction\n")
	}

	// Get the sender address
	from, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return fmt.Errorf("failed to get sender for transaction %s: %v\n", tx.Hash().Hex(), err)
	}

	to := tx.To()

	// Check if sender is in our watch list
	if watchAddresses[from] {
		if to != nil {
			event := e.processTransaction(tx, from.Hex(), to.Hex(), blockTime)
			if e.EventEmitter != nil {
				e.EventEmitter.EmitEvent(event)
			} else {
				log.Printf("Warning: EventEmitter is nil, cannot emit event\n")
			}
			log.Printf("Detected outgoing transaction from watched address %s\n", from.Hex())
		}
	}

	// Check if recipient is in our watch list
	if to != nil && watchAddresses[*to] {
		event := e.processTransaction(tx, from.Hex(), to.Hex(), blockTime)
		if e.EventEmitter != nil {
			e.EventEmitter.EmitEvent(event)
		} else {
			log.Printf("Warning: EventEmitter is nil, cannot emit event\n")
		}
		log.Printf("Detected incoming transaction to watched address %s\n", to.Hex())
	}

	return nil
}

func (e *EthereumMonitor) processTransaction(tx *types.Transaction, from, to string, blockTime uint64) models.TransactionEvent {
	// Get transaction value in ETH
	value := new(big.Float).Quo(
		new(big.Float).SetInt(tx.Value()),
		new(big.Float).SetFloat64(1e18), // Convert from Wei to ETH
	)
	valueStr := value.Text('f', 18)
	valueStr = strings.TrimRight(strings.TrimRight(valueStr, "0"), ".")

	// Calculate gas fees
	var gasPrice *big.Int
	if tx.Type() == types.DynamicFeeTxType {
		gasPrice = tx.GasFeeCap()
	} else {
		gasPrice = tx.GasPrice()
	}

	gasPriceFloat := new(big.Float).Quo(
		new(big.Float).SetInt(gasPrice),
		new(big.Float).SetFloat64(1e18), // Convert from Wei to ETH
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

// Custom RoundTripper to add headers
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
	// Get the latest block number
	blockNumber, err := e.client.BlockNumber(context.Background())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get latest block number: %v", err)
	}

	// Get the balance at the latest block
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
			ApiKey:      os.Getenv("BLOCKDAEMON_API_KEY"),
			Addresses:   []string{"0x00000000219ab540356cBB839Cbe05303d7705Fa"},
		},

		MaxRetries: 5,
		RetryDelay: 5 * time.Second,
	}
}

func (e *EthereumMonitor) Start(ctx context.Context, emitter interfaces.EventEmitter) error {
	for {
		select {
		case <-ctx.Done():
			log.Printf("%s monitor shutting down", e.GetChainName())
			return nil
		default:
			// Set the EventEmitter
			e.EventEmitter = emitter

			// Initialize Start monitor
			if err := e.Initialize(); err != nil {
				log.Fatalf("Failed to initialize Start monitor: %v", err)
				return err
			}
			// Start monitoring Start blockchain
			if err := e.StartMonitoring(); err != nil {
				log.Fatalf("Failed to start Start monitoring: %v", err)
				return err
			}
			return nil
		}
	}
}
