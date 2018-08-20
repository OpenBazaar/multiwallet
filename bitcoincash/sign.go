package bitcoincash

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/OpenBazaar/spvwallet"
	wi "github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	btc "github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/coinset"
	hd "github.com/btcsuite/btcutil/hdkeychain"
	"github.com/btcsuite/btcutil/txsort"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/btcsuite/btcwallet/wallet/txrules"
	"github.com/cpacia/bchutil"

	"github.com/OpenBazaar/multiwallet/util"
)

func (w *BitcoinCashWallet) buildTx(amount int64, addr btc.Address, feeLevel wi.FeeLevel, optionalOutput *wire.TxOut) (*wire.MsgTx, error) {
	// Check for dust
	script, _ := bchutil.PayToAddrScript(addr)
	if txrules.IsDustAmount(btc.Amount(amount), len(script), txrules.DefaultRelayFeePerKb) {
		return nil, wi.ErrorDustAmount
	}

	var additionalPrevScripts map[wire.OutPoint][]byte
	var additionalKeysByAddress map[string]*btc.WIF
	var inVals map[wire.OutPoint]int64

	// Create input source
	height, _ := w.ws.ChainTip()
	utxos, err := w.db.Utxos().GetAll()
	if err != nil {
		return nil, err
	}
	coinMap := util.GatherCoins(height, utxos, w.ScriptToAddress, w.km.GetKeyForScript)

	coins := make([]coinset.Coin, 0, len(coinMap))
	for k := range coinMap {
		coins = append(coins, k)
	}
	inputSource := func(target btc.Amount) (total btc.Amount, inputs []*wire.TxIn, inputValues []btcutil.Amount, scripts [][]byte, err error) {
		coinSelector := coinset.MaxValueAgeCoinSelector{MaxInputs: 10000, MinChangeAmount: btc.Amount(0)}
		coins, err := coinSelector.CoinSelect(target, coins)
		if err != nil {
			return total, inputs, inputValues, scripts, wi.ErrorInsuffientFunds
		}
		additionalPrevScripts = make(map[wire.OutPoint][]byte)
		additionalKeysByAddress = make(map[string]*btc.WIF)
		inVals = make(map[wire.OutPoint]int64)
		for _, c := range coins.Coins() {
			total += c.Value()
			outpoint := wire.NewOutPoint(c.Hash(), c.Index())
			in := wire.NewTxIn(outpoint, []byte{}, [][]byte{})
			in.Sequence = 0 // Opt-in RBF so we can bump fees
			inputs = append(inputs, in)
			additionalPrevScripts[*outpoint] = c.PkScript()
			key := coinMap[c]
			addr, err := key.Address(w.params)
			if err != nil {
				continue
			}
			privKey, err := key.ECPrivKey()
			if err != nil {
				continue
			}
			wif, _ := btc.NewWIF(privKey, w.params, true)
			additionalKeysByAddress[addr.EncodeAddress()] = wif
			val := c.Value()
			sat := val.ToUnit(btc.AmountSatoshi)
			inVals[*outpoint] = int64(sat)
		}
		return total, inputs, inputValues, scripts, nil
	}

	// Get the fee per kilobyte
	feePerKB := int64(w.GetFeePerByte(feeLevel)) * 1000

	// outputs
	out := wire.NewTxOut(amount, script)

	// Create change source
	changeSource := func() ([]byte, error) {
		addr := w.CurrentAddress(wi.INTERNAL)
		script, err := bchutil.PayToAddrScript(addr)
		if err != nil {
			return []byte{}, err
		}
		return script, nil
	}

	outputs := []*wire.TxOut{out}
	if optionalOutput != nil {
		outputs = append(outputs, optionalOutput)
	}
	authoredTx, err := newUnsignedTransaction(outputs, btc.Amount(feePerKB), inputSource, changeSource)
	if err != nil {
		return nil, err
	}

	// BIP 69 sorting
	txsort.InPlaceSort(authoredTx.Tx)

	// Sign tx
	getKey := txscript.KeyClosure(func(addr btc.Address) (*btcec.PrivateKey, bool, error) {
		addrStr := addr.EncodeAddress()
		wif := additionalKeysByAddress[addrStr]
		return wif.PrivKey, wif.CompressPubKey, nil
	})
	getScript := txscript.ScriptClosure(func(
		addr btc.Address) ([]byte, error) {
		return []byte{}, nil
	})
	for i, txIn := range authoredTx.Tx.TxIn {
		prevOutScript := additionalPrevScripts[txIn.PreviousOutPoint]
		script, err := bchutil.SignTxOutput(w.params,
			authoredTx.Tx, i, prevOutScript, txscript.SigHashAll, getKey,
			getScript, txIn.SignatureScript, inVals[txIn.PreviousOutPoint])
		if err != nil {
			return nil, errors.New("Failed to sign transaction")
		}
		txIn.SignatureScript = script
	}
	return authoredTx.Tx, nil
}

