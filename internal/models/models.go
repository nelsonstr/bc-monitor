package models

import (
	"time"
)

type User struct {
	Id string

	Addresess map[BlockchainName][]string
}

// TransactionEvent represents a blockchain transaction event
type TransactionEvent struct {
	From        string
	To          string
	Amount      string
	Fees        string
	Chain       BlockchainName
	TxHash      string
	Timestamp   time.Time
	ExplorerURL string
}
