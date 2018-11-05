package client

import (
	"fmt"
	"gopkg.in/jarcoal/httpmock.v1"
	"net/http"
	"testing"
	"time"
)

func NewTestPool() *ClientPool {
	p, _ := NewClientPool([]string{"http://localhost:8332", "http://localhost:8336"}, nil)
	p.client = &http.Client{}
	p.Start()
	return p
}

func TestServerRotation(t *testing.T) {
	setup()
	defer teardown()

	var (
		p          = NewTestPool()
		testPath   = fmt.Sprintf("http://%s/tx/1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428", p.apiUrl.Host)
		testPath2  = fmt.Sprintf("http://%s/tx/1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428", "localhost:8336")
		expectedTx = TestTx
	)

	// Test valid response from first server
	response, err := httpmock.NewJsonResponse(http.StatusOK, expectedTx)
	if err != nil {
		t.Error(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response, nil
		},
	)

	tx, err := p.GetTransaction("1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428")
	if err != nil {
		t.Error(err)
	}
	validateTransaction(*tx, expectedTx, t)

	// Test invalid response, server rotation, then valid response from second server
	response1, err := httpmock.NewJsonResponse(http.StatusInternalServerError, expectedTx)
	if err != nil {
		t.Error(err)
	}

	response2, err := httpmock.NewJsonResponse(http.StatusOK, expectedTx)
	if err != nil {
		t.Error(err)
	}

	httpmock.RegisterResponder(http.MethodGet, testPath,
		func(req *http.Request) (*http.Response, error) {
			return response1, nil
		},
	)

	httpmock.RegisterResponder(http.MethodGet, testPath2,
		func(req *http.Request) (*http.Response, error) {
			return response2, nil
		},
	)

	tx, err = p.GetTransaction("1be612e4f2b79af279e0b307337924072b819b3aca09fcb20370dd9492b83428")
	if err != nil {
		t.Error(err)
	}
	validateTransaction(*tx, expectedTx, t)
}

func TestClientPool_BlockNotify(t *testing.T) {
	var (
		p        = NewTestPool()
		testHash = "0000000000000000003f1fb88ac3dab0e607e87def0e9031f7bea02cb464a04f"
	)
	// Rotate the server to make sure we can still send through the new client connect
	// to the pool chans
	p.rotateServer()

	go func() {
		p.blockNotifyChan <- Block{Hash: testHash}
	}()

	ticker := time.NewTicker(time.Second)
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
	p := NewTestPool()
	// Rotate the server to make sure we can still send through the new client connect
	// to the pool chans
	p.rotateServer()

	go func() {
		p.txNotifyChan <- TestTx
	}()

	ticker := time.NewTicker(time.Second)
	select {
	case <-ticker.C:
		t.Error("Timed out waiting for tx")
	case b := <-p.TransactionNotify():
		for n, in := range b.Inputs {
			f, err := toFloat(in.ValueIface)
			if err != nil {
				t.Error(err)
			}
			b.Inputs[n].Value = f
		}
		for n, out := range b.Outputs {
			f, err := toFloat(out.ValueIface)
			if err != nil {
				t.Error(err)
			}
			b.Outputs[n].Value = f
		}
		validateTransaction(b, TestTx, t)
	}
}
