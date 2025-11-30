package models

import (
	"math/big"
	"time"
)

type User struct {
	ID        string
	Addresses map[BlockchainName][]string
}

// TransactionEvent represents a blockchain transaction event
type TransactionEvent struct {
	From        string
	To          string
	Amount      *big.Float
	Fees        *big.Float
	Chain       BlockchainName
	TxHash      string
	Timestamp   time.Time
	ExplorerURL string
}
