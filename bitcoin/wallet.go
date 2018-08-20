package bitcoin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/OpenBazaar/spvwallet"
	wi "github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	btc "github.com/btcsuite/btcutil"
	hd "github.com/btcsuite/btcutil/hdkeychain"
	"github.com/btcsuite/btcwallet/wallet/txrules"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/net/proxy"

	"github.com/OpenBazaar/multiwallet/client"
	"github.com/OpenBazaar/multiwallet/config"
	"github.com/OpenBazaar/multiwallet/keys"
	"github.com/OpenBazaar/multiwallet/service"
	"github.com/OpenBazaar/multiwallet/util"
)

type BitcoinWallet struct {
	db     wi.Datastore
	km     *keys.KeyManager
	params *chaincfg.Params
	client client.APIClient
	ws     *service.WalletService
	fp     *spvwallet.FeeProvider

	mPrivKey *hd.ExtendedKey
	mPubKey  *hd.ExtendedKey
}

func NewBitcoinWallet(cfg config.CoinConfig, mnemonic string, params *chaincfg.Params, proxy proxy.Dialer) (*BitcoinWallet, error) {
	seed := bip39.NewSeed(mnemonic, "")

	mPrivKey, err := hd.NewMaster(seed, params)
	if err != nil {
		return nil, err
	}
	mPubKey, err := mPrivKey.Neuter()
	if err != nil {
		return nil, err
	}
	km, err := keys.NewKeyManager(cfg.DB.Keys(), params, mPrivKey, wi.Bitcoin, keyToAddress)
	if err != nil {
		return nil, err
	}

	c, err := client.NewInsightClient(cfg.ClientAPI.String(), proxy)
	if err != nil {
		return nil, err
	}

	wm := service.NewWalletService(cfg.DB, km, c, params, wi.Bitcoin)

	fp := spvwallet.NewFeeProvider(cfg.MaxFee, cfg.HighFee, cfg.MediumFee, cfg.LowFee, cfg.FeeAPI.String(), proxy)

	return &BitcoinWallet{cfg.DB, km, params, c, wm, fp, mPrivKey, mPubKey}, nil
}

func keyToAddress(key *hd.ExtendedKey, params *chaincfg.Params) (btc.Address, error) {
	return key.Address(params)
}

func (w *BitcoinWallet) Start() {
	w.ws.Start()
}

func (w *BitcoinWallet) Params() *chaincfg.Params {
	return w.params
}

func (w *BitcoinWallet) CurrencyCode() string {
	if w.params.Name == chaincfg.MainNetParams.Name {
		return "btc"
	} else {
		return "tbtc"
	}
}

func (w *BitcoinWallet) IsDust(amount int64) bool {
	return txrules.IsDustAmount(btc.Amount(amount), 25, txrules.DefaultRelayFeePerKb)
}

func (w *BitcoinWallet) MasterPrivateKey() *hd.ExtendedKey {
	return w.mPrivKey
}

func (w *BitcoinWallet) MasterPublicKey() *hd.ExtendedKey {
	return w.mPubKey
}

func (w *BitcoinWallet) ChildKey(keyBytes []byte, chaincode []byte, isPrivateKey bool) (*hd.ExtendedKey, error) {
	parentFP := []byte{0x00, 0x00, 0x00, 0x00}
	var id []byte
	if isPrivateKey {
		id = w.params.HDPrivateKeyID[:]
	} else {
		id = w.params.HDPublicKeyID[:]
	}
	hdKey := hd.NewExtendedKey(
		id,
		keyBytes,
		chaincode,
		parentFP,
		0,
		0,
		isPrivateKey)
	return hdKey.Child(0)
}

func (w *BitcoinWallet) CurrentAddress(purpose wi.KeyPurpose) btc.Address {
	key, _ := w.km.GetCurrentKey(purpose)
	addr, _ := key.Address(w.params)
	return btc.Address(addr)
}

func (w *BitcoinWallet) NewAddress(purpose wi.KeyPurpose) btc.Address {
	i, _ := w.db.Keys().GetUnused(purpose)
	key, _ := w.km.GenerateChildKey(purpose, uint32(i[1]))
	addr, _ := key.Address(w.params)
	w.db.Keys().MarkKeyAsUsed(addr.ScriptAddress())
	return btc.Address(addr)
}

func (w *BitcoinWallet) DecodeAddress(addr string) (btc.Address, error) {
	return btc.DecodeAddress(addr, w.params)
}

func (w *BitcoinWallet) ScriptToAddress(script []byte) (btc.Address, error) {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(script, w.params)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("unknown script")
	}
	return addrs[0], nil
}

func (w *BitcoinWallet) AddressToScript(addr btc.Address) ([]byte, error) {
	return txscript.PayToAddrScript(addr)
}

func (w *BitcoinWallet) HasKey(addr btc.Address) bool {
	_, err := w.km.GetKeyForScript(addr.ScriptAddress())
	if err != nil {
		return false
	}
	return true
}

