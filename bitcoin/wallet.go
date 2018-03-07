package bitcoin

import (
	"github.com/OpenBazaar/multiwallet/keys"
	"github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/OpenBazaar/multiwallet/client"
)

type BitcoinWallet struct {
	db     wallet.Datastore
	km     *keys.KeyManager
	params *chaincfg.Params
	client client.APIClient
}

func NewBitcoinWallet(db wallet.Datastore, masterPrivKey *hdkeychain.ExtendedKey, client client.APIClient, params *chaincfg.Params) (*BitcoinWallet, error) {
	km, err := keys.NewKeyManager(db.Keys(), params, masterPrivKey, wallet.Bitcoin)
	if err != nil {
		return nil, err
	}
	return &BitcoinWallet{db, km, params, client}, nil
}

