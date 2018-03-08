package main

import (
	"github.com/OpenBazaar/multiwallet"
	"github.com/OpenBazaar/multiwallet/config"
	"github.com/OpenBazaar/wallet-interface"
	"fmt"
	"sync"
	"github.com/btcsuite/btcd/chaincfg"
)

func main() {
	m := make(map[wallet.CoinType]bool)
	m[wallet.Bitcoin] = true
	cfg := config.NewDefaultConfig(m, &chaincfg.TestNet3Params)
	cfg.Mnemonic = "design author ability expose illegal saddle antique setup pledge wife innocent treat"
	w, err := multiwallet.NewMultiWallet(cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	var wg sync.WaitGroup
	wg.Add(1)
	w.Start()
	wg.Wait()
}