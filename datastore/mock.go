package datastore

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"sort"
	"strconv"
	"time"
)

type coinDB struct {
	keys           wallet.Keys
	utxos          wallet.Utxos
	stxos          wallet.Stxos
	txns           wallet.Txns
	watchedScripts wallet.WatchedScripts
}

type MockDatastore struct {
	db map[wallet.CoinType]coinDB
}

func NewMockDatastore() *MockDatastore {
	db := make(map[wallet.CoinType]coinDB)
	db[wallet.Bitcoin] = coinDB{
		&mockKeyStore{make(map[string]*keyStoreEntry)},
		&mockUtxoStore{make(map[string]*wallet.Utxo)},
		&mockStxoStore{make(map[string]*wallet.Stxo)},
		&mockTxnStore{make(map[string]*txnStoreEntry)},
		&mockWatchedScriptsStore{make(map[string][]byte)},
	}
	db[wallet.BitcoinCash] = coinDB{
		&mockKeyStore{make(map[string]*keyStoreEntry)},
		&mockUtxoStore{make(map[string]*wallet.Utxo)},
		&mockStxoStore{make(map[string]*wallet.Stxo)},
		&mockTxnStore{make(map[string]*txnStoreEntry)},
		&mockWatchedScriptsStore{make(map[string][]byte)},
	}
	db[wallet.Zcash] = coinDB{
		&mockKeyStore{make(map[string]*keyStoreEntry)},
		&mockUtxoStore{make(map[string]*wallet.Utxo)},
		&mockStxoStore{make(map[string]*wallet.Stxo)},
		&mockTxnStore{make(map[string]*txnStoreEntry)},
		&mockWatchedScriptsStore{make(map[string][]byte)},
	}
	db[wallet.Litecoin] = coinDB{
		&mockKeyStore{make(map[string]*keyStoreEntry)},
		&mockUtxoStore{make(map[string]*wallet.Utxo)},
		&mockStxoStore{make(map[string]*wallet.Stxo)},
		&mockTxnStore{make(map[string]*txnStoreEntry)},
		&mockWatchedScriptsStore{make(map[string][]byte)},
	}
	return &MockDatastore{db}
}

func (m *MockDatastore) Keys(coinType wallet.CoinType) wallet.Keys {
	return m.db[coinType].keys
}

func (m *MockDatastore) Utxos(coinType wallet.CoinType) wallet.Utxos {
	return m.db[coinType].utxos
}

func (m *MockDatastore) Stxos(coinType wallet.CoinType) wallet.Stxos {
	return m.db[coinType].stxos
}

func (m *MockDatastore) Txns(coinType wallet.CoinType) wallet.Txns {
	return m.db[coinType].txns
}

func (m *MockDatastore) WatchedScripts(coinType wallet.CoinType) wallet.WatchedScripts {
	return m.db[coinType].watchedScripts
}

type keyStoreEntry struct {
	scriptAddress []byte
	path          wallet.KeyPath
	used          bool
	key           *btcec.PrivateKey
}

type mockKeyStore struct {
	keys map[string]*keyStoreEntry
}

func (m *mockKeyStore) Put(scriptAddress []byte, keyPath wallet.KeyPath) error {
	m.keys[hex.EncodeToString(scriptAddress)] = &keyStoreEntry{scriptAddress, keyPath, false, nil}
	return nil
}

func (m *mockKeyStore) ImportKey(scriptAddress []byte, key *btcec.PrivateKey) error {
	kp := wallet.KeyPath{Purpose: wallet.EXTERNAL, Index: -1}
	m.keys[hex.EncodeToString(scriptAddress)] = &keyStoreEntry{scriptAddress, kp, false, key}
	return nil
}

func (m *mockKeyStore) MarkKeyAsUsed(scriptAddress []byte) error {
	key, ok := m.keys[hex.EncodeToString(scriptAddress)]
	if !ok {
		return errors.New("key does not exist")
	}
	key.used = true
	return nil
}

