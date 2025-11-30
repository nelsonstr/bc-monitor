package models

type BlockchainName string

const (
	Ethereum BlockchainName = "Ethereum"
	Bitcoin  BlockchainName = "Bitcoin"
	Solana   BlockchainName = "Solana"
	Polygon  BlockchainName = "Polygon"
	BSC      BlockchainName = "BSC"
)

func (b BlockchainName) String() string {
	return string(b)
}
