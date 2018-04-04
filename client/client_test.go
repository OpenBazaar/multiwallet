package client

import (
	"fmt"
	"github.com/OpenBazaar/golang-socketio"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"gopkg.in/jarcoal/httpmock.v1"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func NewTestClient() *InsightClient {
	u, _ := url.Parse("http://localhost:8334/")
	return &InsightClient{
		httpClient:      http.Client{},
		apiUrl:          *u,
		blockNotifyChan: make(chan Block),
		txNotifyChan:    make(chan Transaction),
		socketClient:    &gosocketio.Client{},
	}
}

func setup() {
	httpmock.Activate()
}

func teardown() {
	httpmock.DeactivateAndReset()
}

var TestTx = Transaction{
	Txid:     "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428",
	Version:  2,
	Locktime: 512378,
	Inputs: []Input{
		{
			Txid:       "6d892f04fc097f430d58ab06229c9b6344a130fc1842da5b990e857daed42194",
			Vout:       1,
			Sequence:   1,
			ValueIface: "0.04294455",
			ScriptSig: Script{
				Hex: "4830450221008665481674067564ef562cfd8d1ca8f1506133fb26a2319e4b8dfba3cedfd5de022038f27121c44e6c64b93b94d72620e11b9de35fd864730175db9176ca98f1ec610121022023e49335a0dddb864ff673468a6cc04e282571b1227933fcf3ff9babbcc662",
			},
			Addr:     "1C74Gbij8Q5h61W58aSKGvXK4rk82T2A3y",
			Satoshis: 4294455,
		},
	},
	Outputs: []Output{
		{
			ScriptPubKey: OutScript{
				Script: Script{
					Hex: "76a914ff3f7d402fbd6d116ba4a02af9784f3ae9b7108a88ac",
				},
				Type:      "pay-to-pubkey-hash",
				Addresses: []string{"1QGdNEDjWnghrjfTBCTDAPZZ3ffoKvGc9B"},
			},
			ValueIface: "0.01398175",
		},
		{
			ScriptPubKey: OutScript{
				Script: Script{
					Hex: "a9148a62462d08a977fa89226a56fca7eb01b6fef67c87",
				},
				Type:      "pay-to-script-hashh",
				Addresses: []string{"3EJiuDqsHuAtFqiLGWKVyCfvqoGpWVCCRs"},
			},
			ValueIface: "0.02717080",
		},
	},
	Time:          1520449061,
	BlockHash:     "0000000000000000003f1fb88ac3dab0e607e87def0e9031f7bea02cb464a04f",
	BlockHeight:   512476,
	Confirmations: 1,
}

func TestInsightClient_GetTransaction(t *testing.T) {
	setup()
	defer teardown()

	var (
		c          = NewTestClient()
		testPath   = fmt.Sprintf("http://%s/tx/1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428", c.apiUrl.Host)
		expectedTx = TestTx
	)

	response, err := httpmock.NewJsonResponse(http.StatusOK, expectedTx)
	if err != nil {
		t.Error(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	tx, err := c.GetTransaction("1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428")
	if err != nil {
		t.Error(err)
	}
	validateTransaction(*tx, expectedTx, t)
}

func TestInsightClient_GetTransactions(t *testing.T) {
	setup()
	defer teardown()

	var (
		c        = NewTestClient()
		testPath = fmt.Sprintf("http://%s/addrs/txs", c.apiUrl.Host)
		expected = TransactionList{
			TotalItems: 1,
			From:       0,
			To:         1,
			Items: []Transaction{
				TestTx,
			},
		}
	)

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Error(err)
	}

	httpmock.RegisterResponder(http.MethodPost, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	addr, err := btcutil.DecodeAddress("1C74Gbij8Q5h61W58aSKGvXK4rk82T2A3y", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	txs, err := c.GetTransactions([]btcutil.Address{addr})
	if err != nil {
		t.Error(err)
	}
	if len(txs) != 1 {
		t.Error("Returned incorrect number of transactions")
		return
	}
	validateTransaction(txs[0], expected.Items[0], t)
}

func validateTransaction(tx, expectedTx Transaction, t *testing.T) {
	if tx.Txid != expectedTx.Txid {
		t.Error("Returned invalid transaction")
	}
	if tx.Version != expectedTx.Version {
		t.Error("Returned invalid transaction")
	}
	if tx.Locktime != expectedTx.Locktime {
		t.Error("Returned invalid transaction")
	}
	if tx.Time != expectedTx.Time {
		t.Error("Returned invalid transaction")
	}
	if tx.BlockHash != expectedTx.BlockHash {
		t.Error("Returned invalid transaction")
	}
	if tx.BlockHeight != expectedTx.BlockHeight {
		t.Error("Returned invalid transaction")
	}
	if tx.Confirmations != expectedTx.Confirmations {
		t.Error("Returned invalid transaction")
	}
	if len(tx.Inputs) != 1 {
		t.Error("Returned incorrect number of inputs")
		return
	}
	if tx.Inputs[0].Txid != expectedTx.Inputs[0].Txid {
		t.Error("Returned invalid transaction")
	}
	if tx.Inputs[0].Value != 0.04294455 {
		t.Error("Returned invalid transaction")
	}
	if tx.Inputs[0].Satoshis != expectedTx.Inputs[0].Satoshis {
		t.Error("Returned invalid transaction")
	}
	if tx.Inputs[0].Addr != expectedTx.Inputs[0].Addr {
		t.Error("Returned invalid transaction")
	}
	if tx.Inputs[0].Sequence != expectedTx.Inputs[0].Sequence {
		t.Error("Returned invalid transaction")
	}
	if tx.Inputs[0].Vout != expectedTx.Inputs[0].Vout {
		t.Error("Returned invalid transaction")
	}
	if tx.Inputs[0].ScriptSig.Hex != expectedTx.Inputs[0].ScriptSig.Hex {
		t.Error("Returned invalid transaction")
	}

	if len(tx.Outputs) != 2 {
		t.Error("Returned incorrect number of outputs")
		return
	}
	if tx.Outputs[0].Value != 0.01398175 {
		t.Error("Returned invalid transaction")
	}
	if tx.Outputs[0].ScriptPubKey.Hex != expectedTx.Outputs[0].ScriptPubKey.Hex {
		t.Error("Returned invalid transaction")
	}
	if tx.Outputs[0].ScriptPubKey.Type != expectedTx.Outputs[0].ScriptPubKey.Type {
		t.Error("Returned invalid transaction")
	}
	if tx.Outputs[0].ScriptPubKey.Addresses[0] != expectedTx.Outputs[0].ScriptPubKey.Addresses[0] {
		t.Error("Returned invalid transaction")
	}
	if tx.Outputs[1].Value != 0.02717080 {
		t.Error("Returned invalid transaction")
	}
	if tx.Outputs[1].ScriptPubKey.Hex != expectedTx.Outputs[1].ScriptPubKey.Hex {
		t.Error("Returned invalid transaction")
	}
	if tx.Outputs[1].ScriptPubKey.Type != expectedTx.Outputs[1].ScriptPubKey.Type {
		t.Error("Returned invalid transaction")
	}
	if tx.Outputs[1].ScriptPubKey.Addresses[0] != expectedTx.Outputs[1].ScriptPubKey.Addresses[0] {
		t.Error("Returned invalid transaction")
	}
}

func TestInsightClient_GetUtxos(t *testing.T) {
	setup()
	defer teardown()

	var (
		c        = NewTestClient()
		testPath = fmt.Sprintf("http://%s/addrs/utxo", c.apiUrl.Host)
		expected = []Utxo{
			{
				Address:       "1QGdNEDjWnghrjfTBCTDAPZZ3ffoKvGc9B",
				ScriptPubKey:  "76a914ff3f7d402fbd6d116ba4a02af9784f3ae9b7108a88ac",
				Vout:          0,
				Satoshis:      1398175,
				Confirmations: 1,
				Txid:          "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428",
				AmountIface:   "0.01398175",
			},
		}
	)

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Error(err)
	}

	httpmock.RegisterResponder(http.MethodPost, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	addr, err := btcutil.DecodeAddress("1QGdNEDjWnghrjfTBCTDAPZZ3ffoKvGc9B", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	utxos, err := c.GetUtxos([]btcutil.Address{addr})
	if err != nil {
		t.Error(err)
	}
	if len(utxos) != 1 {
		t.Error("Returned incorrect number of utxos")
	}
	validateUtxo(utxos[0], expected[0], t)
}

func validateUtxo(utxo, expected Utxo, t *testing.T) {
	if utxo.Txid != expected.Txid {
		t.Error("Invalid utxo")
	}
	if utxo.Satoshis != expected.Satoshis {
		t.Error("Invalid utxo")
	}
	if utxo.Confirmations != expected.Confirmations {
		t.Error("Invalid utxo")
	}
	if utxo.Vout != expected.Vout {
		t.Error("Invalid utxo")
	}
	if utxo.ScriptPubKey != expected.ScriptPubKey {
		t.Error("Invalid utxo")
	}
	if utxo.Address != expected.Address {
		t.Error("Invalid utxo")
	}
	if utxo.Amount != 0.01398175 {
		t.Error("Invalid utxo")
	}
}

func TestInsightClient_BlockNotify(t *testing.T) {
	var (
		c        = NewTestClient()
		testHash = "0000000000000000003f1fb88ac3dab0e607e87def0e9031f7bea02cb464a04f"
	)

	go func() {
		c.blockNotifyChan <- Block{Hash: testHash}
	}()

	ticker := time.NewTicker(time.Second)
	select {
	case <-ticker.C:
		t.Error("Timed out waiting for block")
	case b := <-c.BlockNotify():
		if b.Hash != testHash {
			t.Error("Returned incorrect block hash")
		}
	}
}

func TestInsightClient_TransactionNotify(t *testing.T) {
	c := NewTestClient()

	go func() {
		c.txNotifyChan <- TestTx
	}()

	ticker := time.NewTicker(time.Second)
	select {
	case <-ticker.C:
		t.Error("Timed out waiting for tx")
	case b := <-c.TransactionNotify():
		for n, in := range b.Inputs {
			f, err := toFloat(in.ValueIface)
			if err != nil {
				t.Error(err)
			}
			b.Inputs[n].Value = f
		}
		for n, out := range b.Outputs {
			f, err := toFloat(out.ValueIface)
			if err != nil {
				t.Error(err)
			}
			b.Outputs[n].Value = f
		}
		validateTransaction(b, TestTx, t)
	}
}

func TestInsightClient_Broadcast(t *testing.T) {
	setup()
	defer teardown()

	type txid struct {
		Txid string `json:"txid"`
	}

	var (
		c        = NewTestClient()
		testPath = fmt.Sprintf("http://%s/tx/send", c.apiUrl.Host)
		expected = txid{"1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428"}
	)

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Error(err)
	}

	httpmock.RegisterResponder(http.MethodPost, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	id, err := c.Broadcast([]byte{0x00, 0x01, 0x02, 0x03})
	if err != nil {
		t.Error(err)
	}
	if id != expected.Txid {
		t.Error("Returned incorrect txid")
	}
}

func TestInsightClient_GetBestBlock(t *testing.T) {
	setup()
	defer teardown()

	var (
		c        = NewTestClient()
		testPath = fmt.Sprintf("http://%s/blocks", c.apiUrl.Host)
		expected = BlockSummaryList{
			Blocks: []Block{
				{
					Hash:     "00000000000000000108a1f4d4db839702d72f16561b1154600a26c453ecb378",
					Height:   2,
					Time:     12345,
					Size:     200,
					TxLength: 5,
				},
				{
					Hash:     "0000000000c96f193d23fde69a2fff56793e99e23cbd51947828a33e287ff659",
					Height:   1,
					Time:     23456,
					Size:     300,
					TxLength: 6,
				},
			},
		}
	)

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Error(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	best, err := c.GetBestBlock()
	if err != nil {
		t.Error(err)
	}
	if best.TxLength != expected.Blocks[0].TxLength {
		t.Errorf("Invalid block obj")
	}
	if best.Size != expected.Blocks[0].Size {
		t.Errorf("Invalid block obj")
	}
	if best.Time != expected.Blocks[0].Time {
		t.Errorf("Invalid block obj")
	}
	if best.Height != expected.Blocks[0].Height {
		t.Errorf("Invalid block obj")
	}
	if best.Hash != expected.Blocks[0].Hash {
		t.Errorf("Invalid block obj")
	}
	if best.Parent != expected.Blocks[1].Hash {
		t.Errorf("Invalid block obj")
	}
}

func Test_toFloat64(t *testing.T) {
	f, err := toFloat(12.345)
	if err != nil {
		t.Error(err)
	}
	if f != 12.345 {
		t.Error("Returned incorrect float")
	}
	f, err = toFloat("456.789")
	if err != nil {
		t.Error(err)
	}
	if f != 456.789 {
		t.Error("Returned incorrect float")
	}
}

func TestInsightClient_setupListeners(t *testing.T) {
	setup()
	defer teardown()

	var (
		c             = NewTestClient()
		mockSocket    = &MockSocketClient{make(map[string]func(h *gosocketio.Channel, args interface{})), []string{}}
		testBlockPath = fmt.Sprintf("http://%s/blocks", c.apiUrl.Host)
		expected      = BlockSummaryList{
			Blocks: []Block{
				{
					Hash:     "00000000000000000108a1f4d4db839702d72f16561b1154600a26c453ecb378",
					Height:   2,
					Time:     12345,
					Size:     200,
					TxLength: 5,
				},
				{
					Hash:     "0000000000c96f193d23fde69a2fff56793e99e23cbd51947828a33e287ff659",
					Height:   1,
					Time:     23456,
					Size:     300,
					TxLength: 6,
				},
			},
		}
		testTxPath = fmt.Sprintf("http://%s/tx/1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428", c.apiUrl.Host)
		expectedTx = TestTx
	)

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Error(err)
	}
	httpmock.RegisterResponder(http.MethodGet, testBlockPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)
	response2, err := httpmock.NewJsonResponse(http.StatusOK, expectedTx)
	if err != nil {
		t.Error(err)
	}
	httpmock.RegisterResponder(http.MethodGet, testTxPath,
		func(req *http.Request) (*http.Response, error) {
			return response2, nil
		},
	)

	c.socketClient = mockSocket
	c.setupListeners()

	go func() {
		m := make(map[string]interface{})
		m[""] = "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428"
		mockSocket.callbacks["bitcoind/hashblock"](nil, "")
		mockSocket.callbacks["bitcoind/addresstxid"](nil, m)
	}()

	ticker := time.NewTicker(time.Second * 2)
	var best Block
	select {
	case b := <-c.blockNotifyChan:
		best = b
	case <-ticker.C:
		t.Error("Block notify listener timed out")
		return
	}
	if best.TxLength != expected.Blocks[0].TxLength {
		t.Errorf("Invalid block obj")
	}
	if best.Size != expected.Blocks[0].Size {
		t.Errorf("Invalid block obj")
	}
	if best.Time != expected.Blocks[0].Time {
		t.Errorf("Invalid block obj")
	}
	if best.Height != expected.Blocks[0].Height {
		t.Errorf("Invalid block obj")
	}
	if best.Hash != expected.Blocks[0].Hash {
		t.Errorf("Invalid block obj")
	}
	if best.Parent != expected.Blocks[1].Hash {
		t.Errorf("Invalid block obj")
	}

	ticker = time.NewTicker(time.Second * 2)
	var trans Transaction
	select {
	case tx := <-c.txNotifyChan:
		trans = tx
	case <-ticker.C:
		t.Error("Tx notify listener timed out")
		return
	}
	validateTransaction(trans, TestTx, t)
}

func TestInsightClient_ListenAddress(t *testing.T) {
	setup()
	defer teardown()

	var (
		c          = NewTestClient()
		mockSocket = &MockSocketClient{make(map[string]func(h *gosocketio.Channel, args interface{})), []string{}}
	)

	addr, err := btcutil.DecodeAddress("17rxURoF96VhmkcEGCj5LNQkmN9HVhWb7F", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}

	c.socketClient = mockSocket
	c.ListenAddress(addr)

	if mockSocket.listeningAddresses[0] != addr.String() {
		t.Error("Failed to listen on address")
	}
}
