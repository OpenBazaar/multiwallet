package client

import "github.com/btcsuite/btcutil"

type APIClient interface {

	// For a given txid get back the full transaction
	GetTransaction(txid string) (*Transaction, error)

	// Get back all the transactions for the given list of addresses
	GetTransactions(addrs []btcutil.Address) ([]Transaction, error)

	// Get back all spendable UTXOs for the given list of addresses
	GetUtxos(addrs []btcutil.Address) ([]Utxo, error)

	// Returns a chan which fires on each new block
	BlockNotify() <-chan Block

	// Returns a chan which fires whenever a new transaction is received or
	// when an existing transaction confirms for all addresses the API is listening on.
	TransactionNotify() <-chan Transaction

	// Listen for events on this addresses. Results are returned to TransactionNotify()
	ListenAddress(addr btcutil.Address)

	// Broadcast a transaction to the network
	Broadcast(tx []byte) (string, error)

	// Get info on the current chain tip
	GetBestBlock() (*Block, error)
}
