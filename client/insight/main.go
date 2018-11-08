package main

import (
	wi "github.com/OpenBazaar/wallet-interface"
	"log"
	"net/http"
)

func main() {
	insight := NewMockInsightServer(wi.Bitcoin)
	serveMux := http.NewServeMux()
	serveMux.Handle("/socket.io/", insight.socketServer)
	serveMux.HandleFunc("/blocks", insight.handleGetBestBlock)
	serveMux.HandleFunc("/generate", insight.handleGenerate)
	serveMux.HandleFunc("/tx/send", insight.handleGetTransaction)
	serveMux.HandleFunc("/tx/", insight.handleGetTransaction)
	serveMux.HandleFunc("/generatetoaddress", insight.handleGenerateToAddress)
	log.Fatal(http.ListenAndServe(":8080", serveMux))
}