func newUnsignedTransaction(outputs []*wire.TxOut, feePerKb btc.Amount, fetchInputs txauthor.InputSource, fetchChange txauthor.ChangeSource) (*txauthor.AuthoredTx, error) {

	var targetAmount btc.Amount
	for _, txOut := range outputs {
		targetAmount += btc.Amount(txOut.Value)
	}

	estimatedSize := EstimateSerializeSize(1, outputs, true, P2PKH)
	targetFee := txrules.FeeForSerializeSize(feePerKb, estimatedSize)

	for {
		inputAmount, inputs, _, scripts, err := fetchInputs(targetAmount + targetFee)
		if err != nil {
			return nil, err
		}
		if inputAmount < targetAmount+targetFee {
			return nil, errors.New("insufficient funds available to construct transaction")
		}

		maxSignedSize := EstimateSerializeSize(len(inputs), outputs, true, P2PKH)
		maxRequiredFee := txrules.FeeForSerializeSize(feePerKb, maxSignedSize)
		remainingAmount := inputAmount - targetAmount
		if remainingAmount < maxRequiredFee {
			targetFee = maxRequiredFee
			continue
		}

		unsignedTransaction := &wire.MsgTx{
			Version:  wire.TxVersion,
			TxIn:     inputs,
			TxOut:    outputs,
			LockTime: 0,
		}
		changeIndex := -1
		changeAmount := inputAmount - targetAmount - maxRequiredFee
		if changeAmount != 0 && !txrules.IsDustAmount(changeAmount,
			P2PKHOutputSize, txrules.DefaultRelayFeePerKb) {
			changeScript, err := fetchChange()
			if err != nil {
				return nil, err
			}
			if len(changeScript) > P2PKHPkScriptSize {
				return nil, errors.New("fee estimation requires change " +
					"scripts no larger than P2PKH output scripts")
			}
			change := wire.NewTxOut(int64(changeAmount), changeScript)
			l := len(outputs)
			unsignedTransaction.TxOut = append(outputs[:l:l], change)
			changeIndex = l
		}

		return &txauthor.AuthoredTx{
			Tx:          unsignedTransaction,
			PrevScripts: scripts,
			TotalInput:  inputAmount,
			ChangeIndex: changeIndex,
		}, nil
	}
}

func (w *BitcoinCashWallet) bumpFee(txid chainhash.Hash) (*chainhash.Hash, error) {
	txn, err := w.db.Txns().Get(txid)
	if err != nil {
		return nil, err
	}
	if txn.Height > 0 {
		return nil, spvwallet.BumpFeeAlreadyConfirmedError
	}
	if txn.Height < 0 {
		return nil, spvwallet.BumpFeeTransactionDeadError
	}
	// Check utxos for CPFP
	utxos, _ := w.db.Utxos().GetAll()
	for _, u := range utxos {
		if u.Op.Hash.IsEqual(&txid) && u.AtHeight == 0 {
			addr, err := w.ScriptToAddress(u.ScriptPubkey)
			if err != nil {
				return nil, err
			}
			key, err := w.km.GetKeyForScript(addr.ScriptAddress())
			if err != nil {
				return nil, err
			}
			in := wi.TransactionInput{
				LinkedAddress: addr,
				OutpointIndex: u.Op.Index,
				OutpointHash:  u.Op.Hash.CloneBytes(),
				Value:         int64(u.Value),
			}
			transactionID, err := w.sweepAddress([]wi.TransactionInput{in}, nil, key, nil, wi.FEE_BUMP)
			if err != nil {
				return nil, err
			}
			return transactionID, nil
		}
	}
	return nil, spvwallet.BumpFeeNotFoundError
}

