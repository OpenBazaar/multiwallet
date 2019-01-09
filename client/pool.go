package client

import (
	"context"
	"errors"
	"github.com/OpenBazaar/multiwallet/client/blockbook"
	"github.com/OpenBazaar/multiwallet/model"
	"github.com/btcsuite/btcutil"
	"github.com/op/go-logging"
	"golang.org/x/net/proxy"
	"net/http"
	"sync"
)

var Log = logging.MustGetLogger("pool")

// ClientPool is an implementation of the APIClient interface which will handle
// server failure, rotate servers, and retry API requests.
type ClientPool struct {
	blockChan        chan model.Block
	cancelListenChan context.CancelFunc
	listenAddrs      []btcutil.Address
	listenAddrsLock  sync.Mutex
	poolManager      *rotationManager
	proxyDialer      proxy.Dialer
	txChan           chan model.Transaction
	unblockStart     chan struct{}

	HTTPClient  http.Client
	ClientCache []*blockbook.BlockBookClient
}

func (p *ClientPool) newMaximumTryEnumerator() *maxTryEnum {
	return &maxTryEnum{max: 3, attempts: 0}
}

type maxTryEnum struct{ max, attempts int }

func (m *maxTryEnum) next() bool {
	var now = m.attempts
	m.attempts++
	return now < m.max
}

// NewClientPool instantiates a new ClientPool object with the given server APIs
func NewClientPool(endpoints []string, proxyDialer proxy.Dialer) (*ClientPool, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("no client endpoints provided")
	}

	var (
		clientCache = make([]*blockbook.BlockBookClient, len(endpoints))
		pool        = &ClientPool{
			blockChan:    make(chan model.Block),
			poolManager:  &rotationManager{},
			listenAddrs:  make([]btcutil.Address, 0),
			txChan:       make(chan model.Transaction),
			unblockStart: make(chan struct{}, 1),
			ClientCache:  clientCache,
		}
		manager, err = newRotationManager(endpoints, proxyDialer)
	)
	if err != nil {
		return nil, err
	}
	pool.poolManager = manager
	return pool, nil
}

// Start will attempt to connect to the first available server. If it fails to
// connect it will rotate through the servers to try to find one that works.
func (p *ClientPool) Start() error {
	go p.run()
	return nil
}

func (p *ClientPool) run() {
	for {
		select {
		case <-p.unblockStart:
			return
		default:
			p.runLoop()
		}
	}
}

func (p *ClientPool) runLoop() error {
	p.poolManager.SelectNext()
	var closeChan = make(chan error, 0)
	if err := p.poolManager.StartCurrent(closeChan); err != nil {
		Log.Errorf("error starting %s: %s", p.poolManager.currentTarget, err.Error())
		p.poolManager.FailCurrent()
		p.poolManager.CloseCurrent()
		return err
	}
	var ctx context.Context
	ctx, p.cancelListenChan = context.WithCancel(context.Background())
	go p.listenChans(ctx)
	p.replayListenAddresses()
	if err := <-closeChan; err != nil {
		p.poolManager.FailCurrent()
		p.poolManager.CloseCurrent()
	}
	return nil
}

// Close proxies the same request to the active InsightClient
func (p *ClientPool) Close() {
	if p.cancelListenChan != nil {
		p.cancelListenChan()
		p.cancelListenChan = nil
	}
	p.unblockStart <- struct{}{}
	p.poolManager.CloseCurrent()
}

// FailAndCloseCurrentClient cleans up the active client's connections, and
// signals to the rotation manager that it is unhealthy. The internal runLoop
// will detect the client's closing and attempt to start the next available.
func (p *ClientPool) FailAndCloseCurrentClient() {
	if p.cancelListenChan != nil {
		p.cancelListenChan()
		p.cancelListenChan = nil
	}
	p.poolManager.FailCurrent()
	p.poolManager.CloseCurrent()
}

