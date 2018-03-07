package multiwallet

import (
	"github.com/OpenBazaar/multiwallet/config"
	"github.com/OpenBazaar/wallet-interface"
	"time"
	"github.com/op/go-logging"
	hd "github.com/btcsuite/btcutil/hdkeychain"
	"github.com/tyler-smith/go-bip39"
	"github.com/OpenBazaar/multiwallet/bitcoin"
)

var log = logging.MustGetLogger("bitcoin")

type MultiWallet struct {
	wallets map[wallet.CoinType]wallet.Wallet
}

func NewMultiWallet(cfg config.Config) (*MultiWallet, error) {
	log.SetBackend(logging.AddModuleLevel(cfg.Logger))

	if cfg.Mnemonic == "" {
		ent, err := bip39.NewEntropy(128)
		if err != nil {
			return nil, err
		}
		mnemonic, err := bip39.NewMnemonic(ent)
		if err != nil {
			return nil, err
		}
		cfg.Mnemonic = mnemonic
		cfg.CreationDate = time.Now()
	}
	seed := bip39.NewSeed(cfg.Mnemonic, "")

	mPrivKey, err := hd.NewMaster(seed, cfg.Params)
	if err != nil {
		return nil, err
	}

	wallets := make(map[wallet.CoinType]wallet.Wallet)
	for _, coin := range cfg.Coins {
		db, err := cfg.DB.GetDatastoreForWallet(coin.CoinType)
		if err != nil {
			return nil, err
		}
		var w wallet.Wallet
		switch(coin.CoinType) {
		case wallet.Bitcoin:
			w, err = bitcoin.NewBitcoinWallet(db, mPrivKey, cfg.Params)
			if err != nil {
				return nil, err
			}
			wallets[coin.CoinType] = w
		}
	}
	return &MultiWallet{wallets}, nil
}

