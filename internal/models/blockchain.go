package models

type BlockchainName string

const (
	Ethereum BlockchainName = "Ethereum"
	Bitcoin  BlockchainName = "Bitcoin"
	Solana   BlockchainName = "Solana"
)

func (b BlockchainName) String() string {
	return string(b)
}
