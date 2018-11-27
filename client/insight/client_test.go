package insight_test

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/OpenBazaar/multiwallet/client/insight"
	"github.com/OpenBazaar/multiwallet/model"
	"github.com/OpenBazaar/multiwallet/model/mock"
	"github.com/OpenBazaar/multiwallet/test"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
)

func MustNewInsightClient(target string) *insight.InsightClient {
	ic, err := insight.NewInsightClient(target, nil)
	if err != nil {
		panic(err)
	}
	return ic
}

var TestTx = model.Transaction{
	Txid:     "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428",
	Version:  2,
	Locktime: 512378,
	Inputs: []model.Input{
		{
			Txid:       "6d892f04fc097f430d58ab06229c9b6344a130fc1842da5b990e857daed42194",
			Vout:       1,
			Sequence:   1,
			ValueIface: "0.04294455",
			ScriptSig: model.Script{
				Hex: "4830450221008665481674067564ef562cfd8d1ca8f1506133fb26a2319e4b8dfba3cedfd5de022038f27121c44e6c64b93b94d72620e11b9de35fd864730175db9176ca98f1ec610121022023e49335a0dddb864ff673468a6cc04e282571b1227933fcf3ff9babbcc662",
			},
			Addr:     "1C74Gbij8Q5h61W58aSKGvXK4rk82T2A3y",
			Satoshis: 4294455,
		},
	},
	Outputs: []model.Output{
		{
			ScriptPubKey: model.OutScript{
				Script: model.Script{
					Hex: "76a914ff3f7d402fbd6d116ba4a02af9784f3ae9b7108a88ac",
				},
				Type:      "pay-to-pubkey-hash",
				Addresses: []string{"1QGdNEDjWnghrjfTBCTDAPZZ3ffoKvGc9B"},
			},
			ValueIface: "0.01398175",
		},
		{
			ScriptPubKey: model.OutScript{
				Script: model.Script{
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

func TestInsightClient_GetInfo(t *testing.T) {
	var (
		endpoint     = "http://localhost:8334"
		c            = MustNewInsightClient(endpoint)
		testPath     = fmt.Sprintf("%s/status", endpoint)
		expectedInfo = mock.MockInfo
		httpClient   = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, model.Status{Info: expectedInfo})
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	info, err := c.GetInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !expectedInfo.IsEqual(*info) {
		t.Errorf("returned invalid info: %v", info)
	}
}

func TestInsightClient_GetTransaction(t *testing.T) {
	var (
		endpoint   = "http://localhost:8334"
		c          = MustNewInsightClient(endpoint)
		testPath   = fmt.Sprintf("%s/tx/1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428", endpoint)
		expectedTx = TestTx
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, expectedTx)
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	tx, err := c.GetTransaction("1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428")
	if err != nil {
		t.Fatal(err)
	}
	test.ValidateTransaction(*tx, expectedTx, t)
}

func TestInsightClient_GetRawTransaction(t *testing.T) {
	var (
		endpoint        = "http://localhost:8334"
		c               = MustNewInsightClient(endpoint)
		testPath        = fmt.Sprintf("%s/rawtx/1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428", endpoint)
		expectedTxBytes = []byte("encoded tx data here")
		httpClient      = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, model.RawTxResponse{RawTx: hex.EncodeToString(expectedTxBytes)})
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	txBytes, err := c.GetRawTransaction("1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428")
	if err != nil {
		t.Fatal(err)
	}
	if string(txBytes) != string(expectedTxBytes) {
		t.Errorf("returned unexpected raw tx bytes: %v\n", hex.EncodeToString(txBytes))
	}
}

func TestInsightClient_GetTransactions(t *testing.T) {
	var (
		endpoint = "http://localhost:8334"
		c        = MustNewInsightClient(endpoint)
		testPath = fmt.Sprintf("%s/addrs/txs", endpoint)
		expected = model.TransactionList{
			TotalItems: 1,
			From:       0,
			To:         1,
			Items: []model.Transaction{
				TestTx,
			},
		}
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodPost, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	addr, err := btcutil.DecodeAddress("1C74Gbij8Q5h61W58aSKGvXK4rk82T2A3y", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	txs, err := c.GetTransactions([]btcutil.Address{addr})
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 1 {
		t.Error("Returned incorrect number of transactions")
		return
	}
	test.ValidateTransaction(txs[0], expected.Items[0], t)
}

func TestInsightClient_GetUtxos(t *testing.T) {
	var (
		endpoint = "http://localhost:8334"
		c        = MustNewInsightClient(endpoint)
		testPath = fmt.Sprintf("%s/addrs/utxo", endpoint)
		expected = []model.Utxo{
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
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodPost, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	addr, err := btcutil.DecodeAddress("1QGdNEDjWnghrjfTBCTDAPZZ3ffoKvGc9B", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	utxos, err := c.GetUtxos([]btcutil.Address{addr})
	if err != nil {
		t.Fatal(err)
	}
	if len(utxos) != 1 {
		t.Error("Returned incorrect number of utxos")
	}
	validateUtxo(utxos[0], expected[0], t)
}

func validateUtxo(utxo, expected model.Utxo, t *testing.T) {
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
		endpoint = "http://localhost:8334"
		c        = MustNewInsightClient(endpoint)
		testHash = "0000000000000000003f1fb88ac3dab0e607e87def0e9031f7bea02cb464a04f"
	)

	go func() {
		c.BlockChannel() <- model.Block{Hash: testHash}
	}()

	ticker := time.NewTicker(time.Second)
	select {
	case <-ticker.C:
		t.Fatal("Timed out waiting for block")
	case b := <-c.BlockNotify():
		if b.Hash != testHash {
			t.Error("Returned incorrect block hash")
		}
	}
}

func TestInsightClient_TransactionNotify(t *testing.T) {
	endpoint := "http://localhost:8334"
	c := MustNewInsightClient(endpoint)

	go func() {
		c.TxChannel() <- TestTx
	}()

	ticker := time.NewTicker(time.Second)
	select {
	case <-ticker.C:
		t.Fatal("Timed out waiting for tx")
	case b := <-c.TransactionNotify():
		for n, in := range b.Inputs {
			f, err := model.ToFloat(in.ValueIface)
			if err != nil {
				t.Error(err)
			}
			b.Inputs[n].Value = f
		}
		for n, out := range b.Outputs {
			f, err := model.ToFloat(out.ValueIface)
			if err != nil {
				t.Error(err)
			}
			b.Outputs[n].Value = f
		}
		test.ValidateTransaction(b, TestTx, t)
	}
}

func TestInsightClient_Broadcast(t *testing.T) {

	type Response struct {
		Txid string `json:"txid"`
	}

	var (
		endpoint   = "http://localhost:8334"
		c          = MustNewInsightClient(endpoint)
		testPath   = fmt.Sprintf("%s/tx/send", endpoint)
		expected   = Response{"1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428"}
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodPost, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	id, err := c.Broadcast([]byte{0x00, 0x01, 0x02, 0x03})
	if err != nil {
		t.Fatal(err)
	}
	if id != expected.Txid {
		t.Error("Returned incorrect txid")
	}
}

func TestInsightClient_GetBestBlock(t *testing.T) {
	var (
		endpoint = "http://localhost:8334"
		c        = MustNewInsightClient(endpoint)
		testPath = fmt.Sprintf("%s/blocks", endpoint)
		expected = model.BlockList{
			Blocks: []model.Block{
				{
					Hash:   "00000000000000000108a1f4d4db839702d72f16561b1154600a26c453ecb378",
					Height: 2,
					Time:   12345,
					Size:   200,
					Tx:     make([]string, 5),
				},
				{
					Hash:   "0000000000c96f193d23fde69a2fff56793e99e23cbd51947828a33e287ff659",
					Height: 1,
					Time:   23456,
					Size:   300,
					Tx:     make([]string, 6),
				},
			},
		}
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	best, err := c.GetBestBlock()
	if err != nil {
		t.Fatal(err)
	}
	validateBlock(*best, expected.Blocks[0], expected.Blocks[1].Hash, t)
}

func validateBlock(b, expected model.Block, prevhash string, t *testing.T) {
	if len(b.Tx) != len(expected.Tx) {
		t.Errorf("Invalid block obj")
	}
	if b.Size != expected.Size {
		t.Errorf("Invalid block obj")
	}
	if b.Time != expected.Time {
		t.Errorf("Invalid block obj")
	}
	if b.Height != expected.Height {
		t.Errorf("Invalid block obj")
	}
	if b.Hash != expected.Hash {
		t.Errorf("Invalid block obj")
	}
	if b.PreviousBlockhash != prevhash {
		t.Errorf("Invalid block obj")
	}
}

func TestInsightClient_GetBlocksBefore(t *testing.T) {

	var (
		endpoint = "http://localhost:8334"
		c        = MustNewInsightClient(endpoint)
		testPath = fmt.Sprintf("%s/blocks", endpoint)
		expected = model.BlockList{
			Blocks: []model.Block{
				{
					Hash:              "0000000000c96f193d23fde69a2fff56793e99e23cbd51947828a33e287ff659",
					Height:            1,
					Time:              12345,
					Size:              300,
					Tx:                make([]string, 6),
					PreviousBlockhash: "000000000be13618b0149ade349a6da46c0f434b65033017de5d450a9bc1bd7f",
				},
			},
		}
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	blocks, err := c.GetBlocksBefore(time.Unix(23450, 0), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks.Blocks) != 1 {
		t.Errorf("returned incorrect number of blocks: %v", len(blocks.Blocks))
	}
	validateBlock(blocks.Blocks[0], expected.Blocks[0], expected.Blocks[0].PreviousBlockhash, t)
}

func TestInsightClient_setupListeners(t *testing.T) {
	var (
		endpoint      = "http://localhost:8334"
		c             = MustNewInsightClient(endpoint)
		mockSocket    = mock.NewMockWebsocketClient()
		testBlockPath = fmt.Sprintf("%s/blocks", endpoint)
		expected      = model.BlockList{
			Blocks: []model.Block{
				{
					Hash:   "00000000000000000108a1f4d4db839702d72f16561b1154600a26c453ecb378",
					Height: 2,
					Time:   12345,
					Size:   200,
					Tx:     make([]string, 5),
				},
				{
					Hash:   "0000000000c96f193d23fde69a2fff56793e99e23cbd51947828a33e287ff659",
					Height: 1,
					Time:   23456,
					Size:   300,
					Tx:     make([]string, 6),
				},
			},
		}
		testTxPath = fmt.Sprintf("%s/tx/1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428", endpoint)
		expectedTx = TestTx
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	c.SocketClient = mockSocket

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Fatal(err)
	}
	httpmock.RegisterResponder(http.MethodGet, testBlockPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)
	response2, err := httpmock.NewJsonResponse(http.StatusOK, expectedTx)
	if err != nil {
		t.Fatal(err)
	}
	httpmock.RegisterResponder(http.MethodGet, testTxPath,
		func(req *http.Request) (*http.Response, error) {
			return response2, nil
		},
	)

	go c.Start()
	time.Sleep(time.Second)

	go func() {
		m := make(map[string]interface{})
		m[""] = "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428"
		mockSocket.SendCallback("bitcoind/hashblock", nil, "")
		mockSocket.SendCallback("bitcoind/addresstxid", nil, m)
	}()

	ticker := time.NewTicker(time.Second * 2)
	var best model.Block
	select {
	case b := <-c.BlockChannel():
		best = b
	case <-ticker.C:
		t.Fatal("Block notify listener timed out")
		return
	}
	if len(best.Tx) != len(expected.Blocks[0].Tx) {
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
	if best.PreviousBlockhash != expected.Blocks[1].Hash {
		t.Errorf("Invalid block obj")
	}

	ticker = time.NewTicker(time.Second * 2)
	var trans model.Transaction
	select {
	case tx := <-c.TxChannel():
		trans = tx
	case <-ticker.C:
		t.Fatal("Tx notify listener timed out")
		return
	}
	test.ValidateTransaction(trans, TestTx, t)
}

func TestInsightClient_ListenAddress(t *testing.T) {
	var (
		endpoint   = "http://localhost:8334"
		c          = MustNewInsightClient(endpoint)
		mockSocket = mock.NewMockWebsocketClient()
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	addr, err := btcutil.DecodeAddress("17rxURoF96VhmkcEGCj5LNQkmN9HVhWb7F", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}

	c.SocketClient = mockSocket
	c.ListenAddress(addr)

	if !mockSocket.IsListeningForAddress(addr.String()) {
		t.Fatal("Failed to listen on address")
	}
}

func TestInsightClient_EstimateFee(t *testing.T) {
	var (
		endpoint   = "http://localhost:8334"
		c          = MustNewInsightClient(endpoint)
		testPath   = fmt.Sprintf("%s/utils/estimatefee", endpoint)
		expected   = map[int]float64{2: 1.234}
		httpClient = http.Client{}
	)
	httpmock.ActivateNonDefault(&httpClient)
	defer httpmock.DeactivateAndReset()
	c.HTTPClient = httpClient

	response, err := httpmock.NewJsonResponse(http.StatusOK, expected)
	if err != nil {
		t.Fatal(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	fee, err := c.EstimateFee(2)
	if err != nil {
		t.Fatal(err)
	}
	if fee != int(expected[2]*1e8) {
		t.Errorf("returned unexpected fee: %v", fee)
	}
}

func TestDefaultPort(t *testing.T) {
	urls := []struct {
		url  string
		port int
	}{
		{"https://btc.bloqapi.net/insight-api", 443},
		{"http://test-insight.bitpay.com/api", 80},
		{"http://test-bch-insight.bitpay.com:3333/api", 3333},
	}
	for _, s := range urls {
		u, err := url.Parse(s.url)
		if err != nil {
			t.Fatal(err)
		}
		port := model.DefaultPort(*u)
		if port != s.port {
			t.Error("Returned incorrect port")
		}
	}
}
