package multiwallet

import (
	"github.com/OpenBazaar/multiwallet/config"
	"github.com/OpenBazaar/wallet-interface"
	"time"
	"github.com/op/go-logging"
	hd "github.com/btcsuite/btcutil/hdkeychain"
	"github.com/tyler-smith/go-bip39"
	"github.com/OpenBazaar/multiwallet/bitcoin"
	client2 "github.com/OpenBazaar/multiwallet/client"
)

var log = logging.MustGetLogger("multiwallet")

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
		var w wallet.Wallet
		switch(coin.CoinType) {
		case wallet.Bitcoin:
			client, err := client2.NewInsightClient(coin.ClientAPI.String(), cfg.Proxy)
			if err != nil {
				return nil, err
			}
			w, err = bitcoin.NewBitcoinWallet(coin.DB, mPrivKey, client, cfg.Params)
			if err != nil {
				return nil, err
			}
			wallets[coin.CoinType] = w
		}
	}
	return &MultiWallet{wallets}, nil
}

