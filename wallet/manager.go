package wallet

import (
	"encoding/hex"
	"github.com/OpenBazaar/multiwallet/client"
	"github.com/OpenBazaar/multiwallet/keys"
	"github.com/OpenBazaar/multiwallet/litecoin"
	"github.com/OpenBazaar/multiwallet/zcash"
	"github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/cpacia/bchutil"
	"github.com/op/go-logging"
	"strconv"
	"sync"
	"time"
)

var log = logging.MustGetLogger("walletManager")

type WalletManager struct {
	db       wallet.Datastore
	km       *keys.KeyManager
	client   client.APIClient
	params   *chaincfg.Params
	coinType wallet.CoinType

	chainHeight uint32

	lock sync.RWMutex
}

func NewWalletManager(db wallet.Datastore, km *keys.KeyManager, client client.APIClient, params *chaincfg.Params, coinType wallet.CoinType) *WalletManager {
	return &WalletManager{db, km, client, params, coinType, 0, sync.RWMutex{}}
}

func (m *WalletManager) Start() {
	log.Noticef("Starting %s WalletManager", m.coinType.String())
	go m.updateState()
	go m.listen()
}

func (m *WalletManager) listen() {
	addrs := m.getStoredAddresses()
	for _, sa := range addrs {
		m.client.ListenAddress(sa.Addr)
	}

	for {
		select {
		case tx := <- m.client.TransactionNotify():
			go m.processIncomingTransaction(tx)
		case block := <- m.client.BlockNotify():
			go m.processIncomingBlock(block)
		}
	}
}

// This is a transaction fresh off the wire. Let's save it to the db.
func (m *WalletManager) processIncomingTransaction(tx client.Transaction) {
	addrs := m.getStoredAddresses()
	m.lock.RLock()
	chainHeight := int32(m.chainHeight)
	m.lock.RUnlock()
	m.saveSingleTxToDB(tx, chainHeight, addrs)
}

// A new block was found, let's update our state to make sure all heights are accurate in case there
// was a reorg. TODO: is there a more efficient way to do this?
func (m *WalletManager) processIncomingBlock(block client.Block) {
	m.updateState()
}

// updateState will query the API for both UTXOs and TXs relevant to our wallet and then update
// the db state to match the API responses.
func (m *WalletManager) updateState() {
	// Start by fetching the chain height from the API
	log.Debugf("Querying for %s chain height", m.coinType.String())
	info, err := m.client.GetInfo()
	if err == nil {
		log.Debugf("%s chain height: %d", m.coinType.String(), info.Blocks)
		m.lock.Lock()
		m.chainHeight = uint32(info.Blocks)
		m.lock.Unlock()
	}  else {
		log.Error("Error querying API for chain height: %s", err.Error())
	}

	// Load wallet addresses and watch only addresses from the db
	addrs := m.getStoredAddresses()

	go m.syncUtxos(addrs)
	go m.syncTxs(addrs)

}

// Query API for UTXOs and synchronize db state
func (m *WalletManager) syncUtxos(addrs map[string]storedAddress) {
	log.Debugf("Querying for %s utxos", m.coinType.String())
	var query []btcutil.Address
	for _, sa := range addrs {
		query = append(query, sa.Addr)
	}
	utxos, err := m.client.GetUtxos(query)
	if err != nil {
		log.Error("Error downloading utxos for %s: %s", m.coinType.String(), err.Error())
	} else {
		log.Debugf("Downloaded %d %s utxos", len(utxos), m.coinType.String())
		m.saveUtxosToDB(utxos, addrs)
	}
}

