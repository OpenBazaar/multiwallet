package main

import (
	"flag"
	wi "github.com/OpenBazaar/wallet-interface"
	"log"
	"net/http"
	"strconv"
)

func main() {
	portPtr := flag.Int("port", 8080, "server port")

	insight := NewMockInsightServer(wi.Bitcoin)
	serveMux := http.NewServeMux()
	serveMux.Handle("/socket.io/", insight.socketServer)
	serveMux.HandleFunc("/blocks", insight.handleGetBestBlock)
	serveMux.HandleFunc("/generate", insight.handleGenerate)
	serveMux.HandleFunc("/tx/send", insight.handleBroadcast)
	serveMux.HandleFunc("/tx/", insight.handleGetTransaction)
	serveMux.HandleFunc("/addrs/txs/", insight.handleGetTransactions)
	serveMux.HandleFunc("/addrs/utxos/", insight.handleGetUtxos)
	serveMux.HandleFunc("/generatetoaddress", insight.handleGenerateToAddress)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*portPtr), serveMux))
}
