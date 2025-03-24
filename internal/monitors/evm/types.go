package evm

type EthereumBlockDetails struct {
	Hash          string                `json:"hash"`
	Transactions  []EthereumTransaction `json:"transactions"`
	Number        string                `json:"number"`
	Timestamp     string                `json:"timestamp"`
	BlobGasUsed   string                `json:"blobGasUsed"`
	Difficulty    string                `json:"difficulty"`
	ExcessBlobGas string                `json:"excessBlobGas"`
	GasUsed       string                `json:"gasUsed"`
	Nonce         string                `json:"nonce"`
	Size          string                `json:"size"`
	StateRoot     string                `json:"stateRoot"`
	BaseFeePerGas string                `json:"baseFeePerGas"`
}

type EthereumTransaction struct {
	BlockNumber          string `json:"blockNumber"`
	From                 string `json:"from"`
	Gas                  string `json:"gas"`
	GasPrice             string `json:"gasPrice"`
	Hash                 string `json:"hash"`
	Input                string `json:"input"`
	Nonce                string `json:"nonce"`
	To                   string `json:"to"`
	TransactionIndex     string `json:"transactionIndex"`
	Value                string `json:"value"`
	Type                 string `json:"type"`
	ChainId              string `json:"chainId"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
}
