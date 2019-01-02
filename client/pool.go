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

	"github.com/OpenBazaar/multiwallet/client/blockbook"
	"github.com/OpenBazaar/multiwallet/model"
	"github.com/btcsuite/btcutil"
	logging "github.com/op/go-logging"
	"golang.org/x/net/proxy"
)

var Log = logging.MustGetLogger("pool")

// ClientPool is an implementation of the APIClient interface which will handle
// server failure, rotate servers, and retry API requests.
type ClientPool struct {
	blockChan        chan model.Block
	cancelListenChan context.CancelFunc
	clientEndpoints  []string
	proxyDialer      proxy.Dialer
	txChan           chan model.Transaction
	poolManager      *rotationManager

	HTTPClient  http.Client
	ClientCache []*blockbook.BlockBookClient
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

// NewClientPool instantiates a new ClientPool object with the given server APIs
func NewClientPool(endpoints []string, proxyDialer proxy.Dialer) (*ClientPool, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("no client endpoints provided")
	}

	var (
		clientCache = make([]*blockbook.BlockBookClient, len(endpoints))
		pool        = &ClientPool{
			blockChan:       make(chan model.Block),
			ClientCache:     clientCache,
			clientEndpoints: endpoints,
			txChan:          make(chan model.Transaction),
			poolManager:     &rotationManager{},
		}
		manager, err = newRotationManager(endpoints, proxyDialer, pool.doRequest)
	)
	if err != nil {
		return nil, err
	}
	pool.poolManager = manager
	return pool, nil
}

// Start will attempt to connect to the first insight server. If it fails to
// connect it will rotate through the servers to try to find one that works.
func (p *ClientPool) Start() error {
	for e := p.newMaximumTryEnumerator(); e.next(); {
		p.poolManager.SelectNext()
		if err := p.poolManager.StartCurrent(); err != nil {
			Log.Errorf("failed start: %s", err)
			p.poolManager.FailCurrent()
			continue
		}
		return nil
	}
	Log.Errorf("all servers failed to start")
	return errors.New("all insight servers failed to start")
}

// Close proxies the same request to the active InsightClient
func (p *ClientPool) Close() {
	p.poolManager.CloseCurrent()
}

// FailRotateAndStartNextClient cleans up the active client's connections, and
// attempts to start the next available client's connection.
func (p *ClientPool) FailRotateAndStartNextClient() error {
	if p.cancelListenChan != nil {
		p.cancelListenChan()
		p.cancelListenChan = nil
	}
	p.poolManager.FailCurrent()
	p.poolManager.CloseCurrent()
	p.poolManager.SelectNext()

	for e := p.newMaximumTryEnumerator(); e.next(); {
		startErr := p.poolManager.StartCurrent()
		if startErr == nil {
			var ctx context.Context
			ctx, p.cancelListenChan = context.WithCancel(context.Background())
			go p.listenChans(ctx)
			return nil
		}
		Log.Errorf("error starting %s: %s", p.poolManager.currentTarget, startErr.Error())
		p.poolManager.FailCurrent()
		p.poolManager.SelectNext()
		continue
	}
	return fmt.Errorf("unable to find an available server")
}

// listenChans proxies the block and tx chans from the InsightClient to the ClientPool's channels
func (p *ClientPool) listenChans(ctx context.Context) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	for {
		select {
		case block := <-client.BlockChannel():
			p.blockChan <- block
		case tx := <-client.TxChannel():
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
		var client = p.poolManager.AcquireCurrent()
		requestUrl := client.EndpointURL()
		requestUrl.Path = path.Join(client.EndpointURL().Path, endpoint)
		req, err := http.NewRequest(method, requestUrl.String(), bytes.NewReader(body))
		if query != nil {
			req.URL.RawQuery = query.Encode()
		}
		if err != nil {
			Log.Errorf("error preparing request (%s %s)", method, requestUrl.String())
			Log.Errorf("\terror continued: %s", err.Error())
			p.poolManager.ReleaseCurrent()
			return nil, fmt.Errorf("invalid request: %s", err)
		}
		req.Header.Add("Content-Type", "application/json")

		resp, err := p.HTTPClient.Do(req)
		if err != nil {
			Log.Errorf("error making request (%s %s)", method, requestUrl.String())
			Log.Errorf("\terror continued: %s", err.Error())
			p.poolManager.ReleaseCurrent()
			if err := p.FailRotateAndStartNextClient(); err != nil {
				return nil, err
			}
			continue
		}
		// Try again if for some reason it returned a bad request
		if resp.StatusCode == http.StatusBadRequest {
			// Reset the body so we can read it again.
			req.Body = ioutil.NopCloser(bytes.NewReader(body))
			resp, err = p.HTTPClient.Do(req)
			if err != nil {
				Log.Errorf("error making request (%s %s)", method, requestUrl.String())
				Log.Errorf("\terror continued: %s", err.Error())
				p.poolManager.ReleaseCurrent()
				if err := p.FailRotateAndStartNextClient(); err != nil {
					return nil, err
				}
				continue
			}
		}
		if resp.StatusCode != http.StatusOK {
			p.poolManager.ReleaseCurrent()
			if err := p.FailRotateAndStartNextClient(); err != nil {
				return nil, err
			}
			continue
		}
		p.poolManager.ReleaseCurrent()
		return resp, nil
	}
	return nil, errors.New("exhausted maximum attempts for request")
}

// BlockNofity proxies the active InsightClient's block channel
func (p *ClientPool) BlockNotify() <-chan model.Block {
	return p.blockChan
}

// Broadcast proxies the same request to the active InsightClient
func (p *ClientPool) Broadcast(tx []byte) (string, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.Broadcast(tx)
}

// EstimateFee proxies the same request to the active InsightClient
func (p *ClientPool) EstimateFee(nBlocks int) (int, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.EstimateFee(nBlocks)
}

// GetBestBlock proxies the same request to the active InsightClient
func (p *ClientPool) GetBestBlock() (*model.Block, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.GetBestBlock()
}

// GetInfo proxies the same request to the active InsightClient
func (p *ClientPool) GetInfo() (*model.Info, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.GetInfo()
}

// GetRawTransaction proxies the same request to the active InsightClient
func (p *ClientPool) GetRawTransaction(txid string) ([]byte, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.GetRawTransaction(txid)
}

// GetTransactions proxies the same request to the active InsightClient
func (p *ClientPool) GetTransactions(addrs []btcutil.Address) ([]model.Transaction, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.GetTransactions(addrs)
}

// GetTransaction proxies the same request to the active InsightClient
func (p *ClientPool) GetTransaction(txid string) (*model.Transaction, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.GetTransaction(txid)
}

// GetUtxos proxies the same request to the active InsightClient
func (p *ClientPool) GetUtxos(addrs []btcutil.Address) ([]model.Utxo, error) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	return client.GetUtxos(addrs)
}

// ListenAddress proxies the same request to the active InsightClient
func (p *ClientPool) ListenAddress(addr btcutil.Address) {
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	client.ListenAddress(addr)
}

// TransactionNotify proxies the active InsightClient's tx channel
func (p *ClientPool) TransactionNotify() <-chan model.Transaction { return p.txChan }
