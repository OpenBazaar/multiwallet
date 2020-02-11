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

	if w.IsDust(*big.NewInt(0)) {
		t.Error("expected zero to be dust, but was not")
	}

	overflowedInt := *new(big.Int).Add(big.NewInt(math.MaxInt64), big.NewInt(1))
	if overflowedInt.IsInt64() {
		t.Error("expected big.Int to be overflowed, but wasn't")
	}
	if w.IsDust(overflowedInt) {
		t.Error("expected overflowed big.Int to not be dust, but was")
	}
}
