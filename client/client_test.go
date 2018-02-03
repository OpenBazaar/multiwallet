package client

import (
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"testing"
)

// TODO: this isn't a real test
func TestInsightClient_GetInfo(t *testing.T) {
	client, err := NewInsightClient("https://cashexplorer.bitcoin.com/api", nil)
	if err != nil {
		t.Error(err)
		return
	}
	addr1, _ := btcutil.DecodeAddress("1KRaKmNSoVe4gKaWoxiF9gsXuGr4tky1LB", &chaincfg.MainNetParams)

	tl, err := client.GetTransactions([]btcutil.Address{addr1})
	if err != nil {
		t.Error(err)
		return
	}
	s, _ := json.MarshalIndent(&tl, "", "    ")
	fmt.Println(string(s))

	stat, err := client.GetInfo()
	if err != nil {
		t.Error(err)
		return
	}
	s, _ = json.MarshalIndent(stat, "", "    ")
	fmt.Println(string(s))
}