func (w *BitcoinWallet) Balance() (confirmed, unconfirmed int64) {
	utxos, _ := w.db.Utxos().GetAll()
	txns, _ := w.db.Txns().GetAll(false)
	return util.CalcBalance(utxos, txns)
}

func (w *BitcoinWallet) Transactions() ([]wi.Txn, error) {
	return w.db.Txns().GetAll(false)
}

func (w *BitcoinWallet) GetTransaction(txid chainhash.Hash) (wi.Txn, error) {
	txn, err := w.db.Txns().Get(txid)
	return txn, err
}

func (w *BitcoinWallet) ChainTip() (uint32, chainhash.Hash) {
	return w.ws.ChainTip()
}

func (w *BitcoinWallet) GetFeePerByte(feeLevel wi.FeeLevel) uint64 {
	return w.fp.GetFeePerByte(feeLevel)
}

func (w *BitcoinWallet) Spend(amount int64, addr btc.Address, feeLevel wi.FeeLevel) (*chainhash.Hash, error) {
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

func (w *BitcoinWallet) BumpFee(txid chainhash.Hash) (*chainhash.Hash, error) {
	return w.bumpFee(txid)
}

func (w *BitcoinWallet) EstimateFee(ins []wi.TransactionInput, outs []wi.TransactionOutput, feePerByte uint64) uint64 {
	tx := new(wire.MsgTx)
	for _, out := range outs {
		output := wire.NewTxOut(out.Value, out.Address.ScriptAddress())
		tx.TxOut = append(tx.TxOut, output)
	}
	estimatedSize := EstimateSerializeSize(len(ins), tx.TxOut, false, P2PKH)
	fee := estimatedSize * int(feePerByte)
	return uint64(fee)
}

func (w *BitcoinWallet) EstimateSpendFee(amount int64, feeLevel wi.FeeLevel) (uint64, error) {
	return w.estimateSpendFee(amount, feeLevel)
}

/*
func (w *BitcoinWallet) sweepAddress(ins []wi.TransactionInput, address *btc.Address, key *hd.ExtendedKey, redeemScript *[]byte, feeLevel wi.FeeLevel) (*chainhash.Hash, error) {
	var internalAddr btc.Address
	if address != nil {
		internalAddr = *address
	} else {
		internalAddr = w.CurrentAddress(wi.INTERNAL)
	}
	script, err := txscript.PayToAddrScript(internalAddr)
	if err != nil {
		return nil, err
	}

	var val int64
	var inputs []*wire.TxIn
	additionalPrevScripts := make(map[wire.OutPoint][]byte)
	for _, in := range ins {
		val += in.Value
		ch, err := chainhash.NewHashFromStr(hex.EncodeToString(in.OutpointHash))
		if err != nil {
			return nil, err
		}
		script, err := txscript.PayToAddrScript(in.LinkedAddress)
		if err != nil {
			return nil, err
		}
		outpoint := wire.NewOutPoint(ch, in.OutpointIndex)
		input := wire.NewTxIn(outpoint, []byte{}, [][]byte{})
		inputs = append(inputs, input)
		additionalPrevScripts[*outpoint] = script
	}
	out := wire.NewTxOut(val, script)

	txType := spvwallet.P2PKH
	if redeemScript != nil {
		txType = spvwallet.P2SH_1of2_Multisig
		_, err := spvwallet.LockTimeFromRedeemScript(*redeemScript)
		if err == nil {
			txType = spvwallet.P2SH_Multisig_Timelock_1Sig
		}
	}
	estimatedSize := spvwallet.EstimateSerializeSize(len(ins), []*wire.TxOut{out}, false, txType)

	// Calculate the fee
	feePerByte := int(w.GetFeePerByte(feeLevel))
	fee := estimatedSize * feePerByte

	outVal := val - int64(fee)
	if outVal < 0 {
		outVal = 0
	}
	out.Value = outVal

	tx := &wire.MsgTx{
		Version:  wire.TxVersion,
		TxIn:     inputs,
		TxOut:    []*wire.TxOut{out},
		LockTime: 0,
	}

	// BIP 69 sorting
	txsort.InPlaceSort(tx)

	// Sign tx
	privKey, err := key.ECPrivKey()
	if err != nil {
		return nil, err
	}
	pk := privKey.PubKey().SerializeCompressed()
	addressPub, err := btc.NewAddressPubKey(pk, w.params)

	getKey := txscript.KeyClosure(func(addr btc.Address) (*btcec.PrivateKey, bool, error) {
		if addressPub.EncodeAddress() == addr.EncodeAddress() {
			wif, err := btc.NewWIF(privKey, w.params, true)
			if err != nil {
				return nil, false, err
			}
			return wif.PrivKey, wif.CompressPubKey, nil
		}
		return nil, false, errors.New("Not found")
	})
	getScript := txscript.ScriptClosure(func(addr btc.Address) ([]byte, error) {
		if redeemScript == nil {
			return []byte{}, nil
		}
		return *redeemScript, nil
	})

	// Check if time locked
	var timeLocked bool
	if redeemScript != nil {
		rs := *redeemScript
		if rs[0] == txscript.OP_IF {
			timeLocked = true
			tx.Version = 2
		}
		for _, txIn := range tx.TxIn {
			locktime, err := spvwallet.LockTimeFromRedeemScript(*redeemScript)
			if err != nil {
				return nil, err
			}
			txIn.Sequence = locktime
		}
	}

	hashes := txscript.NewTxSigHashes(tx)
	for i, txIn := range tx.TxIn {
		if redeemScript == nil {
			prevOutScript := additionalPrevScripts[txIn.PreviousOutPoint]
			script, err := txscript.SignTxOutput(w.params,
				tx, i, prevOutScript, txscript.SigHashAll, getKey,
				getScript, txIn.SignatureScript)
			if err != nil {
				return nil, errors.New("Failed to sign transaction")
			}
			txIn.SignatureScript = script
		} else {
			sig, err := txscript.RawTxInWitnessSignature(tx, hashes, i, ins[i].Value, *redeemScript, txscript.SigHashAll, privKey)
			if err != nil {
				return nil, err
			}
			var witness wire.TxWitness
			if timeLocked {
				witness = wire.TxWitness{sig, []byte{}}
			} else {
				witness = wire.TxWitness{[]byte{}, sig}
			}
			witness = append(witness, *redeemScript)
			txIn.Witness = witness
		}
	}

	// broadcast
	// Broadcast
	var buf bytes.Buffer
	tx.BtcEncode(&buf, wire.ProtocolVersion, wire.WitnessEncoding)

	_, err = w.client.Broadcast(buf.Bytes())
	if err != nil {
		return nil, err
	}

	txid := tx.TxHash()
	return &txid, nil
}
*/

func (w *BitcoinWallet) SweepAddress(ins []wi.TransactionInput, address *btc.Address, key *hd.ExtendedKey, redeemScript *[]byte, feeLevel wi.FeeLevel) (*chainhash.Hash, error) {
	return w.sweepAddress(ins, address, key, redeemScript, feeLevel)
}

func (w *BitcoinWallet) CreateMultisigSignature(ins []wi.TransactionInput, outs []wi.TransactionOutput, key *hd.ExtendedKey, redeemScript []byte, feePerByte uint64) ([]wi.Signature, error) {
	return w.createMultisigSignature(ins, outs, key, redeemScript, feePerByte)
}

func (w *BitcoinWallet) Multisign(ins []wi.TransactionInput, outs []wi.TransactionOutput, sigs1 []wi.Signature, sigs2 []wi.Signature, redeemScript []byte, feePerByte uint64, broadcast bool) ([]byte, error) {
	return w.multisign(ins, outs, sigs1, sigs2, redeemScript, feePerByte, broadcast)
}

func (w *BitcoinWallet) GenerateMultisigScript(keys []hd.ExtendedKey, threshold int, timeout time.Duration, timeoutKey *hd.ExtendedKey) (addr btc.Address, redeemScript []byte, err error) {
	return w.generateMultisigScript(keys, threshold, timeout, timeoutKey)
}

func (w *BitcoinWallet) AddWatchedAddress(addr btc.Address) error {
	w.client.ListenAddress(addr)
	return nil
}

func (w *BitcoinWallet) AddWatchedScript(script []byte) error {
	err := w.db.WatchedScripts().Put(script)
	if err != nil {
		return err
	}
	addr, err := w.ScriptToAddress(script)
	if err != nil {
		return err
	}
	w.client.ListenAddress(addr)
	return nil
}

func (w *BitcoinWallet) AddTransactionListener(callback func(wi.TransactionCallback)) {
	w.ws.AddTransactionListener(callback)
}

func (w *BitcoinWallet) ReSyncBlockchain(fromTime time.Time) {
	go w.ws.UpdateState()
}

func (w *BitcoinWallet) GetConfirmations(txid chainhash.Hash) (uint32, uint32, error) {
	txn, err := w.db.Txns().Get(txid)
	if err != nil {
		return 0, 0, err
	}
	if txn.Height == 0 {
		return 0, 0, nil
	}
	chainTip, _ := w.ChainTip()
	return chainTip - uint32(txn.Height) + 1, uint32(txn.Height), nil
}

func (w *BitcoinWallet) Close() {
	w.ws.Stop()
	w.client.Close()
}

func (w *BitcoinWallet) DumpTables(wr io.Writer) {
	fmt.Fprintln(wr, "Transactions-----")
	txns, _ := w.db.Txns().GetAll(true)
	for _, tx := range txns {
		fmt.Fprintf(wr, "Hash: %s, Height: %d, Value: %d, WatchOnly: %t\n", tx.Txid, int(tx.Height), int(tx.Value), tx.WatchOnly)
	}
	fmt.Fprintln(wr, "\nUtxos-----")
	utxos, _ := w.db.Utxos().GetAll()
	for _, u := range utxos {
		fmt.Fprintf(wr, "Hash: %s, Index: %d, Height: %d, Value: %d, WatchOnly: %t\n", u.Op.Hash.String(), int(u.Op.Index), int(u.AtHeight), int(u.Value), u.WatchOnly)
	}
}
