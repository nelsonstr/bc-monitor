package bitcoin

import (
	"blockchain-monitor/internal/events"
	"blockchain-monitor/internal/models"
	"blockchain-monitor/internal/monitors"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// MockEventEmitter is a mock implementation of EventEmitter for testing
type MockEventEmitter struct {
	emittedEvents []models.TransactionEvent
	emitError     error
	closeError    error
	mu            sync.Mutex
}

func (m *MockEventEmitter) EmitEvent(event models.TransactionEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.emitError != nil {
		return m.emitError
	}
	m.emittedEvents = append(m.emittedEvents, event)
	return nil
}

func (m *MockEventEmitter) Close() error {
	return m.closeError
}

func (m *MockEventEmitter) GetEmittedEvents() []models.TransactionEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := make([]models.TransactionEvent, len(m.emittedEvents))
	copy(events, m.emittedEvents)
	return events
}

func (m *MockEventEmitter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emittedEvents = nil
}

// setupTestMonitor creates a test monitor with mocked dependencies
func setupTestMonitor() (*BitcoinMonitor, *MockEventEmitter, *httptest.Server) {
	logger := zerolog.New(nil)
	emitter := &MockEventEmitter{}

	baseMonitor := monitors.NewBaseMonitor(
		models.Bitcoin,
		100, // high rate limit for tests
		"http://localhost:8332",
		"",
		"https://blockchair.com/bitcoin",
		&logger,
		emitter,
	)

	monitor := NewBitcoinMonitor(baseMonitor)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}

		var response models.RPCResponse
		response.Jsonrpc = "2.0"
		response.ID = req.ID

		switch req.Method {
		case "getbestblockhash":
			response.Result = json.RawMessage(`"0000000000000000000abc123def456"`)
		case "getblockcount":
			response.Result = json.RawMessage(`850000`)
		case "getblock":
			// Mock block response
			block := BlockDetails{
				Hash:   "0000000000000000000abc123def456",
				Tx:     []string{"tx1", "tx2"},
				Height: 850000,
			}
			result, _ := json.Marshal(block)
			response.Result = result
		case "getrawtransaction":
			// Mock transaction response
			tx := TransactionDetails{
				Txid: "tx1",
				Time: time.Now().Unix(),
				Vin: []Vin{
					{TxID: "prevtx", Vout: 0},
				},
				Vout: []Vout{
					{
						Value: 0.5,
						N:     0,
						ScriptPubKey: ScriptPubKey{
							Address: "1ABC...",
						},
					},
				},
			}
			result, _ := json.Marshal(tx)
			response.Result = result
		default:
			response.Error = &models.RPCError{Code: -32601, Message: "Method not found"}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))

	// Override the RPC endpoint
	baseMonitor.RpcEndpoint = server.URL

	return monitor, emitter, server
}

func TestNewBitcoinMonitor(t *testing.T) {
	logger := zerolog.New(nil)
	emitter := &MockEventEmitter{}

	baseMonitor := monitors.NewBaseMonitor(
		models.Bitcoin,
		10,
		"http://localhost:8332",
		"testkey",
		"https://blockchair.com/bitcoin",
		&logger,
		emitter,
	)

	monitor := NewBitcoinMonitor(baseMonitor)

	if monitor == nil {
		t.Fatal("NewBitcoinMonitor returned nil")
	}

	if monitor.BaseMonitor != baseMonitor {
		t.Error("BaseMonitor not set correctly")
	}

	if monitor.GetChainName() != models.Bitcoin {
		t.Errorf("Expected chain name Bitcoin, got %s", monitor.GetChainName())
	}
}

