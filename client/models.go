package client

type Status struct {
	Info Info `json:"info"`
}

type Info struct {
	Version         int         `json:"version"`
	ProtocolVersion int         `json:"protocolversion"`
	Blocks          int         `json:"blocks"`
	TimeOffset      int         `json:"timeoffset"`
	Connections     int         `json:"connections"`
	DifficultyIface interface{} `json:"difficulty"`
	Difficulty      float64
	Testnet         bool        `json:"testnet"`
	RelayFeeIface   interface{} `json:"relayfee"`
	RelayFee        float64
	Errors          string `json:"errors"`
	Network         string `json:"network"`
}

type Block struct {
	Hash string `json:"hash"`
}

type Utxo struct {
	Address       string      `json:"address"`
	Txid          string      `json:"txid"`
	Vout          int         `json:"vout"`
	ScriptPubKey  string      `json:"scriptPubKey"`
	AmountIface   interface{} `json:"amount"`
	Amount        float64
	Satoshis      int64 `json:"satoshis"`
	Confirmations int   `json:"confirmations"`
}

type TransactionList struct {
	TotalItems int           `json:"totalItems"`
	From       int           `json:"from"`
	To         int           `json:"to"`
	Items      []Transaction `json:"items"`
}

type Transaction struct {
	Txid          string   `json:"txid"`
	Version       int      `json:"version"`
	Locktime      int      `json:"locktime"`
	Inputs        []Input  `json:"vin"`
	Outputs       []Output `json:"vout"`
	BlockHash     string   `json:"blockhash"`
	BlockHeight   int      `json:"blockheight"`
	Confirmations int      `json:"confirmations"`
	Time          int64    `json:"time"`
	BlockTime     int64    `json:"blocktime"`
}

type Input struct {
	Txid            string      `json:"txid"`
	Vout            int         `json:"vout"`
	Sequence        int         `json:"sequence"`
	N               int         `json:"n"`
	ScriptSig       Script      `json:"scriptSig"`
	Addr            string      `json:"addr"`
	Satoshis        int64       `json:"valueSat"`
	ValueIface      interface{} `json:"value"`
	Value           float64
	DoubleSpentTxid string `json:"doubleSpentTxID"`
}

type Output struct {
	ValueIface   interface{} `json:"value"`
	Value        float64
	N            int       `json:"n"`
	ScriptPubKey OutScript `json:"scriptPubKey"`
	SpentTxid    string    `json:"spentTxId"`
	SpentIndex   int       `json:"spentIndex"`
	SpentHeight  int       `json:"spentHeight"`
}

type Script struct {
	Hex string `json:"hex"`
	Asm string `json:"asm"`
}

type OutScript struct {
	Script
	Addresses []string `json:"addresses"`
	Type      string   `json:"type"`
}
