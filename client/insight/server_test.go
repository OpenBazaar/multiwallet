package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/OpenBazaar/multiwallet/client"
	wi "github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"net/http"
	"testing"
)

type DummyWriter struct {
	data string
	code int
}

func (d *DummyWriter) Write(p []byte) (n int, err error) {
	d.data = string(p)
	return 0, err
}
func (d *DummyWriter) WriteHeader(statusCode int) {
	d.code = statusCode
}
func (d *DummyWriter) Header() http.Header {
	return http.Header{}
}

type RawTx struct {
	Raw string `json:"rawtx"`
}

func buildRawTransaction(in *wire.OutPoint, outAddr string, outValue int64) ([]byte, string, error) {
	tx := wire.NewMsgTx(1)
	txIn := wire.NewTxIn(in, nil, nil)
	tx.TxIn = append(tx.TxIn, txIn)

	addr, err := btcutil.DecodeAddress(outAddr, &chaincfg.RegressionNetParams)
	if err != nil {
		return nil, "", err
	}
	script, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return nil, "", err
	}
	out := wire.NewTxOut(outValue, script)
	tx.TxOut = append(tx.TxOut, out)
	var b2 bytes.Buffer
	err = tx.BtcEncode(&b2, wire.ProtocolVersion, wire.WitnessEncoding)
	if err != nil {
		return nil, "", err
	}

	raw := RawTx{hex.EncodeToString(b2.Bytes())}
	rawJson, err := json.MarshalIndent(&raw, "", "    ")
	if err != nil {
		return nil, "", err
	}
	return rawJson, tx.TxHash().String(), nil
}

