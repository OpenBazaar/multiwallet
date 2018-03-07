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

type MockDatastore struct {
	keys           wallet.Keys
	utxos          wallet.Utxos
	stxos          wallet.Stxos
	txns           wallet.Txns
	watchedScripts wallet.WatchedScripts
}

type MockMultiwalletDatastore struct {
	db map[wallet.CoinType]wallet.Datastore
}

func (m *MockMultiwalletDatastore) GetDatastoreForWallet(coinType wallet.CoinType) (wallet.Datastore, error) {
	db, ok := m.db[coinType]
	if !ok {
		return nil, errors.New("Cointype not supported")
	}
	return db, nil
}

func NewMockMultiwalletDatastore() *MockMultiwalletDatastore {
	db := make(map[wallet.CoinType]wallet.Datastore)
	db[wallet.Bitcoin] = wallet.Datastore(&MockDatastore{
		&MockKeyStore{make(map[string]*KeyStoreEntry)},
		&MockUtxoStore{make(map[string]*wallet.Utxo)},
		&MockStxoStore{make(map[string]*wallet.Stxo)},
		&MockTxnStore{make(map[string]*txnStoreEntry)},
		&MockWatchedScriptsStore{make(map[string][]byte)},
	})
	db[wallet.BitcoinCash] = wallet.Datastore(&MockDatastore{
		&MockKeyStore{make(map[string]*KeyStoreEntry)},
		&MockUtxoStore{make(map[string]*wallet.Utxo)},
		&MockStxoStore{make(map[string]*wallet.Stxo)},
		&MockTxnStore{make(map[string]*txnStoreEntry)},
		&MockWatchedScriptsStore{make(map[string][]byte)},
	})
	db[wallet.Zcash] = wallet.Datastore(&MockDatastore{
		&MockKeyStore{make(map[string]*KeyStoreEntry)},
		&MockUtxoStore{make(map[string]*wallet.Utxo)},
		&MockStxoStore{make(map[string]*wallet.Stxo)},
		&MockTxnStore{make(map[string]*txnStoreEntry)},
		&MockWatchedScriptsStore{make(map[string][]byte)},
	})
	db[wallet.Litecoin] = wallet.Datastore(&MockDatastore{
		&MockKeyStore{make(map[string]*KeyStoreEntry)},
		&MockUtxoStore{make(map[string]*wallet.Utxo)},
		&MockStxoStore{make(map[string]*wallet.Stxo)},
		&MockTxnStore{make(map[string]*txnStoreEntry)},
		&MockWatchedScriptsStore{make(map[string][]byte)},
	})
	return &MockMultiwalletDatastore{db}
}

func (m *MockDatastore) Keys() wallet.Keys {
	return m.keys
}

func (m *MockDatastore) Utxos() wallet.Utxos {
	return m.utxos
}

func (m *MockDatastore) Stxos() wallet.Stxos {
	return m.stxos
}

func (m *MockDatastore) Txns() wallet.Txns {
	return m.txns
}

func (m *MockDatastore) WatchedScripts() wallet.WatchedScripts {
	return m.watchedScripts
}

type KeyStoreEntry struct {
	ScriptAddress []byte
	Path          wallet.KeyPath
	Used          bool
	Key           *btcec.PrivateKey
}

type MockKeyStore struct {
	Keys map[string]*KeyStoreEntry
}

func (m *MockKeyStore) Put(scriptAddress []byte, keyPath wallet.KeyPath) error {
	m.Keys[hex.EncodeToString(scriptAddress)] = &KeyStoreEntry{scriptAddress, keyPath, false, nil}
	return nil
}

func (m *MockKeyStore) ImportKey(scriptAddress []byte, key *btcec.PrivateKey) error {
	kp := wallet.KeyPath{Purpose: wallet.EXTERNAL, Index: -1}
	m.Keys[hex.EncodeToString(scriptAddress)] = &KeyStoreEntry{scriptAddress, kp, false, key}
	return nil
}

