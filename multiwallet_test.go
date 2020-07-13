package multiwallet

import (
	"github.com/OpenBazaar/multiwallet/cache"
	"github.com/OpenBazaar/multiwallet/config"
	"github.com/OpenBazaar/multiwallet/datastore"
	"github.com/OpenBazaar/wallet-interface"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/op/go-logging"
	"os"
	"testing"
)

func TestMultiWallet_Filecoin(t *testing.T) {
	mdb := datastore.NewMockMultiwalletDatastore()
	db, err := mdb.GetDatastoreForWallet(wallet.Filecoin)
	if err != nil {
		t.Fatal(err)
	}

	logger := logging.NewLogBackend(os.Stdout, "", 0)

	cfg := &config.Config{
		Mnemonic: "abcdefg",
		Params: &chaincfg.MainNetParams,
		Cache: cache.NewMockCacher(),
		Coins: []config.CoinConfig{
			{
				CoinType: wallet.Filecoin,
				DB: db,
				ClientAPIs: []string{"http://localhost:8080/api"},
			},
		},
		Logger: logger,
	}

	w, err := NewMultiWallet(cfg)
	if err != nil {
		t.Fatal(err)
	}

	w.Start()

	select{}
}
