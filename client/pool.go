package client

import (
	"bytes"
	"context"
	"github.com/go-errors/errors"
	"golang.org/x/net/proxy"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
)

// ClientPool is an implementation of the APIClient interface which will handle
// server failure, rotate servers, and retry API requests.
type ClientPool struct {
	*InsightClient
	urls []string
	activeServer int
	proxyDialer  proxy.Dialer
	blockChan chan Block
	txChan chan Transaction
	httpClient http.Client
	cancelFunc context.CancelFunc
	client *http.Client
}

// NewClientPool instantiates a new ClientPool object with the given server APIs
func NewClientPool(urls []string, proxyDialer proxy.Dialer) (*ClientPool, error) {
	for _, apiUrl := range urls {
		u, err := url.Parse(apiUrl)
		if err != nil {
			return nil, err
		}

		if err := validateScheme(u); err != nil {
			return nil, err
		}
	}
	return &ClientPool{
		urls: urls,
		blockChan: make(chan Block),
		txChan: make(chan Transaction),
		proxyDialer: proxyDialer,
	}, nil
}

// Start will attempt to connect to the first server URl. If it fails to
// connect it will rotate through the servers to try to find one that works.
func (p *ClientPool) Start() error {
	for _, url := range p.urls {
		client, err := NewInsightClient(url, p.proxyDialer)
		if err != nil {
			Log.Error(err)
			continue
		}
		p.InsightClient = client
		p.InsightClient.requestFunc = p.doRequest
		ctx, cancel := context.WithCancel(context.Background())
		p.cancelFunc = cancel
		if p.client != nil {
			p.httpClient = *p.client
		}
		go p.listenChans(ctx)
		go p.connectWebsockets()
		return nil
	}
	return errors.New("all insight servers failed to start")
}

// connectWebsockets attempts to connect to the server's socketio websocket
// endpoint. If that fails it will rotate the server and try a new one.
func (p *ClientPool) connectWebsockets() {
	err := p.setupListeners(p.apiUrl, p.proxyDialer)
	if err != nil {
		p.rotateServer()
	}
}

// Stop will disconnect from the socket client
func (p *ClientPool) Stop() {
	p.cancelFunc()
	if p.socketClient != nil {
		p.socketClient.Close()
	}
}

// rotateServer sets the active client to the next provided API URL. Because the new
// InsightClient instantiates new channels we have to call listenChans again so we
// can proxy the new channels through this object.
func (p *ClientPool) rotateServer() {
	p.cancelFunc()
	i := (p.activeServer + 1) % len(p.urls)
	client, err := NewInsightClient(p.urls[i], p.proxyDialer)
	if err != nil {
		Log.Error(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancelFunc = cancel
	p.InsightClient = client
	p.InsightClient.requestFunc = p.doRequest
	if p.client != nil {
		p.httpClient = *p.client
	}
	go p.listenChans(ctx)
	go p.connectWebsockets()
}

// doRequest handles making the HTTP request with server rotation and retires. Only if all servers return an
// error will this method return an error.
func (p *ClientPool) doRequest(endpoint, method string, body []byte, query url.Values) (*http.Response, error) {
	for i:=0; i<len(p.urls); i++ {
		requestUrl := p.apiUrl
		requestUrl.Path = path.Join(p.apiUrl.Path, endpoint)
		req, err := http.NewRequest(method, requestUrl.String(), bytes.NewReader(body))
		if query != nil {
			req.URL.RawQuery = query.Encode()
		}
		if err != nil {
			p.rotateServer()
			continue
		}
		req.Header.Add("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			p.rotateServer()
			continue
		}
		// Try again if for some reason it returned a bad request
		if resp.StatusCode == http.StatusBadRequest {
			// Reset the body so we can read it again.
			req.Body = ioutil.NopCloser(bytes.NewReader(body))
			resp, err = p.httpClient.Do(req)
			if err != nil {
				p.rotateServer()
				continue
			}
		}
		if resp.StatusCode != http.StatusOK {
			p.rotateServer()
			continue
		}
		return resp, nil
	}
	return nil, errors.New("all insight servers return invalid response")
}

// listenChans proxies the block and tx chans from the InsightClient to the ClientPool's channels
func (p *ClientPool) listenChans(ctx context.Context) {
out:
	for {
		select {
		case block := <- p.blockNotifyChan:
			p.blockChan <- block
		case tx := <- p.txNotifyChan:
			p.txChan <- tx
		case <-ctx.Done():
			break out
		}
	}
}

// BlockNofity proxies the active InsightClient's block channel
func (p *ClientPool) BlockNotify() <-chan Block {
	return p.blockChan
}

// TransactionNotify proxies the active InsightClient's tx channel
func (p *ClientPool) TransactionNotify() <-chan Transaction {
	return p.txChan
}