// listenChans proxies the block and tx chans from the InsightClient to the ClientPool's channels
func (p *ClientPool) listenChans(ctx context.Context) {
	var (
		client    = p.poolManager.AcquireCurrent()
		blockChan = client.BlockChannel()
		txChan    = client.TxChannel()
	)
	defer p.poolManager.ReleaseCurrent()
	go func() {
		for {
			select {
			case block := <-blockChan:
				p.blockChan <- block
			case tx := <-txChan:
				p.txChan <- tx
			case <-ctx.Done():
				return
			}
		}
	}()
}

// executeRequest handles making the HTTP request with server rotation and retires. Only if all servers return an
// error will this method return an error.
func (p *ClientPool) executeRequest(client *blockbook.BlockBookClient, queryFunc func(c *blockbook.BlockBookClient) error) error {
	for e := p.newMaximumTryEnumerator(); e.next(); {
		if err := queryFunc(client); err != nil {
			p.poolManager.ReleaseCurrent()
			p.FailAndCloseCurrentClient()
			client = p.poolManager.AcquireCurrent()
		} else {
			return nil
		}
	}
	return errors.New("exhausted maximum attempts for request")
}

// BlockNofity proxies the active InsightClient's block channel
func (p *ClientPool) BlockNotify() <-chan model.Block {
	return p.blockChan
}

// Broadcast proxies the same request to the active InsightClient
func (p *ClientPool) Broadcast(tx []byte) (string, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var txid string
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		txid, err = c.Broadcast(tx)
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return txid, err
}

// EstimateFee proxies the same request to the active InsightClient
func (p *ClientPool) EstimateFee(nBlocks int) (int, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var fee int
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		fee, err = c.EstimateFee(nBlocks)
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return fee, err
}

// GetBestBlock proxies the same request to the active InsightClient
func (p *ClientPool) GetBestBlock() (*model.Block, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var block *model.Block
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		block, err = c.GetBestBlock()
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return block, err
}

// GetInfo proxies the same request to the active InsightClient
func (p *ClientPool) GetInfo() (*model.Info, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var info *model.Info
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		info, err = c.GetInfo()
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return info, err
}

// GetRawTransaction proxies the same request to the active InsightClient
func (p *ClientPool) GetRawTransaction(txid string) ([]byte, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var tx []byte
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		tx, err = c.GetRawTransaction(txid)
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return tx, err
}

// GetTransactions proxies the same request to the active InsightClient
func (p *ClientPool) GetTransactions(addrs []btcutil.Address) ([]model.Transaction, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var txs []model.Transaction
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		txs, err = c.GetTransactions(addrs)
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return txs, err
}

// GetTransaction proxies the same request to the active InsightClient
func (p *ClientPool) GetTransaction(txid string) (*model.Transaction, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var tx *model.Transaction
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		tx, err = c.GetTransaction(txid)
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return tx, err
}

// GetUtxos proxies the same request to the active InsightClient
func (p *ClientPool) GetUtxos(addrs []btcutil.Address) ([]model.Utxo, error) {
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	var utxos []model.Utxo
	var err error
	queryFunc := func(c *blockbook.BlockBookClient) error {
		utxos, err = c.GetUtxos(addrs)
		return err
	}
	err = p.executeRequest(client, queryFunc)
	return utxos, err
}

// ListenAddress proxies the same request to the active InsightClient
func (p *ClientPool) ListenAddress(addr btcutil.Address) {
	p.listenAddrsLock.Lock()
	defer p.listenAddrsLock.Unlock()
	var client = p.poolManager.AcquireCurrentWhenReady()
	defer p.poolManager.ReleaseCurrent()
	p.listenAddrs = append(p.listenAddrs, addr)
	client.ListenAddress(addr)
}

func (p *ClientPool) replayListenAddresses() {
	p.listenAddrsLock.Lock()
	defer p.listenAddrsLock.Unlock()
	var client = p.poolManager.AcquireCurrent()
	defer p.poolManager.ReleaseCurrent()
	for _, addr := range p.listenAddrs {
		client.ListenAddress(addr)
	}
}

// TransactionNotify proxies the active InsightClient's tx channel
func (p *ClientPool) TransactionNotify() <-chan model.Transaction { return p.txChan }
