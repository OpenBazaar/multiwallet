package bitcoin

import (
	"github.com/OpenBazaar/wallet-interface"
	"github.com/OpenBazaar/multiwallet/keys"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
)

type BitcoinWallet struct {
	db wallet.Datastore
	km *keys.KeyManager
	params *chaincfg.Params
}

func NewBitcoinWallet(db wallet.Datastore, masterPrivKey *hdkeychain.ExtendedKey, params *chaincfg.Params) (*BitcoinWallet, error) {
	km, err := keys.NewKeyManager(db.Keys(), params, masterPrivKey, wallet.Bitcoin)
	if err != nil {
		return nil, err
	}
	return &BitcoinWallet{db, km, params}, nil
}