// For each API response we will have to figure out height at which the UTXO has confirmed (if it has) and
// build a UTXO object suitable for saving to the database. If the database contains any UTXOs not returned
// by the API we will delete them.
func (m *WalletManager) saveUtxosToDB(utxos []client.Utxo, addrs map[string]storedAddress) {
	// Get current utxos
	currentUtxos, err := m.db.Utxos().GetAll()
	if err != nil {
		log.Error("Error loading utxos for %s: %s", m.coinType.String(), err.Error())
		return
	}

	m.lock.RLock()
	chainHeight := int32(m.chainHeight)
	m.lock.RUnlock()

	newUtxos := make(map[string]wallet.Utxo)
	// Iterate over new utxos and put them to the db
	for _, u := range utxos {
		ch, err := chainhash.NewHashFromStr(u.Txid)
		if err != nil {
			log.Error("Error converting to chainhash for %s: %s", m.coinType.String(), err.Error())
			continue
		}
		scriptBytes, err := hex.DecodeString(u.ScriptPubKey)
		if err != nil {
			log.Error("Error converting to script bytes for %s: %s", m.coinType.String(), err.Error())
			continue
		}
		var watchOnly bool
		sa, ok := addrs[u.Address]
		if ok {
			watchOnly = sa.WatchOnly
		}
		height := int32(0)
		if u.Confirmations > 0 {
			height = chainHeight - (int32(u.Confirmations) - 1)
		}

		newU := wallet.Utxo{
			Op:           *wire.NewOutPoint(ch, uint32(u.Vout)),
			Value:        u.Satoshis,
			WatchOnly:    watchOnly,
			ScriptPubkey: scriptBytes,
			AtHeight:     height,
		}
		newUtxos[serializeUtxo(newU)] = newU

		m.db.Utxos().Put(newU)
	}
	// If any old utxos were not returned by the API, delete them.
	for _, cur := range currentUtxos {
		_, ok := newUtxos[serializeUtxo(cur)]
		if !ok {
			m.db.Utxos().Delete(cur)
		}
	}
}

// For use as a map key
func serializeUtxo(u wallet.Utxo) string {
	ser := u.Op.Hash.String()
	ser += strconv.Itoa(int(u.Op.Index))
	return ser
}

// Query API for TXs and synchronize db state
func (m *WalletManager) syncTxs(addrs map[string]storedAddress) {
	log.Debugf("Querying for %s transactions", m.coinType.String())
	var query []btcutil.Address
	for _, sa := range addrs {
		query = append(query, sa.Addr)
	}
	txs, err := m.client.GetTransactions(query)
	if err != nil {
		log.Error("Error downloading txs for %s: %s", m.coinType.String(), err.Error())
	} else {
		log.Debugf("Downloaded %d %s transactions", len(txs), m.coinType.String())
		m.saveTxsToDB(txs, addrs)
	}
}

// For each API response we will need to determine the net coins leaving/entering the wallet as well as determine
// if the transaction was exclusively for our `watch only` addresses. We will also build a Tx object suitable
// for saving to the db and delete any existing txs not returned by the API. Finally, for any output matching a key
// in our wallet we need to mark that key as used in the db
func (m *WalletManager) saveTxsToDB(txns []client.Transaction, addrs map[string]storedAddress) {
	// Get current utxos
	currentTxs, err := m.db.Txns().GetAll(true)
	if err != nil {
		log.Error("Error loading utxos for %s: %s", m.coinType.String(), err.Error())
		return
	}

	m.lock.RLock()
	chainHeight := int32(m.chainHeight)
	m.lock.RUnlock()

	newTxs := make(map[string]bool)
	// Iterate over new utxos and put them to the db
	for _, u := range txns {
		m.saveSingleTxToDB(u, chainHeight, addrs)
		newTxs[u.Txid] = true
	}
	// If any old utxos were not returned by the API, delete them.
	for _, cur := range currentTxs {
		if !newTxs[cur.Txid]{
			ch, err := chainhash.NewHashFromStr(cur.Txid)
			if err != nil {
				log.Error("Error converting to chainhash for %s: %s", m.coinType.String(), err.Error())
				continue
			}
			m.db.Txns().Delete(ch)
		}
	}
}