/*
func (w *BitcoinCashWallet) sweepAddress(utxos []wi.Utxo, address *btc.Address, key *hd.ExtendedKey, redeemScript *[]byte, feeLevel wi.FeeLevel) (*chainhash.Hash, error) {
	var internalAddr btc.Address
	if address != nil {
		internalAddr = *address
	} else {
		internalAddr = w.CurrentAddress(wi.INTERNAL)
	}
	script, err := bchutil.PayToAddrScript(internalAddr)
	if err != nil {
		return nil, err
	}

	var val int64
	var inputs []*wire.TxIn
	additionalPrevScripts := make(map[wire.OutPoint][]byte)
	for _, u := range utxos {
		val += u.Value
		in := wire.NewTxIn(&u.Op, []byte{}, [][]byte{})
		inputs = append(inputs, in)
		additionalPrevScripts[u.Op] = u.ScriptPubkey
	}
	out := wire.NewTxOut(val, script)

	txType := P2PKH
	if redeemScript != nil {
		txType = P2SH_1of2_Multisig
		_, err := spvwallet.LockTimeFromRedeemScript(*redeemScript)
		if err == nil {
			txType = P2SH_Multisig_Timelock_1Sig
		}
	}
	estimatedSize := EstimateSerializeSize(len(utxos), []*wire.TxOut{out}, false, txType)

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
	keyAddr, err := w.km.KeyToAddress(key)
	if err != nil {
		return nil, err
	}

	getKey := txscript.KeyClosure(func(addr btc.Address) (*btcec.PrivateKey, bool, error) {
		if bytes.Equal(addr.ScriptAddress(), keyAddr.ScriptAddress()) {
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
			for _, txIn := range tx.TxIn {
				locktime, err := spvwallet.LockTimeFromRedeemScript(*redeemScript)
				if err != nil {
					return nil, err
				}
				txIn.Sequence = locktime
			}
		}
	}

	for i, txIn := range tx.TxIn {
		if !timeLocked {
			prevOutScript := additionalPrevScripts[txIn.PreviousOutPoint]
			script, err := bchutil.SignTxOutput(w.params,
				tx, i, prevOutScript, txscript.SigHashAll, getKey,
				getScript, txIn.SignatureScript, utxos[i].Value)
			if err != nil {
				return nil, errors.New("Failed to sign transaction")
			}
			txIn.SignatureScript = script
		} else {
			priv, err := key.ECPrivKey()
			if err != nil {
				return nil, err
			}
			script, err := bchutil.RawTxInSignature(tx, i, *redeemScript, txscript.SigHashAll, priv, utxos[i].Value)
			if err != nil {
				return nil, err
			}
			builder := txscript.NewScriptBuilder().
				AddData(script).
				AddOp(txscript.OP_0).
				AddData(*redeemScript)
			scriptSig, _ := builder.Script()
			txIn.SignatureScript = scriptSig
		}
	}

	// broadcast
	var buf bytes.Buffer
	tx.BtcEncode(&buf, wire.ProtocolVersion, wire.BaseEncoding)
	_, err = w.client.Broadcast(buf.Bytes())
	if err != nil {
		return nil, err
	}

	txid := tx.TxHash()
	return &txid, nil
}
*/

func (w *BitcoinCashWallet) sweepAddress(ins []wi.TransactionInput, address *btc.Address, key *hd.ExtendedKey, redeemScript *[]byte, feeLevel wi.FeeLevel) (*chainhash.Hash, error) {
	var internalAddr btc.Address
	if address != nil {
		internalAddr = *address
	} else {
		internalAddr = w.CurrentAddress(wi.INTERNAL)
	}
	script, err := bchutil.PayToAddrScript(internalAddr)
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
		script, err := bchutil.PayToAddrScript(in.LinkedAddress)
		if err != nil {
			return nil, err
		}
		outpoint := wire.NewOutPoint(ch, in.OutpointIndex)
		input := wire.NewTxIn(outpoint, []byte{}, [][]byte{})
		inputs = append(inputs, input)
		additionalPrevScripts[*outpoint] = script
	}
	out := wire.NewTxOut(val, script)

	txType := P2PKH
	if redeemScript != nil {
		txType = P2SH_1of2_Multisig
		_, err := spvwallet.LockTimeFromRedeemScript(*redeemScript)
		if err == nil {
			txType = P2SH_Multisig_Timelock_1Sig
		}
	}
	estimatedSize := EstimateSerializeSize(len(ins), []*wire.TxOut{out}, false, txType)

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
			for _, txIn := range tx.TxIn {
				locktime, err := spvwallet.LockTimeFromRedeemScript(*redeemScript)
				if err != nil {
					return nil, err
				}
				txIn.Sequence = locktime
			}
		}
	}

	for i, txIn := range tx.TxIn {
		if !timeLocked {
			prevOutScript := additionalPrevScripts[txIn.PreviousOutPoint]
			script, err := bchutil.SignTxOutput(w.params,
				tx, i, prevOutScript, txscript.SigHashAll, getKey,
				getScript, txIn.SignatureScript, ins[i].Value)
			if err != nil {
				return nil, errors.New("Failed to sign transaction")
			}
			txIn.SignatureScript = script
		} else {
			priv, err := key.ECPrivKey()
			if err != nil {
				return nil, err
			}
			script, err := bchutil.RawTxInSignature(tx, i, *redeemScript, txscript.SigHashAll, priv, ins[i].Value)
			if err != nil {
				return nil, err
			}
			builder := txscript.NewScriptBuilder().
				AddData(script).
				AddOp(txscript.OP_0).
				AddData(*redeemScript)
			scriptSig, _ := builder.Script()
			txIn.SignatureScript = scriptSig
		}
	}

	// broadcast
	var buf bytes.Buffer
	tx.BtcEncode(&buf, wire.ProtocolVersion, wire.WitnessEncoding)
	_, err = w.client.Broadcast(buf.Bytes())
	if err != nil {
		return nil, err
	}

	txid := tx.TxHash()
	return &txid, nil
}

