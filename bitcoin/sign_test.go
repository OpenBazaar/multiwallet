package bitcoin

import (
	"bytes"
	"encoding/hex"
	"github.com/OpenBazaar/multiwallet/client"
	"github.com/OpenBazaar/multiwallet/datastore"
	"github.com/OpenBazaar/multiwallet/keys"
	"github.com/OpenBazaar/multiwallet/service"
	"github.com/OpenBazaar/spvwallet"
	"github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/hdkeychain"
	"testing"
	"time"
)

type FeeResponse struct {
	Priority int `json:"priority"`
	Normal   int `json:"normal"`
	Economic int `json:"economic"`
}

func newMockWallet() (*BitcoinWallet, error) {
	mockDb := datastore.NewMockMultiwalletDatastore()

	db, err := mockDb.GetDatastoreForWallet(wallet.Bitcoin)
	if err != nil {
		return nil, err
	}
	cli := client.NewMockApiClient()
	params := &chaincfg.MainNetParams

	seed, err := hex.DecodeString("16c034c59522326867593487c03a8f9615fb248406dd0d4ffb3a6b976a248403")
	if err != nil {
		return nil, err
	}
	master, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		return nil, err
	}
	km, err := keys.NewKeyManager(db.Keys(), params, master, wallet.Bitcoin)
	if err != nil {
		return nil, err
	}

	ws := service.NewWalletService(db, km, cli, params, wallet.Bitcoin)

	fp := spvwallet.NewFeeProvider(2000, 300, 200, 100, "", nil)

	bw := &BitcoinWallet{
		params: params,
		km:     km,
		client: cli,
		ws:     ws,
		db:     db,
		fp:     fp,
	}
	return bw, nil
}

func TestBitcoinWallet_buildTx(t *testing.T) {
	w, err := newMockWallet()
	w.ws.Start()
	time.Sleep(time.Second / 2)
	if err != nil {
		t.Error(err)
	}
	addr, err := w.DecodeAddress("1AhsMpyyyVyPZ9KDUgwsX3zTDJWWSsRo4f")
	if err != nil {
		t.Error(err)
	}

	// Test build normal tx
	tx, err := w.buildTx(1500000, addr, wallet.NORMAL, nil)
	if err != nil {
		t.Error(err)
	}
	if !containsOutput(tx, addr) {
		t.Error("Built tx does not contain the requested output")
	}
	if !validInputs(tx, w.db) {
		t.Error("Built tx does not contain valid inputs")
	}
	if !validChangeAddress(tx, w.db, w.params) {
		t.Error("Built tx does not contain a valid change output")
	}

	// Insuffient funds
	_, err = w.buildTx(1000000000, addr, wallet.NORMAL, nil)
	if err != wallet.ErrorInsuffientFunds {
		t.Error("Failed to throw insuffient funds error")
	}

	// Dust
	_, err = w.buildTx(1, addr, wallet.NORMAL, nil)
	if err != wallet.ErrorDustAmount {
		t.Error("Failed to throw dust error")
	}
}

func containsOutput(tx *wire.MsgTx, addr btcutil.Address) bool {
	for _, o := range tx.TxOut {
		script, _ := txscript.PayToAddrScript(addr)
		if bytes.Equal(script, o.PkScript) {
			return true
		}
	}
	return false
}

func validInputs(tx *wire.MsgTx, db wallet.Datastore) bool {
	utxos, _ := db.Utxos().GetAll()
	uMap := make(map[wire.OutPoint]bool)
	for _, u := range utxos {
		uMap[u.Op] = true
	}
	for _, in := range tx.TxIn {
		if !uMap[in.PreviousOutPoint] {
			return false
		}
	}
	return true
}

func validChangeAddress(tx *wire.MsgTx, db wallet.Datastore, params *chaincfg.Params) bool {
	for _, out := range tx.TxOut {
		_, addrs, _, err := txscript.ExtractPkScriptAddrs(out.PkScript, params)
		if err != nil {
			continue
		}
		if len(addrs) == 0 {
			continue
		}
		_, err = db.Keys().GetPathForKey(addrs[0].ScriptAddress())
		if err == nil {
			return true
		}
	}
	return false
}
