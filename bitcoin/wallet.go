package bitcoin

import (
	"github.com/OpenBazaar/multiwallet/client"
	"github.com/OpenBazaar/multiwallet/keys"
	"github.com/OpenBazaar/multiwallet/service"
	wi "github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	hd "github.com/btcsuite/btcutil/hdkeychain"
	"github.com/tyler-smith/go-bip39"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"fmt"
	"io"
	"github.com/btcsuite/btcd/txscript"
	"errors"
	"github.com/OpenBazaar/multiwallet/config"
	"golang.org/x/net/proxy"
	"bytes"
	"github.com/btcsuite/btcd/wire"
)

type BitcoinWallet struct {
	db       wi.Datastore
	km       *keys.KeyManager
	params   *chaincfg.Params
	client   client.APIClient
	wm       *service.WalletManager
	fp       *FeeProvider
	mnemonic string
}

func NewBitcoinWallet(cfg config.CoinConfig, mnemonic string, params *chaincfg.Params, proxy proxy.Dialer) (*BitcoinWallet, error) {
	seed := bip39.NewSeed(mnemonic, "")

	mPrivKey, err := hd.NewMaster(seed, params)
	if err != nil {
		return nil, err
	}
	km, err := keys.NewKeyManager(cfg.DB.Keys(), params, mPrivKey, wi.Bitcoin)
	if err != nil {
		return nil, err
	}

	c, err := client.NewInsightClient(cfg.ClientAPI.String(), proxy)
	if err != nil {
		return nil, err
	}

	wm := service.NewWalletManager(cfg.DB, km, c, params, wi.Bitcoin)

	fp := NewFeeProvider(cfg.MaxFee, cfg.HighFee, cfg.MediumFee, cfg.LowFee, cfg.FeeAPI.String(), proxy)

	return &BitcoinWallet{cfg.DB, km, params, c, wm, fp, mnemonic}, nil
}

func (w *BitcoinWallet) Start() {
	w.wm.Start()
}

func (w *BitcoinWallet) Mnemonic() string {
	return w.mnemonic
}

func (w *BitcoinWallet) ChainTip()  (uint32, chainhash.Hash) {
	return w.wm.ChainTip()
}

func (w *BitcoinWallet) CurrentAddress(purpose wi.KeyPurpose) btcutil.Address {
	key, _ := w.km.GetCurrentKey(purpose)
	addr, _ := key.Address(w.params)
	return btcutil.Address(addr)
}

func (w *BitcoinWallet) NewAddress(purpose wi.KeyPurpose) btcutil.Address {
	i, _ := w.db.Keys().GetUnused(purpose)
	key, _ := w.km.GenerateChildKey(purpose, uint32(i[1]))
	addr, _ := key.Address(w.params)
	w.db.Keys().MarkKeyAsUsed(addr.ScriptAddress())
	return btcutil.Address(addr)
}

func (w *BitcoinWallet) DumpTables(wr io.Writer) {
	fmt.Fprintln(wr, "Transactions-----")
	txns, _ := w.db.Txns().GetAll(true)
	for _, tx := range txns {
		fmt.Fprintf(wr,"Hash: %s, Height: %d, Value: %d, WatchOnly: %t\n", tx.Txid, int(tx.Height), int(tx.Value), tx.WatchOnly)
	}
	fmt.Fprintln(wr,"\nUtxos-----")
	utxos, _ := w.db.Utxos().GetAll()
	for _, u := range utxos {
		fmt.Fprintf(wr,"Hash: %s, Index: %d, Height: %d, Value: %d, WatchOnly: %t\n", u.Op.Hash.String(), int(u.Op.Index), int(u.AtHeight), int(u.Value), u.WatchOnly)
	}
}

func (w *BitcoinWallet) DecodeAddress(addr string) (btcutil.Address, error) {
	return btcutil.DecodeAddress(addr, w.params)
}

func (w *BitcoinWallet) ScriptToAddress(script []byte) (btcutil.Address, error) {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(script, w.params)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("unknown script")
	}
	return addrs[0], nil
}

func (w *BitcoinWallet) AddressToScript(addr btcutil.Address) ([]byte, error) {
	return txscript.PayToAddrScript(addr)
}

func (w *BitcoinWallet) HasKey(addr btcutil.Address) bool {
	_, err := w.km.GetKeyForScript(addr.ScriptAddress())
	if err != nil {
		return false
	}
	return true
}

func (w *BitcoinWallet) Spend(amount int64, addr btcutil.Address, feeLevel wi.FeeLevel) (*chainhash.Hash, error) {
	tx, err := w.buildTx(amount, addr, feeLevel, nil)
	if err != nil {
		return nil, err
	}
	// Broadcast
	var buf bytes.Buffer
	tx.BtcEncode(&buf, wire.ProtocolVersion, wire.WitnessEncoding)

	_, err = w.client.Broadcast(buf.Bytes())
	if err != nil {
		return nil, err
	}

	ch := tx.TxHash()
	return &ch, nil
}

func (w *BitcoinWallet) GetFeePerByte(feeLevel wi.FeeLevel) uint64 {
	return w.fp.GetFeePerByte(feeLevel)
}

func checkIfStxoIsConfirmed(txid string, txmap map[string]wi.Txn) bool {
	// First look up tx and derserialize
	txn, ok := txmap[txid]
	if !ok {
		return false
	}
	tx := wire.NewMsgTx(1)
	rbuf := bytes.NewReader(txn.Bytes)
	err := tx.BtcDecode(rbuf, wire.ProtocolVersion, wire.WitnessEncoding)
	if err != nil {
		return false
	}

	// For each input, recursively check if confirmed
	inputsConfirmed := true
	for _, in := range tx.TxIn {
		checkTx, ok := txmap[in.PreviousOutPoint.Hash.String()]
		if ok { // Is an stxo. If confirmed we can return true. If no, we need to check the dependency.
			if checkTx.Height == 0 {
				if !checkIfStxoIsConfirmed(in.PreviousOutPoint.Hash.String(), txmap) {
					inputsConfirmed = false
				}
			}
		} else { // We don't have the tx in our db so it can't be an stxo. Return false.
			return false
		}
	}
	return inputsConfirmed
}

func (w *BitcoinWallet) Balance() (confirmed, unconfirmed int64) {
	utxos, _ := w.db.Utxos().GetAll()
	txns, _ := w.db.Txns().GetAll(false)
	var txmap = make(map[string]wi.Txn)
	for _, tx := range txns {
		txmap[tx.Txid] = tx
	}

	for _, utxo := range utxos {
		if !utxo.WatchOnly {
			if utxo.AtHeight > 0 {
				confirmed += utxo.Value
			} else {
				if checkIfStxoIsConfirmed(utxo.Op.Hash.String(), txmap) {
					confirmed += utxo.Value
				} else {
					unconfirmed += utxo.Value
				}
			}
		}
	}
	return confirmed, unconfirmed
}