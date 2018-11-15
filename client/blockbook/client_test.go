package blockbook

import (
	"encoding/json"
	"fmt"
	laddr "github.com/OpenBazaar/multiwallet/litecoin/address"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"testing"
)

func TestBlockBookClient_BlockNotify(t *testing.T) {
	client, err := NewBlockBookClient("https://ltc.blockbook.api.openbazaar.org", nil)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := laddr.DecodeAddress("LMadJwqLskwZLcpPxAFbWRKyKaTtz8YB3E", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := client.GetUtxos([]btcutil.Address{addr})
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.MarshalIndent(tx, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(out))
}