func (w *BitcoinCashWallet) createMultisigSignature(ins []wi.TransactionInput, outs []wi.TransactionOutput, key *hd.ExtendedKey, redeemScript []byte, feePerByte uint64) ([]wi.Signature, error) {
	var sigs []wi.Signature
	tx := wire.NewMsgTx(1)
	for _, in := range ins {
		ch, err := chainhash.NewHashFromStr(hex.EncodeToString(in.OutpointHash))
		if err != nil {
			return sigs, err
		}
		outpoint := wire.NewOutPoint(ch, in.OutpointIndex)
		input := wire.NewTxIn(outpoint, []byte{}, [][]byte{})
		tx.TxIn = append(tx.TxIn, input)
	}
	for _, out := range outs {
		output := wire.NewTxOut(out.Value, out.Address.ScriptAddress())
		tx.TxOut = append(tx.TxOut, output)
	}

	// Subtract fee
	txType := P2SH_2of3_Multisig
	_, err := spvwallet.LockTimeFromRedeemScript(redeemScript)
	if err == nil {
		txType = P2SH_Multisig_Timelock_2Sigs
	}
	estimatedSize := EstimateSerializeSize(len(ins), tx.TxOut, false, txType)
	fee := estimatedSize * int(feePerByte)
	if len(tx.TxOut) > 0 {
		feePerOutput := fee / len(tx.TxOut)
		for _, output := range tx.TxOut {
			output.Value -= int64(feePerOutput)
		}
	}

	// BIP 69 sorting
	txsort.InPlaceSort(tx)

	signingKey, err := key.ECPrivKey()
	if err != nil {
		return sigs, err
	}

	for i := range tx.TxIn {
		sig, err := bchutil.RawTxInSignature(tx, i, redeemScript, txscript.SigHashAll, signingKey, ins[i].Value)
		if err != nil {
			continue
		}
		bs := wi.Signature{InputIndex: uint32(i), Signature: sig}
		sigs = append(sigs, bs)
	}
	return sigs, nil
}

