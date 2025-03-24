package solana

import (
	"time"
)

type AccountChange struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Result struct {
			Value struct {
				Lamports uint64 `json:"lamports"`
			} `json:"value"`
		} `json:"result"`
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Subscription uint64 `json:"subscription"`
	} `json:"params"`
}

type SolanaTransaction struct {
	BlockTime   int64  `json:"blockTime"`
	Meta        Meta   `json:"meta"`
	Slot        uint64 `json:"slot"`
	Transaction struct {
		Message struct {
			AccountKeys  []string `json:"accountKeys"`
			Instructions []struct {
				ProgramIdIndex int    `json:"programIdIndex"`
				Accounts       []int  `json:"accounts"`
				Data           string `json:"data"`
			} `json:"instructions"`
		} `json:"message"`
		Signatures []string `json:"signatures"`
	} `json:"transaction"`
}

type Meta struct {
	Fee          uint64   `json:"fee"`
	PreBalances  []uint64 `json:"preBalances"`
	PostBalances []uint64 `json:"postBalances"`
}

type SolanaTransactionDetails struct {
	From      string
	To        string
	Amount    float64
	Fees      float64
	TxHash    string
	Timestamp time.Time
}
