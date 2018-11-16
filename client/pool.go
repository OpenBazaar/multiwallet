package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"sync"

	"github.com/btcsuite/btcutil"
	"golang.org/x/net/proxy"
)

// ClientPool is an implementation of the APIClient interface which will handle
// server failure, rotate servers, and retry API requests.
type ClientPool struct {
	clientEndpoints  []string
	clientCache      []*InsightClient
	activeServer     int
	proxyDialer      proxy.Dialer
	blockChan        chan Block
	txChan           chan Transaction
	httpClient       http.Client
	cancelListenChan context.CancelFunc
	rotationMutex    sync.RWMutex
}

func (p *ClientPool) newMaximumTryEnumerator() *maxTryEnum {
	return &maxTryEnum{max: len(p.clientEndpoints), attempts: 0}
}

type maxTryEnum struct{ max, attempts int }

func (m *maxTryEnum) next() bool {
	var now = m.attempts
	m.attempts++
	return now <= m.max
}

func (p *ClientPool) currentClient() *InsightClient {
	p.rotationMutex.RLock()
	defer p.rotationMutex.RUnlock()
	return p.clientCache[p.activeServer]
}

// NewClientPool instantiates a new ClientPool object with the given server APIs
func NewClientPool(endpoints []string, proxyDialer proxy.Dialer) (*ClientPool, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("no client endpoints provided")
	}
	var (
		clientCache = make([]*InsightClient, len(endpoints))
		pool        = &ClientPool{
			blockChan:       make(chan Block),
			clientCache:     clientCache,
			clientEndpoints: endpoints,
			proxyDialer:     proxyDialer,
			txChan:          make(chan Transaction),
		}
	)
	for i, apiUrl := range endpoints {
		c, err := NewInsightClient(apiUrl, proxyDialer)
		if err != nil {
			return nil, err
		}
		c.requestFunc = pool.doRequest
		pool.clientCache[i] = c
	}
	return pool, nil
}

// Start will attempt to connect to the first insight server. If it fails to
// connect it will rotate through the servers to try to find one that works.
func (p *ClientPool) Start() error {
	for e := p.newMaximumTryEnumerator(); e.next(); {
		if err := p.rotateAndStartNextClient(); err != nil {
			Log.Errorf("failed start: %s", err)
			continue
		}
		return nil
	}
	Log.Errorf("all servers failed to start")
	return errors.New("all insight servers failed to start")
}

// rotateAndStartNextClient cleans up the active client's connections, and attempts to start the
// next client's connection. If an error is returned, it can be assumed that new
// client could not start and rotateAndStartNextClient needs to be retried. The caller of this
// method should track the retry attempts so as to not repeat indefinitely.
func (p *ClientPool) rotateAndStartNextClient() error {
	// Signal rotation and wait for connections to drain
	p.rotationMutex.Lock()
	defer p.rotationMutex.Unlock()

	if p.cancelListenChan != nil {
		p.cancelListenChan()
		p.cancelListenChan = nil
	}
	p.clientCache[p.activeServer].Close()
	p.activeServer = (p.activeServer + 1) % len(p.clientCache)
	nextClient := p.clientCache[p.activeServer]

	Log.Infof("starting server %s...", p.clientEndpoints[p.activeServer])
	// Should be first connection signal, ensure rotation isn't triggered elsewhere
	if err := nextClient.Start(); err != nil {
		nextClient.Close()
		return err
	}
	var ctx context.Context
	ctx, p.cancelListenChan = context.WithCancel(context.Background())
	go p.listenChans(ctx)
	return nil
}

// listenChans proxies the block and tx chans from the InsightClient to the ClientPool's channels
func (p *ClientPool) listenChans(ctx context.Context) {
	for {
		select {
		case block := <-p.currentClient().blockNotifyChan:
			p.blockChan <- block
		case tx := <-p.currentClient().txNotifyChan:
			p.txChan <- tx
		case <-ctx.Done():
			return
		}
	}
}

// doRequest handles making the HTTP request with server rotation and retires. Only if all servers return an
// error will this method return an error.
func (p *ClientPool) doRequest(endpoint, method string, body []byte, query url.Values) (*http.Response, error) {
	for e := p.newMaximumTryEnumerator(); e.next(); {
		p.rotationMutex.RLock()
		requestUrl := p.currentClient().apiUrl
		requestUrl.Path = path.Join(p.currentClient().apiUrl.Path, endpoint)
		req, err := http.NewRequest(method, requestUrl.String(), bytes.NewReader(body))
		if query != nil {
			req.URL.RawQuery = query.Encode()
		}
		if err != nil {
			p.rotationMutex.RUnlock()
			return nil, fmt.Errorf("invalid request: %s", err)
		}
		req.Header.Add("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			p.rotationMutex.RUnlock()
			p.rotateAndStartNextClient()
			continue
		}
		// Try again if for some reason it returned a bad request
		if resp.StatusCode == http.StatusBadRequest {
			// Reset the body so we can read it again.
			req.Body = ioutil.NopCloser(bytes.NewReader(body))
			resp, err = p.httpClient.Do(req)
			if err != nil {
				p.rotationMutex.RUnlock()
				p.rotateAndStartNextClient()
				continue
			}
		}
		if resp.StatusCode != http.StatusOK {
			p.rotationMutex.RUnlock()
			p.rotateAndStartNextClient()
			continue
		}
		p.rotationMutex.RUnlock()
		return resp, nil
	}
	return nil, errors.New("all insight servers return invalid response")
}

// BlockNofity proxies the active InsightClient's block channel
func (p *ClientPool) BlockNotify() <-chan Block {
	return p.blockChan
}

// Broadcast proxies the same request to the active InsightClient
func (p *ClientPool) Broadcast(tx []byte) (string, error) {
	return p.currentClient().Broadcast(tx)
}

// Close proxies the same request to the active InsightClient
func (p *ClientPool) Close() {
	p.currentClient().Close()
}

// EstimateFee proxies the same request to the active InsightClient
func (p *ClientPool) EstimateFee(nBlocks int) (int, error) {
	return p.currentClient().EstimateFee(nBlocks)
}

// GetBestBlock proxies the same request to the active InsightClient
func (p *ClientPool) GetBestBlock() (*Block, error) {
	return p.currentClient().GetBestBlock()
}

// GetInfo proxies the same request to the active InsightClient
func (p *ClientPool) GetInfo() (*Info, error) {
	return p.currentClient().GetInfo()
}

// GetRawTransaction proxies the same request to the active InsightClient
func (p *ClientPool) GetRawTransaction(txid string) ([]byte, error) {
	return p.currentClient().GetRawTransaction(txid)
}

// GetTransactions proxies the same request to the active InsightClient
func (p *ClientPool) GetTransactions(addrs []btcutil.Address) ([]Transaction, error) {
	return p.currentClient().GetTransactions(addrs)
}

// GetTransaction proxies the same request to the active InsightClient
func (p *ClientPool) GetTransaction(txid string) (*Transaction, error) {
	return p.currentClient().GetTransaction(txid)
}

// GetUtxos proxies the same request to the active InsightClient
func (p *ClientPool) GetUtxos(addrs []btcutil.Address) ([]Utxo, error) {
	return p.currentClient().GetUtxos(addrs)
}

// ListenAddress proxies the same request to the active InsightClient
func (p *ClientPool) ListenAddress(addr btcutil.Address) {
	p.currentClient().ListenAddress(addr)
}

// TransactionNotify proxies the active InsightClient's tx channel
func (p *ClientPool) TransactionNotify() <-chan Transaction {
	return p.txChan
}
