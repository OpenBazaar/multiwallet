package client_test

import (
	"fmt"
	"github.com/OpenBazaar/multiwallet/client"
	"github.com/OpenBazaar/multiwallet/model"
	"github.com/OpenBazaar/multiwallet/model/mock"
	"github.com/OpenBazaar/multiwallet/test"
	"github.com/OpenBazaar/multiwallet/test/factory"
	"gopkg.in/jarcoal/httpmock.v1"
	"net/http"
	"testing"
	"time"
)

func TestServerRotation(t *testing.T) {
	var (
		endpointOne = "http://localhost:8332"
		endpointTwo = "http://localhost:8336"
		p, err      = client.NewClientPool([]string{endpointOne, endpointTwo}, nil)
		txid        = "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428"
		testPath    = func(host string) string { return fmt.Sprintf("%s/tx/%s", host, txid) }
		expectedTx  = factory.NewTransaction()
	)
	if err != nil {
		t.Fatal(err)
	}

	mockedHTTPClient := http.Client{}
	httpmock.ActivateNonDefault(&mockedHTTPClient)
	defer httpmock.DeactivateAndReset()
	for _, c := range p.Clients() {
		c.HTTPClient = mockedHTTPClient
	}
	p.HTTPClient = mockedHTTPClient

	mock.MockWebsocketClientOnClientPool(p)

	httpmock.RegisterResponder(http.MethodGet, testPath(endpointOne),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponse(http.StatusOK, expectedTx)
		},
	)

	err = p.Start()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.GetTransaction(txid)
	if err != nil {
		t.Fatal(err)
	}
	test.ValidateTransaction(*tx, expectedTx, t)

	// Test invalid response, server rotation, then valid response from second server
	httpmock.Reset()
	httpmock.RegisterResponder(http.MethodGet, testPath(endpointOne),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponse(http.StatusInternalServerError, expectedTx)
		},
	)

	httpmock.RegisterResponder(http.MethodGet, testPath(endpointTwo),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponse(http.StatusOK, expectedTx)
		},
	)

	tx, err = p.GetTransaction(txid)
	if err != nil {
		t.Fatal(err)
	}
	test.ValidateTransaction(*tx, expectedTx, t)
}

func TestClientPool_BlockNotify(t *testing.T) {
	var (
		endpointOne = "http://localhost:8332"
		endpointTwo = "http://localhost:8336"
		p, err      = client.NewClientPool([]string{endpointOne, endpointTwo}, nil)
		testHash    = "0000000000000000003f1fb88ac3dab0e607e87def0e9031f7bea02cb464a04f"
		txid        = "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428"
		testPath    = func(host string) string { return fmt.Sprintf("%s/tx/%s", host, txid) }
		expectedTx  = factory.NewTransaction()
	)
	if err != nil {
		t.Fatal(err)
	}

	mock.MockWebsocketClientOnClientPool(p)
	mockedHTTPClient := http.Client{}
	httpmock.ActivateNonDefault(&mockedHTTPClient)
	defer httpmock.DeactivateAndReset()
	for _, c := range p.Clients() {
		c.HTTPClient = mockedHTTPClient
	}
	p.HTTPClient = mockedHTTPClient

	mock.MockWebsocketClientOnClientPool(p)

	err = p.Start()
	if err != nil {
		t.Fatal(err)
	}

	client := p.CurrentClient()
	var p1, p2 string
	if client.EndpointURL().Host == "localhost:8332" {
		p1 = endpointOne
		p2 = endpointTwo
	} else {
		p1 = endpointTwo
		p2 = endpointOne
	}

	// GetTransaction should fail for endpoint one and succeed for endpoint two
	httpmock.RegisterResponder(http.MethodGet, testPath(p1),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponse(http.StatusBadRequest, nil)
		},
	)

	httpmock.RegisterResponder(http.MethodGet, testPath(p2),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponse(http.StatusOK, expectedTx)
		},
	)

	go func() {
		client.BlockChannel() <- model.Block{Hash: testHash}
	}()

	ticker := time.NewTicker(time.Second * 2)
	select {
	case <-ticker.C:
		t.Error("Timed out waiting for block")
	case b := <-p.BlockNotify():
		if b.Hash != testHash {
			t.Error("Returned incorrect block hash")
		}
	}

	p.GetTransaction(txid)

	client = p.CurrentClient()

	go func() {
		client.BlockChannel() <- model.Block{Hash: testHash}
	}()

	ticker = time.NewTicker(time.Second * 2)
	select {
	case <-ticker.C:
		t.Error("Timed out waiting for block")
	case b := <-p.BlockNotify():
		if b.Hash != testHash {
			t.Error("Returned incorrect block hash")
		}
	}
}

func TestClientPool_TransactionNotify(t *testing.T) {
	var (
		endpointOne = "http://localhost:8332"
		endpointTwo = "http://localhost:8336"
		p, err      = client.NewClientPool([]string{endpointOne, endpointTwo}, nil)
		txid        = "1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428"
		testPath    = func(host string) string { return fmt.Sprintf("%s/tx/%s", host, txid) }
		expectedTx  = factory.NewTransaction()
	)
	if err != nil {
		t.Fatal(err)
	}

	mock.MockWebsocketClientOnClientPool(p)
	mockedHTTPClient := http.Client{}
	httpmock.ActivateNonDefault(&mockedHTTPClient)
	defer httpmock.DeactivateAndReset()
	for _, c := range p.Clients() {
		c.HTTPClient = mockedHTTPClient
	}
	p.HTTPClient = mockedHTTPClient

	mock.MockWebsocketClientOnClientPool(p)

	err = p.Start()
	if err != nil {
		t.Fatal(err)
	}

	client := p.CurrentClient()
	var p1, p2 string
	if client.EndpointURL().Host == "localhost:8332" {
		p1 = endpointOne
		p2 = endpointTwo
	} else {
		p1 = endpointTwo
		p2 = endpointOne
	}

	// GetTransaction should fail for endpoint one and succeed for endpoint two
	httpmock.RegisterResponder(http.MethodGet, testPath(p1),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponse(http.StatusBadRequest, nil)
		},
	)

	httpmock.RegisterResponder(http.MethodGet, testPath(p2),
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponse(http.StatusOK, expectedTx)
		},
	)

	go func() {
		client.TxChannel() <- expectedTx
	}()

	ticker := time.NewTicker(time.Second * 2)
	select {
	case <-ticker.C:
		t.Error("Timed out waiting for tx")
	case b := <-p.TransactionNotify():
		if b.Txid != expectedTx.Txid {
			t.Error("Returned incorrect tx hash")
		}
	}

	p.GetTransaction(txid)

	client = p.CurrentClient()

	go func() {
		client.TxChannel() <- expectedTx
	}()

	ticker = time.NewTicker(time.Second * 2)
	select {
	case <-ticker.C:
		t.Error("Timed out waiting for tx")
	case b := <-p.TransactionNotify():
		if b.Txid != expectedTx.Txid {
			t.Error("Returned incorrect tx hash")
		}
	}
}