func (m *mockKeyStore) GetLastKeyIndex(purpose wallet.KeyPurpose) (int, bool, error) {
	i := -1
	used := false
	for _, key := range m.keys {
		if key.path.Purpose == purpose && key.path.Index > i {
			i = key.path.Index
			used = key.used
		}
	}
	if i == -1 {
		return i, used, errors.New("No saved keys")
	}
	return i, used, nil
}

func (m *mockKeyStore) GetPathForKey(scriptAddress []byte) (wallet.KeyPath, error) {
	key, ok := m.keys[hex.EncodeToString(scriptAddress)]
	if !ok || key.path.Index == -1 {
		return wallet.KeyPath{}, errors.New("key does not exist")
	}
	return key.path, nil
}

func (m *mockKeyStore) GetKey(scriptAddress []byte) (*btcec.PrivateKey, error) {
	for _, k := range m.keys {
		if k.path.Index == -1 && bytes.Equal(scriptAddress, k.scriptAddress) {
			return k.key, nil
		}
	}
	return nil, errors.New("Not found")
}

func (m *mockKeyStore) GetImported() ([]*btcec.PrivateKey, error) {
	var keys []*btcec.PrivateKey
	for _, k := range m.keys {
		if k.path.Index == -1 {
			keys = append(keys, k.key)
		}
	}
	return keys, nil
}

func (m *mockKeyStore) GetUnused(purpose wallet.KeyPurpose) ([]int, error) {
	var i []int
	for _, key := range m.keys {
		if !key.used && key.path.Purpose == purpose {
			i = append(i, key.path.Index)
		}
	}
	sort.Ints(i)
	return i, nil
}

func (m *mockKeyStore) GetAll() ([]wallet.KeyPath, error) {
	var kp []wallet.KeyPath
	for _, key := range m.keys {
		kp = append(kp, key.path)
	}
	return kp, nil
}

func (m *mockKeyStore) GetLookaheadWindows() map[wallet.KeyPurpose]int {
	internalLastUsed := -1
	externalLastUsed := -1
	for _, key := range m.keys {
		if key.path.Purpose == wallet.INTERNAL && key.used && key.path.Index > internalLastUsed {
			internalLastUsed = key.path.Index
		}
		if key.path.Purpose == wallet.EXTERNAL && key.used && key.path.Index > externalLastUsed {
			externalLastUsed = key.path.Index
		}
	}
	internalUnused := 0
	externalUnused := 0
	for _, key := range m.keys {
		if key.path.Purpose == wallet.INTERNAL && !key.used && key.path.Index > internalLastUsed {
			internalUnused++
		}
		if key.path.Purpose == wallet.EXTERNAL && !key.used && key.path.Index > externalLastUsed {
			externalUnused++
		}
	}
	mp := make(map[wallet.KeyPurpose]int)
	mp[wallet.INTERNAL] = internalUnused
	mp[wallet.EXTERNAL] = externalUnused
	return mp
}

type mockUtxoStore struct {
	utxos map[string]*wallet.Utxo
}

func (m *mockUtxoStore) Put(utxo wallet.Utxo) error {
	key := utxo.Op.Hash.String() + ":" + strconv.Itoa(int(utxo.Op.Index))
	m.utxos[key] = &utxo
	return nil
}

func (m *mockUtxoStore) GetAll() ([]wallet.Utxo, error) {
	var utxos []wallet.Utxo
	for _, v := range m.utxos {
		utxos = append(utxos, *v)
	}
	return utxos, nil
}

func (m *mockUtxoStore) SetWatchOnly(utxo wallet.Utxo) error {
	key := utxo.Op.Hash.String() + ":" + strconv.Itoa(int(utxo.Op.Index))
	u, ok := m.utxos[key]
	if !ok {
		return errors.New("Not found")
	}
	u.WatchOnly = true
	return nil
}