func TestBitcoinMonitor_GetExplorerURL(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	tests := []struct {
		name     string
		txHash   string
		expected string
	}{
		{
			name:     "valid tx hash",
			txHash:   "abc123",
			expected: "https://blockchair.com/bitcoin/transaction/abc123",
		},
		{
			name:     "empty tx hash",
			txHash:   "",
			expected: "https://blockchair.com/bitcoin/transaction/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := monitor.GetExplorerURL(tt.txHash)
			if result != tt.expected {
				t.Errorf("GetExplorerURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBitcoinMonitor_getBestBlockHash(t *testing.T) {
	monitor, _, server := setupTestMonitor()
	defer server.Close()

	hash, err := monitor.getBestBlockHash()
	if err != nil {
		t.Fatalf("getBestBlockHash() error = %v", err)
	}

	expected := "0000000000000000000abc123def456"
	if hash != expected {
		t.Errorf("getBestBlockHash() = %v, want %v", hash, expected)
	}
}

func TestBitcoinMonitor_GetBlockHead(t *testing.T) {
	monitor, _, server := setupTestMonitor()
	defer server.Close()

	height, err := monitor.GetBlockHead()
	if err != nil {
		t.Fatalf("GetBlockHead() error = %v", err)
	}

	expected := uint64(850000)
	if height != expected {
		t.Errorf("GetBlockHead() = %v, want %v", height, expected)
	}
}

func TestBitcoinMonitor_getBlock(t *testing.T) {
	monitor, _, server := setupTestMonitor()
	defer server.Close()

	block, err := monitor.getBlock("testhash")
	if err != nil {
		t.Fatalf("getBlock() error = %v", err)
	}

	if block.Hash != "0000000000000000000abc123def456" {
		t.Errorf("Block hash = %v, want %v", block.Hash, "0000000000000000000abc123def456")
	}

	if len(block.Tx) != 2 {
		t.Errorf("Expected 2 transactions, got %d", len(block.Tx))
	}

	if block.Height != 850000 {
		t.Errorf("Block height = %v, want %v", block.Height, 850000)
	}
}

func TestBitcoinMonitor_getTransaction(t *testing.T) {
	monitor, _, server := setupTestMonitor()
	defer server.Close()

	tx, err := monitor.getTransaction("tx1")
	if err != nil {
		t.Fatalf("getTransaction() error = %v", err)
	}

	if tx.Txid != "tx1" {
		t.Errorf("Transaction ID = %v, want %v", tx.Txid, "tx1")
	}

	if len(tx.Vin) != 1 {
		t.Errorf("Expected 1 input, got %d", len(tx.Vin))
	}

	if len(tx.Vout) != 1 {
		t.Errorf("Expected 1 output, got %d", len(tx.Vout))
	}
}

func TestBitcoinMonitor_getPrevOutputValue(t *testing.T) {
	monitor, _, server := setupTestMonitor()
	defer server.Close()

	addr, value, err := monitor.getPrevOutputValue("prevtx", 0)
	if err != nil {
		t.Fatalf("getPrevOutputValue() error = %v", err)
	}

	if addr != "1ABC..." {
		t.Errorf("Address = %v, want %v", addr, "1ABC...")
	}

	if value != 0.5 {
		t.Errorf("Value = %v, want %v", value, 0.5)
	}
}

func TestBitcoinMonitor_AddAddress(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	// Test adding new address
	err := monitor.AddAddress("testaddr1")
	if err != nil {
		t.Fatalf("AddAddress() error = %v", err)
	}

	if !monitor.IsWatchedAddress("testaddr1") {
		t.Error("Address should be watched after adding")
	}

	// Test adding duplicate address
	err = monitor.AddAddress("testaddr1")
	if err != nil {
		t.Fatalf("AddAddress() duplicate error = %v", err)
	}

	// Test non-watched address
	if monitor.IsWatchedAddress("testaddr2") {
		t.Error("Address should not be watched")
	}
}

func TestBitcoinMonitor_emitTransactionEvent(t *testing.T) {
	monitor, emitter, _ := setupTestMonitor()

	tx := &TransactionDetails{
		Txid: "testtx",
		Time: time.Now().Unix(),
		Vout: []Vout{
			{
				Value: 1.0,
				ScriptPubKey: ScriptPubKey{
					Address: "toaddr",
				},
			},
		},
	}

	monitor.emitTransactionEvent(tx, "fromaddr", "toaddr", 1.0, 0.001)

	events := emitter.GetEmittedEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.TxHash != "testtx" {
		t.Errorf("TxHash = %v, want %v", event.TxHash, "testtx")
	}

	if event.From != "fromaddr" {
		t.Errorf("From = %v, want %v", event.From, "fromaddr")
	}

	if event.To != "toaddr" {
		t.Errorf("To = %v, want %v", event.To, "toaddr")
	}

	if event.Amount != "1.000000" {
		t.Errorf("Amount = %v, want %v", event.Amount, "1.000000")
	}

	if event.Fees != "0.001000" {
		t.Errorf("Fees = %v, want %v", event.Fees, "0.001000")
	}

	if event.Chain != models.Bitcoin {
		t.Errorf("Chain = %v, want %v", event.Chain, models.Bitcoin)
	}
}

func TestBitcoinMonitor_Stop(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	err := monitor.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// Test error cases
func TestBitcoinMonitor_getBestBlockHash_Error(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	// Override with error server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", 500)
	}))
	defer server.Close()

	monitor.BaseMonitor.RpcEndpoint = server.URL

	_, err := monitor.getBestBlockHash()
	if err == nil {
		t.Error("Expected error from getBestBlockHash")
	}
}

func TestBitcoinMonitor_getBlock_Error(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	// Override with error server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", 500)
	}))
	defer server.Close()

	monitor.BaseMonitor.RpcEndpoint = server.URL

	_, err := monitor.getBlock("testhash")
	if err == nil {
		t.Error("Expected error from getBlock")
	}
}