func TestGenerate(t *testing.T) {
	r, err := http.NewRequest(http.MethodPost, "http://localhost:8080/generate?nBlocks=1", nil)
	if err != nil {
		t.Fatal(err)
	}

	s := NewMockInsightServer(wi.Bitcoin)
	if s.lastBlock.Height != 1 {
		t.Error("Failed to initialized server with one block")
	}

	s.handleGenerate(&DummyWriter{}, r)
	if s.lastBlock.Height != 2 {
		t.Error("Failed to generate new block")
	}

	r, err = http.NewRequest(http.MethodPost, "http://localhost:8080/blocks", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := DummyWriter{}
	s.handleGetBestBlock(&w, r)
	bl := client.BlockList{}
	err = json.Unmarshal([]byte(w.data), &bl)
	if err != nil {
		t.Fatal(err)
	}
	if bl.Blocks[0].Hash != s.lastBlock.Hash || bl.Blocks[0].Height != s.lastBlock.Height {
		t.Fatal("returned incorrect block")
	}
}

func TestGenerateToAddress(t *testing.T) {
	addr := "n3sQnroRD5LVSd5UCQdUKJRkAiU8iK4un5"
	r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:8080/generatetoaddress?addr=%s&amount=1", addr), nil)
	if err != nil {
		t.Fatal(err)
	}

	s := NewMockInsightServer(wi.Bitcoin)
	s.handleGenerateToAddress(&DummyWriter{}, r)
	utxos, ok := s.utxoIndex[addr]
	if !ok {
		t.Fatal("failed to save new utxo")
	}
	if len(utxos) != 1 {
		t.Fatal("failed to save correct number of utxos")
	}
	if utxos[0].Amount != float64(1) {
		t.Fatal("saved incorrect value for utxo")
	}

	txs, ok := s.addrIndex[addr]
	if !ok {
		t.Fatal("failed to save new transaction")
	}
	if len(txs) != 1 {
		t.Fatal("failed to save correct number of txs")
	}
	if txs[0].Outputs[0].ScriptPubKey.Addresses[0] != addr {
		t.Fatal("saved incorrect address in transaction")
	}
	if txs[0].Outputs[0].Value != float64(1) {
		t.Fatal("saved incorrect amount in transaction")
	}
	if txs[0].Confirmations != 1 {
		t.Fatal("failed to save transaction as confirmed")
	}
}

func TestProcessTransaction(t *testing.T) {
	addr := "n3sQnroRD5LVSd5UCQdUKJRkAiU8iK4un5"
	r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:8080/generatetoaddress?addr=%s&amount=1", addr), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewMockInsightServer(wi.Bitcoin)
	s.handleGenerateToAddress(&DummyWriter{}, r)

	// Create an invalid tx spending unknown inputs
	txinHash := make([]byte, 32)
	rand.Read(txinHash)
	txinCh, err := chainhash.NewHash(txinHash)
	if err != nil {
		t.Fatal(err)
	}
	inOp := wire.NewOutPoint(txinCh, 0)
	rawJson, txHash, err := buildRawTransaction(inOp, "2N7Vh8gCgpPryXCB4PHFuaUJ29zuYQjt3DQ", 100000)
	if err != nil {
		t.Fatal(err)
	}

	// Try to send an invalid transaction with an unknown input
	r, err = http.NewRequest(http.MethodPost, "http://localhost:8080/broadcast", bytes.NewReader(rawJson))
	if err != nil {
		t.Fatal(err)
	}
	w := DummyWriter{}
	s.handleBroadcast(&w, r)

	if w.code == http.StatusOK {
		t.Fatal("invalid transaction was accepted with status OK")
	}
	if _, ok := s.txIndex[txHash]; ok {
		t.Fatal("invalid transaction was added to tx index")
	}

	// Now build a good transaction make sure it's accepted
	utxos, ok := s.utxoIndex[addr]
	if !ok {
		t.Fatal("utxos not found for address")
	}
	prevHash, err := chainhash.NewHashFromStr(utxos[0].Txid)
	if err != nil {
		t.Fatal(err)
	}
	inOp = wire.NewOutPoint(prevHash, uint32(utxos[0].Vout))
	rawJson, txHash, err = buildRawTransaction(inOp, "2N7Vh8gCgpPryXCB4PHFuaUJ29zuYQjt3DQ", 100000)
	if err != nil {
		t.Fatal(err)
	}

	r, err = http.NewRequest(http.MethodPost, "http://localhost:8080/broadcast", bytes.NewReader(rawJson))
	if err != nil {
		t.Fatal(err)
	}
	w = DummyWriter{}
	s.handleBroadcast(&w, r)

	savedTx, ok := s.txIndex[txHash]
	if !ok {
		t.Fatal("valid transaction was not added to tx index")
	}
	if savedTx.Confirmations != 0 {
		t.Fatal("saved tx has incorrect number of confirmations")
	}

	if _, ok := s.utxoIndex["2N7Vh8gCgpPryXCB4PHFuaUJ29zuYQjt3DQ"]; !ok {
		t.Fatal("utxo not set in utxo index")
	}
	if _, ok := s.utxoSet[txHash+":0"]; !ok {
		t.Fatal("utxo not set in utxo set")
	}

	// Generate a block and make sure the tx confirms
	r, err = http.NewRequest(http.MethodPost, "http://localhost:8080/generate?nBlocks=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	s.handleGenerate(&DummyWriter{}, r)
	savedTx, ok = s.txIndex[txHash]
	if !ok {
		t.Fatal("valid transaction was not added to tx index")
	}
	if savedTx.Confirmations != 1 {
		t.Fatal("saved tx has incorrect number of confirmations")
	}
}

func TestFetchTxsandUtxos(t *testing.T) {
	addr := "n3sQnroRD5LVSd5UCQdUKJRkAiU8iK4un5"
	r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:8080/generatetoaddress?addr=%s&amount=1", addr), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := NewMockInsightServer(wi.Bitcoin)
	s.handleGenerateToAddress(&DummyWriter{}, r)

	utxos, ok := s.utxoIndex[addr]
	if !ok {
		t.Fatal("utxos not found for address")
	}
	prevHash, err := chainhash.NewHashFromStr(utxos[0].Txid)
	if err != nil {
		t.Fatal(err)
	}
	inOp := wire.NewOutPoint(prevHash, uint32(utxos[0].Vout))
	rawJson, txhash, err := buildRawTransaction(inOp, "2N7Vh8gCgpPryXCB4PHFuaUJ29zuYQjt3DQ", 100000)
	if err != nil {
		t.Fatal(err)
	}

	r, err = http.NewRequest(http.MethodPost, "http://localhost:8080/broadcast", bytes.NewReader(rawJson))
	if err != nil {
		t.Fatal(err)
	}
	w := DummyWriter{}
	s.handleBroadcast(&w, r)

	type request struct {
		Addrs string `json:"addrs"`
		From  int    `json:"from"`
		To    int    `json:"to"`
	}
	req := request{
		Addrs: "2N7Vh8gCgpPryXCB4PHFuaUJ29zuYQjt3DQ",
		From:  0,
		To:    9999999,
	}
	rawJson, err = json.MarshalIndent(&req, "", "    ")
	if err != nil {
		t.Fatal(err)
	}

	r, err = http.NewRequest(http.MethodPost, "http://localhost:8080/gettransactions", bytes.NewReader(rawJson))
	if err != nil {
		t.Fatal(err)
	}
	w = DummyWriter{}
	s.handleGetTransactions(&w, r)

	if w.code == http.StatusInternalServerError || w.code == http.StatusBadRequest {
		t.Fatal("server returned error")
	}

	txList := client.TransactionList{}
	err = json.Unmarshal([]byte(w.data), &txList)
	if err != nil {
		t.Fatal(err)
	}
	if len(txList.Items) != 1 {
		t.Fatal("failed to return transactions")
	}

	if txList.Items[0].Txid != txhash {
		t.Fatal("returned incorrect transaction")
	}

	// Send GetUtxos query
	r, err = http.NewRequest(http.MethodPost, "http://localhost:8080/getutxos", bytes.NewReader(rawJson))
	if err != nil {
		t.Fatal(err)
	}
	w = DummyWriter{}
	s.handleGetUtxos(&w, r)

	if w.code == http.StatusInternalServerError || w.code == http.StatusBadRequest {
		t.Fatal("server returned error")
	}

	var utxoList []client.Utxo
	err = json.Unmarshal([]byte(w.data), &utxoList)
	if err != nil {
		t.Fatal(err)
	}
	if len(utxoList) != 1 {
		t.Fatal("failed to return transactions")
	}

	if utxoList[0].Txid != txhash && utxoList[0].Vout != 0 {
		t.Fatal("returned incorrect transaction")
	}

}