func (m *mockUtxoStore) Delete(utxo wallet.Utxo) error {
	key := utxo.Op.Hash.String() + ":" + strconv.Itoa(int(utxo.Op.Index))
	_, ok := m.utxos[key]
	if !ok {
		return errors.New("Not found")
	}
	delete(m.utxos, key)
	return nil
}

type mockStxoStore struct {
	stxos map[string]*wallet.Stxo
}

func (m *mockStxoStore) Put(stxo wallet.Stxo) error {
	m.stxos[stxo.SpendTxid.String()] = &stxo
	return nil
}

func (m *mockStxoStore) GetAll() ([]wallet.Stxo, error) {
	var stxos []wallet.Stxo
	for _, v := range m.stxos {
		stxos = append(stxos, *v)
	}
	return stxos, nil
}

func (m *mockStxoStore) Delete(stxo wallet.Stxo) error {
	_, ok := m.stxos[stxo.SpendTxid.String()]
	if !ok {
		return errors.New("Not found")
	}
	delete(m.stxos, stxo.SpendTxid.String())
	return nil
}

type txnStoreEntry struct {
	txn       *wire.MsgTx
	value     int
	height    int
	timestamp time.Time
	watchOnly bool
}

type mockTxnStore struct {
	txns map[string]*txnStoreEntry
}

func (m *mockTxnStore) Put(txn *wire.MsgTx, value, height int, timestamp time.Time, watchOnly bool) error {
	m.txns[txn.TxHash().String()] = &txnStoreEntry{
		txn:       txn,
		value:     value,
		height:    height,
		timestamp: timestamp,
		watchOnly: watchOnly,
	}
	return nil
}

func (m *mockTxnStore) Get(txid chainhash.Hash) (*wire.MsgTx, wallet.Txn, error) {
	t, ok := m.txns[txid.String()]
	if !ok {
		return nil, wallet.Txn{}, errors.New("Not found")
	}
	var buf bytes.Buffer
	t.txn.Serialize(&buf)
	return t.txn, wallet.Txn{txid.String(), int64(t.value), int32(t.height), t.timestamp, t.watchOnly, buf.Bytes()}, nil
}

func (m *mockTxnStore) GetAll(includeWatchOnly bool) ([]wallet.Txn, error) {
	var txns []wallet.Txn
	for _, t := range m.txns {
		var buf bytes.Buffer
		t.txn.Serialize(&buf)
		txn := wallet.Txn{t.txn.TxHash().String(), int64(t.value), int32(t.height), t.timestamp, t.watchOnly, buf.Bytes()}
		txns = append(txns, txn)
	}
	return txns, nil
}

func (m *mockTxnStore) UpdateHeight(txid chainhash.Hash, height int) error {
	txn, ok := m.txns[txid.String()]
	if !ok {
		return errors.New("Not found")
	}
	txn.height = height
	return nil
}

func (m *mockTxnStore) Delete(txid *chainhash.Hash) error {
	_, ok := m.txns[txid.String()]
	if !ok {
		return errors.New("Not found")
	}
	delete(m.txns, txid.String())
	return nil
}

type mockWatchedScriptsStore struct {
	scripts map[string][]byte
}

func (m *mockWatchedScriptsStore) Put(scriptPubKey []byte) error {
	m.scripts[hex.EncodeToString(scriptPubKey)] = scriptPubKey
	return nil
}

func (m *mockWatchedScriptsStore) GetAll() ([][]byte, error) {
	var ret [][]byte
	for _, b := range m.scripts {
		ret = append(ret, b)
	}
	return ret, nil
}

func (m *mockWatchedScriptsStore) Delete(scriptPubKey []byte) error {
	enc := hex.EncodeToString(scriptPubKey)
	_, ok := m.scripts[enc]
	if !ok {
		return errors.New("Not found")
	}
	delete(m.scripts, enc)
	return nil
}
