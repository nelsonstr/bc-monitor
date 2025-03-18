package models

import (
	"time"
)

type User struct {
	id              string
	EthereumAddress []string
	SolanaAddress   []string
	Bitcoin         []string
}

// TransactionEvent represents a blockchain transaction event
type TransactionEvent struct {
	From      string
	To        string
	Amount    string
	Fees      string
	Chain     string
	TxHash    string
	Timestamp time.Time
}