func TestBitcoinMonitor_getTransaction_Error(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	// Override with error server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", 500)
	}))
	defer server.Close()

	monitor.BaseMonitor.RpcEndpoint = server.URL

	_, err := monitor.getTransaction("tx1")
	if err == nil {
		t.Error("Expected error from getTransaction")
	}
}

func TestBitcoinMonitor_getPrevOutputValue_Error(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	// Override with error server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", 500)
	}))
	defer server.Close()

	monitor.BaseMonitor.RpcEndpoint = server.URL

	_, _, err := monitor.getPrevOutputValue("prevtx", 0)
	if err == nil {
		t.Error("Expected error from getPrevOutputValue")
	}
}

// Test Initialize method
func TestBitcoinMonitor_Initialize(t *testing.T) {
	monitor, _, server := setupTestMonitor()
	defer server.Close()

	err := monitor.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if monitor.latestBlockHash == "" {
		t.Error("latestBlockHash should be set")
	}

	if monitor.latestBlockHeight == 0 {
		t.Error("latestBlockHeight should be set")
	}
}

func TestBitcoinMonitor_Initialize_Error_GetBestBlockHash(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	// Override with error server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", 500)
	}))
	defer server.Close()

	monitor.BaseMonitor.RpcEndpoint = server.URL

	err := monitor.Initialize()
	if err == nil {
		t.Error("Expected error from Initialize when getBestBlockHash fails")
	}
}

func TestBitcoinMonitor_Initialize_Error_GetBlockHead(t *testing.T) {
	monitor, _, _ := setupTestMonitor()

	// Server that fails on getblockcount
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req models.RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		response := models.RPCResponse{
			Jsonrpc: "2.0",
			ID:      req.ID,
		}

		if req.Method == "getbestblockhash" {
			response.Result = json.RawMessage(`"hash"`)
		} else {
			http.Error(w, "internal server error", 500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	monitor.BaseMonitor.RpcEndpoint = server.URL

	err := monitor.Initialize()
	if err == nil {
		t.Error("Expected error from Initialize when GetBlockHead fails")
	}
}

// Test processBlock
func TestBitcoinMonitor_processBlock(t *testing.T) {
	monitor, emitter, server := setupTestMonitor()
	defer server.Close()

	emitter.Reset()

	height, err := monitor.processBlock("testhash")
	if err != nil {
		t.Fatalf("processBlock() error = %v", err)
	}

	if height != 850000 {
		t.Errorf("processBlock() height = %v, want %v", height, 850000)
	}

	// Check if events were emitted (depending on mock setup)
	events := emitter.GetEmittedEvents()
	// In this test setup, events may or may not be emitted depending on addresses
	// Just ensure no error occurred
}

// Test processTransaction
func TestBitcoinMonitor_processTransaction(t *testing.T) {
	monitor, emitter, server := setupTestMonitor()
	defer server.Close()

	emitter.Reset()

	// Add a watched address
	monitor.AddAddress("1ABC...")

	err := monitor.processTransaction("tx1")
	if err != nil {
		t.Fatalf("processTransaction() error = %v", err)
	}

	// Check if event was emitted
	events := emitter.GetEmittedEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}