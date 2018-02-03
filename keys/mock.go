package keys

import (
	"encoding/hex"
	"bytes"
	"sort"
	"github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/btcec"
	"errors"
)

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

