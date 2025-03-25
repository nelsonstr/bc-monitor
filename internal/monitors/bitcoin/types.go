package bitcoin

type TransactionDetails struct {
	Txid string `json:"txid"`
	Vin  []Vin  `json:"vin"`
	Vout []Vout `json:"vout"`
	Time int64  `json:"time"`
}

type Vin struct {
	TxID string `json:"txid"`
	Vout int    `json:"vout"`
}

type Vout struct {
	Value        float64      `json:"value"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	N            int          `json:"n"`
}

type ScriptPubKey struct {
	Address string `json:"address"`
}

type BlockDetails struct {
	Hash   string   `json:"hash"`
	Tx     []string `json:"tx"`
	Height uint64   `json:"height"`
}