func (m *MockKeyStore) MarkKeyAsUsed(scriptAddress []byte) error {
	key, ok := m.Keys[hex.EncodeToString(scriptAddress)]
	if !ok {
		return errors.New("key does not exist")
	}
	key.Used = true
	return nil
}

func (m *MockKeyStore) GetLastKeyIndex(purpose wallet.KeyPurpose) (int, bool, error) {
	i := -1
	used := false
	for _, key := range m.Keys {
		if key.Path.Purpose == purpose && key.Path.Index > i {
			i = key.Path.Index
			used = key.Used
		}
	}
	if i == -1 {
		return i, used, errors.New("No saved keys")
	}
	return i, used, nil
}

func (m *MockKeyStore) GetPathForKey(scriptAddress []byte) (wallet.KeyPath, error) {
	key, ok := m.Keys[hex.EncodeToString(scriptAddress)]
	if !ok || key.Path.Index == -1 {
		return wallet.KeyPath{}, errors.New("key does not exist")
	}
	return key.Path, nil
}

func (m *MockKeyStore) GetKey(scriptAddress []byte) (*btcec.PrivateKey, error) {
	for _, k := range m.Keys {
		if k.Path.Index == -1 && bytes.Equal(scriptAddress, k.ScriptAddress) {
			return k.Key, nil
		}
	}
	return nil, errors.New("Not found")
}

func (m *MockKeyStore) GetImported() ([]*btcec.PrivateKey, error) {
	var keys []*btcec.PrivateKey
	for _, k := range m.Keys {
		if k.Path.Index == -1 {
			keys = append(keys, k.Key)
		}
	}
	return keys, nil
}

func (m *MockKeyStore) GetUnused(purpose wallet.KeyPurpose) ([]int, error) {
	var i []int
	for _, key := range m.Keys {
		if !key.Used && key.Path.Purpose == purpose {
			i = append(i, key.Path.Index)
		}
	}
	sort.Ints(i)
	return i, nil
}

func (m *MockKeyStore) GetAll() ([]wallet.KeyPath, error) {
	var kp []wallet.KeyPath
	for _, key := range m.Keys {
		kp = append(kp, key.Path)
	}
	return kp, nil
}

func (m *MockKeyStore) GetLookaheadWindows() map[wallet.KeyPurpose]int {
	internalLastUsed := -1
	externalLastUsed := -1
	for _, key := range m.Keys {
		if key.Path.Purpose == wallet.INTERNAL && key.Used && key.Path.Index > internalLastUsed {
			internalLastUsed = key.Path.Index
		}
		if key.Path.Purpose == wallet.EXTERNAL && key.Used && key.Path.Index > externalLastUsed {
			externalLastUsed = key.Path.Index
		}
	}
	internalUnused := 0
	externalUnused := 0
	for _, key := range m.Keys {
		if key.Path.Purpose == wallet.INTERNAL && !key.Used && key.Path.Index > internalLastUsed {
			internalUnused++
		}
		if key.Path.Purpose == wallet.EXTERNAL && !key.Used && key.Path.Index > externalLastUsed {
			externalUnused++
		}
	}
	mp := make(map[wallet.KeyPurpose]int)
	mp[wallet.INTERNAL] = internalUnused
	mp[wallet.EXTERNAL] = externalUnused
	return mp
}

type MockUtxoStore struct {
	utxos map[string]*wallet.Utxo
}

func (m *MockUtxoStore) Put(utxo wallet.Utxo) error {
	key := utxo.Op.Hash.String() + ":" + strconv.Itoa(int(utxo.Op.Index))
	m.utxos[key] = &utxo
	return nil
}

func (m *MockUtxoStore) GetAll() ([]wallet.Utxo, error) {
	var utxos []wallet.Utxo
	for _, v := range m.utxos {
		utxos = append(utxos, *v)
	}
	return utxos, nil
}

