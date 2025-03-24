package solana

import (
	"time"
)

type TransactionDetailsRaw struct {
	Meta        Meta `json:"meta"`
	Transaction struct {
		Message struct {
			AccountKeys []struct {
				Pubkey   string `json:"pubkey"`
				Signer   bool   `json:"signer"`
				Source   string `json:"source"`
				Writable bool   `json:"writable"`
			} `json:"accountKeys"`
			Instructions []struct {
				ProgramIdIndex int      `json:"programIdIndex"`
				Accounts       []string `json:"accounts"`
				Data           string   `json:"data"`
			} `json:"instructions"`
		} `json:"message"`
	} `json:"transaction"`
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
