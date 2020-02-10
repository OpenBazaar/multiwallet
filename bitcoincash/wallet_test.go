package bitcoincash

import (
	"github.com/OpenBazaar/multiwallet/datastore"
	"github.com/OpenBazaar/wallet-interface"
	"math"
	"math/big"
	"testing"
)

func TestBitcoinCashWallet_IsDust(t *testing.T) {
	ds := datastore.NewMockMultiwalletDatastore()
	db, err := ds.GetDatastoreForWallet(wallet.BitcoinCash)
	if err != nil {
		t.Fatal(err)
	}

	w := BitcoinCashWallet{
		db: db,
	}

	if !w.IsDust(*big.NewInt(0)) {
		t.Error("Zero amount did not return dust")
	}

	if w.IsDust(*new(big.Int).SetUint64(math.MaxInt64 + 1)) {
		t.Error("> max int64 returned false")
	}
}