func (m *MockUtxoStore) SetWatchOnly(utxo wallet.Utxo) error {
	key := utxo.Op.Hash.String() + ":" + strconv.Itoa(int(utxo.Op.Index))
	u, ok := m.utxos[key]
	if !ok {
		return errors.New("Not found")
	}
	u.WatchOnly = true
	return nil
}

func (m *MockUtxoStore) Delete(utxo wallet.Utxo) error {
	key := utxo.Op.Hash.String() + ":" + strconv.Itoa(int(utxo.Op.Index))
	_, ok := m.utxos[key]
	if !ok {
		return errors.New("Not found")
	}
	delete(m.utxos, key)
	return nil
}

type MockStxoStore struct {
	stxos map[string]*wallet.Stxo
}

func (m *MockStxoStore) Put(stxo wallet.Stxo) error {
	m.stxos[stxo.SpendTxid.String()] = &stxo
	return nil
}

func (m *MockStxoStore) GetAll() ([]wallet.Stxo, error) {
	var stxos []wallet.Stxo
	for _, v := range m.stxos {
		stxos = append(stxos, *v)
	}
	return stxos, nil
}

func (m *MockStxoStore) Delete(stxo wallet.Stxo) error {
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

type MockTxnStore struct {
	txns map[string]*txnStoreEntry
}

func (m *MockTxnStore) Put(txn *wire.MsgTx, value, height int, timestamp time.Time, watchOnly bool) error {
	m.txns[txn.TxHash().String()] = &txnStoreEntry{
		txn:       txn,
		value:     value,
		height:    height,
		timestamp: timestamp,
		watchOnly: watchOnly,
	}
	return nil
}

func (m *MockTxnStore) Get(txid chainhash.Hash) (*wire.MsgTx, wallet.Txn, error) {
	t, ok := m.txns[txid.String()]
	if !ok {
		return nil, wallet.Txn{}, errors.New("Not found")
	}
	var buf bytes.Buffer
	t.txn.Serialize(&buf)
	return t.txn, wallet.Txn{txid.String(), int64(t.value), int32(t.height), t.timestamp, t.watchOnly, buf.Bytes()}, nil
}

func (m *MockTxnStore) GetAll(includeWatchOnly bool) ([]wallet.Txn, error) {
	var txns []wallet.Txn
	for _, t := range m.txns {
		var buf bytes.Buffer
		t.txn.Serialize(&buf)
		txn := wallet.Txn{t.txn.TxHash().String(), int64(t.value), int32(t.height), t.timestamp, t.watchOnly, buf.Bytes()}
		txns = append(txns, txn)
	}
	return txns, nil
}

func (m *MockTxnStore) UpdateHeight(txid chainhash.Hash, height int) error {
	txn, ok := m.txns[txid.String()]
	if !ok {
		return errors.New("Not found")
	}
	txn.height = height
	return nil
}

func (m *MockTxnStore) Delete(txid *chainhash.Hash) error {
	_, ok := m.txns[txid.String()]
	if !ok {
		return errors.New("Not found")
	}
	delete(m.txns, txid.String())
	return nil
}

type MockWatchedScriptsStore struct {
	scripts map[string][]byte
}

func (m *MockWatchedScriptsStore) Put(scriptPubKey []byte) error {
	m.scripts[hex.EncodeToString(scriptPubKey)] = scriptPubKey
	return nil
}

func (m *MockWatchedScriptsStore) GetAll() ([][]byte, error) {
	var ret [][]byte
	for _, b := range m.scripts {
		ret = append(ret, b)
	}
	return ret, nil
}

func (m *MockWatchedScriptsStore) Delete(scriptPubKey []byte) error {
	enc := hex.EncodeToString(scriptPubKey)
	_, ok := m.scripts[enc]
	if !ok {
		return errors.New("Not found")
	}
	delete(m.scripts, enc)
	return nil
}