func (m *WalletManager) saveSingleTxToDB(u client.Transaction, chainHeight int32, addrs map[string]storedAddress) {
	msgTx := wire.NewMsgTx(int32(u.Version))
	msgTx.LockTime = uint32(u.Locktime)
	hits := 0
	value := int64(0)

	txHash, err := chainhash.NewHashFromStr(u.Txid)
	if err != nil {
		log.Error("Error converting to txHash for %s: %s", m.coinType.String(), err.Error())
		return
	}
	for _, in := range u.Inputs {
		ch, err := chainhash.NewHashFromStr(in.Txid)
		if err != nil {
			log.Error("Error converting to chainhash for %s: %s", m.coinType.String(), err.Error())
			continue
		}
		op := wire.NewOutPoint(ch, uint32(in.Vout))
		script, err := hex.DecodeString(in.ScriptSig.Hex)
		if err != nil {
			log.Error("Error converting to scriptSig for %s: %s", m.coinType.String(), err.Error())
			continue
		}
		txin := wire.NewTxIn(op, script, [][]byte{})
		txin.Sequence = uint32(in.Sequence)
		msgTx.TxIn = append(msgTx.TxIn, txin)
		sa, ok := addrs[in.Addr]
		if ok && !sa.WatchOnly {
			hits++
			value -= in.Satoshis
		}
	}
	for _, out := range u.Outputs {
		script, err := hex.DecodeString(out.ScriptPubKey.Hex)
		if err != nil {
			log.Error("Error converting to scriptPubkey for %s: %s", m.coinType.String(), err.Error())
			continue
		}
		if len(out.ScriptPubKey.Addresses) == 0 {
			continue
		}
		v := int64(out.Value * 100000000) // TODO: need to use correct number of satoshi for each coin
		sa, ok := addrs[out.ScriptPubKey.Addresses[0]]
		if ok && !sa.WatchOnly {
			hits++
			value += v

			// Mark the key we received coins to as used
			m.db.Keys().MarkKeyAsUsed(sa.Addr.ScriptAddress())
		}
		txout := wire.NewTxOut(v, script)
		msgTx.TxOut = append(msgTx.TxOut, txout)
	}
	height := int32(0)
	if u.Confirmations > 0 {
		height = chainHeight - (int32(u.Confirmations) - 1)
	}

	// TODO: the db interface might need to change here to accept a txid and serialized tx rather than the wire.MsgTx
	// the reason is that it seems unlikely the txhash would be calculated the same way for each coin we support.

	// TODO: Fire tx listener if new tx or if height is changing
	if err != nil {
		m.db.Txns().Put(msgTx, int(value), int(height), time.Now(), hits == 0)
	} else {
		m.db.Txns().UpdateHeight(*txHash, int(height))
	}
}

type storedAddress struct {
	Addr      btcutil.Address
	WatchOnly bool
}

func (m *WalletManager) getStoredAddresses() map[string]storedAddress {
	keys := m.km.GetKeys()
	addrs := make(map[string]storedAddress)
	for _, key := range keys {
		addr, err := m.km.KeyToAddress(key)
		if err != nil {
			log.Errorf("Error getting %s address for key: %s", m.coinType.String(), err.Error())
			continue
		}
		addrs[addr.String()] = storedAddress{addr, false}
	}
	watchScripts, err := m.db.WatchedScripts().GetAll()
	if err != nil {
		log.Errorf("Error loading %s watch scripts: %s", m.coinType.String(), err.Error())
	} else {
		for _, script := range watchScripts {
			switch m.coinType {
			case wallet.Bitcoin:
				addr, err := btcutil.NewAddressScriptHash(script, m.params)
				if err != nil {
					log.Errorf("Error serializing %s script: %s", m.coinType.String(), err.Error())
					continue
				}
				addrs[addr.String()] = storedAddress{addr, true}
			case wallet.BitcoinCash:
				addr, err := bchutil.NewCashAddressScriptHash(script, m.params)
				if err != nil {
					log.Errorf("Error serializing %s script: %s", m.coinType.String(), err.Error())
					continue
				}
				addrs[addr.String()] = storedAddress{addr, true}
			case wallet.Zcash:
				addr, err := zcash.NewAddressScriptHash(script, m.params)
				if err != nil {
					log.Errorf("Error serializing %s script: %s", m.coinType.String(), err.Error())
					continue
				}
				addrs[addr.String()] = storedAddress{addr, true}
			case wallet.Litecoin:
				addr, err := litecoin.NewAddressScriptHash(script, m.params)
				if err != nil {
					log.Errorf("Error serializing %s script: %s", m.coinType.String(), err.Error())
					continue
				}
				addrs[addr.String()] = storedAddress{addr, true}
			}
		}
	}
	return addrs
}