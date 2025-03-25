package health

import (
	"blockchain-monitor/internal/interfaces"
	"blockchain-monitor/internal/logger"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type BlockchainStatus struct {
	Name      string `json:"name"`
	LastBlock uint64 `json:"last_block"`
}

var (
	isReady            int32
	blockchainStatuses = make(map[string]*BlockchainStatus)
	statusMutex        sync.RWMutex
)

func SetReady(ready bool) {
	if ready {
		atomic.StoreInt32(&isReady, 1)
	} else {
		atomic.StoreInt32(&isReady, 0)
	}
}

func LivenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func ReadinessHandler(w http.ResponseWriter, _ *http.Request) {
	statusMutex.RLock()
	defer statusMutex.RUnlock()

	if len(blockchainStatuses) == 0 || atomic.LoadInt32(&isReady) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("Not Ready"))

		return
	}

	response := make(map[string]interface{})
	response["status"] = "Ready"
	response["blockchains"] = blockchainStatuses

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func RegisterMonitor(ctx context.Context, monitor interfaces.BlockchainMonitor) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				blockhead, err := monitor.GetBlockHead()
				if err != nil {
					logger.GetLogger().Error().
						Err(err).
						Str("chain", monitor.GetChainName().String()).
						Msg("Error getting latest block/slot")
				} else {
					updateBlockchainStatus(monitor.GetChainName().String(), blockhead)
				}
				time.Sleep(10 * time.Second)
			}
		}
	}()
}

func updateBlockchainStatus(name string, lastBlockOrSlot uint64) {
	statusMutex.Lock()
	defer statusMutex.Unlock()
	blockchainStatuses[name] = &BlockchainStatus{
		Name:      name,
		LastBlock: lastBlockOrSlot,
	}
}