func (w *BitcoinCashWallet) multisign(ins []wi.TransactionInput, outs []wi.TransactionOutput, sigs1 []wi.Signature, sigs2 []wi.Signature, redeemScript []byte, feePerByte uint64, broadcast bool) ([]byte, error) {
	tx := wire.NewMsgTx(1)
	for _, in := range ins {
		ch, err := chainhash.NewHashFromStr(hex.EncodeToString(in.OutpointHash))
		if err != nil {
			return nil, err
		}
		outpoint := wire.NewOutPoint(ch, in.OutpointIndex)
		input := wire.NewTxIn(outpoint, []byte{}, [][]byte{})
		tx.TxIn = append(tx.TxIn, input)
	}
	for _, out := range outs {
		output := wire.NewTxOut(out.Value, out.Address.ScriptAddress())
		tx.TxOut = append(tx.TxOut, output)
	}

	// Subtract fee
	txType := P2SH_2of3_Multisig
	_, err := spvwallet.LockTimeFromRedeemScript(redeemScript)
	if err == nil {
		txType = P2SH_Multisig_Timelock_2Sigs
	}
	estimatedSize := EstimateSerializeSize(len(ins), tx.TxOut, false, txType)
	fee := estimatedSize * int(feePerByte)
	if len(tx.TxOut) > 0 {
		feePerOutput := fee / len(tx.TxOut)
		for _, output := range tx.TxOut {
			output.Value -= int64(feePerOutput)
		}
	}

	// BIP 69 sorting
	txsort.InPlaceSort(tx)

	// Check if time locked
	var timeLocked bool
	if redeemScript[0] == txscript.OP_IF {
		timeLocked = true
	}

	for i, input := range tx.TxIn {
		var sig1 []byte
		var sig2 []byte
		for _, sig := range sigs1 {
			if int(sig.InputIndex) == i {
				sig1 = sig.Signature
			}
		}
		for _, sig := range sigs2 {
			if int(sig.InputIndex) == i {
				sig2 = sig.Signature
			}
		}
		builder := txscript.NewScriptBuilder()
		builder.AddOp(txscript.OP_0)
		builder.AddData(sig1)
		builder.AddData(sig2)

		if timeLocked {
			builder.AddOp(txscript.OP_1)
		}

		builder.AddData(redeemScript)
		scriptSig, err := builder.Script()
		if err != nil {
			return nil, err
		}
		input.SignatureScript = scriptSig
	}
	// broadcast
	var buf bytes.Buffer
	tx.BtcEncode(&buf, wire.ProtocolVersion, wire.BaseEncoding)
	if broadcast {
		_, err = w.client.Broadcast(buf.Bytes())
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (w *BitcoinCashWallet) generateMultisigScript(keys []hd.ExtendedKey, threshold int, timeout time.Duration, timeoutKey *hd.ExtendedKey) (addr btc.Address, redeemScript []byte, err error) {
	if uint32(timeout.Hours()) > 0 && timeoutKey == nil {
		return nil, nil, errors.New("Timeout key must be non nil when using an escrow timeout")
	}

	if len(keys) < threshold {
		return nil, nil, fmt.Errorf("unable to generate multisig script with "+
			"%d required signatures when there are only %d public "+
			"keys available", threshold, len(keys))
	}

	var ecKeys []*btcec.PublicKey
	for _, key := range keys {
		ecKey, err := key.ECPubKey()
		if err != nil {
			return nil, nil, err
		}
		ecKeys = append(ecKeys, ecKey)
	}

	builder := txscript.NewScriptBuilder()
	if uint32(timeout.Hours()) == 0 {

		builder.AddInt64(int64(threshold))
		for _, key := range ecKeys {
			builder.AddData(key.SerializeCompressed())
		}
		builder.AddInt64(int64(len(ecKeys)))
		builder.AddOp(txscript.OP_CHECKMULTISIG)

	} else {
		ecKey, err := timeoutKey.ECPubKey()
		if err != nil {
			return nil, nil, err
		}
		sequenceLock := blockchain.LockTimeToSequence(false, uint32(timeout.Hours()*6))
		builder.AddOp(txscript.OP_IF)
		builder.AddInt64(int64(threshold))
		for _, key := range ecKeys {
			builder.AddData(key.SerializeCompressed())
		}
		builder.AddInt64(int64(len(ecKeys)))
		builder.AddOp(txscript.OP_CHECKMULTISIG)
		builder.AddOp(txscript.OP_ELSE).
			AddInt64(int64(sequenceLock)).
			AddOp(txscript.OP_CHECKSEQUENCEVERIFY).
			AddOp(txscript.OP_DROP).
			AddData(ecKey.SerializeCompressed()).
			AddOp(txscript.OP_CHECKSIG).
			AddOp(txscript.OP_ENDIF)
	}
	redeemScript, err = builder.Script()
	if err != nil {
		return nil, nil, err
	}
	addr, err = bchutil.NewCashAddressScriptHash(redeemScript, w.params)
	if err != nil {
		return nil, nil, err
	}
	return addr, redeemScript, nil
}

func (w *BitcoinCashWallet) estimateSpendFee(amount int64, feeLevel wi.FeeLevel) (uint64, error) {
	// Since this is an estimate we can use a dummy output address. Let's use a long one so we don't under estimate.
	addr, err := btc.DecodeAddress("qr9chzpkewqq5j3mqcv3l4q82ter772f4cuvc5elvd", w.params)
	if err != nil {
		return 0, err
	}
	tx, err := w.buildTx(amount, addr, feeLevel, nil)
	if err != nil {
		return 0, err
	}
	var outval int64
	for _, output := range tx.TxOut {
		outval += output.Value
	}
	var inval int64
	utxos, err := w.db.Utxos().GetAll()
	if err != nil {
		return 0, err
	}
	for _, input := range tx.TxIn {
		for _, utxo := range utxos {
			if utxo.Op.Hash.IsEqual(&input.PreviousOutPoint.Hash) && utxo.Op.Index == input.PreviousOutPoint.Index {
				inval += utxo.Value
				break
			}
		}
	}
	if inval < outval {
		return 0, errors.New("Error building transaction: inputs less than outputs")
	}
	return uint64(inval - outval), err
}
