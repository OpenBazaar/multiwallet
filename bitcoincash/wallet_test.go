package bitcoincash

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/OpenBazaar/multiwallet/datastore"
	"github.com/OpenBazaar/wallet-interface"
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

func TestBitcoinCashWallet_SpendFailsWhenTooLarge(t *testing.T) {
	ds := datastore.NewMockMultiwalletDatastore()
	db, err := ds.GetDatastoreForWallet(wallet.BitcoinCash)
	if err != nil {
		t.Fatal(err)
	}

	w := BitcoinCashWallet{
		db: db,
	}

	overflowedInt := new(big.Int).Add(big.NewInt(math.MaxInt64), big.NewInt(1))
	_, err = w.Spend(*overflowedInt, nil, wallet.ECONOMIC, "", false)
	if err == nil {
		t.Fatalf("expected overflowed amount to return an error, but did not")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("(%s) is too large", overflowedInt.String())) {
		t.Errorf("expected error to contain (is too large), but was (%s)", err.Error())
	}
}
