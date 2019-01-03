package client

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/OpenBazaar/multiwallet/client/blockbook"
	"golang.org/x/net/proxy"
)

const nilTarget = RotationTarget("")

type RotationTarget string
type rotationManager struct {
	currentTarget RotationTarget
	targetHealth  map[RotationTarget]struct{}
	clientCache   map[RotationTarget]*blockbook.BlockBookClient
	rotateLock    sync.RWMutex
}
type reqFunc func(string, string, []byte, url.Values) (*http.Response, error)

func newRotationManager(targets []string, proxyDialer proxy.Dialer, doReq reqFunc) (*rotationManager, error) {
	var (
		targetHealth = make(map[RotationTarget]struct{})
		clients      = make(map[RotationTarget]*blockbook.BlockBookClient)
	)
	for _, apiUrl := range targets {
		c, err := blockbook.NewBlockBookClient(apiUrl, proxyDialer)
		if err != nil {
			return nil, err
		}
		c.RequestFunc = doReq
		clients[RotationTarget(apiUrl)] = c
		targetHealth[RotationTarget(apiUrl)] = struct{}{}
	}
	m := &rotationManager{
		clientCache:   clients,
		currentTarget: nilTarget,
		targetHealth:  targetHealth,
	}
	m.Lock()
	return m, nil
}

func (r *rotationManager) AcquireCurrent() *blockbook.BlockBookClient {
	r.rotateLock.RLock()
	return r.clientCache[r.currentTarget]
}

func (r *rotationManager) ReleaseCurrent() {
	r.rotateLock.RUnlock()
}

func (r *rotationManager) CloseCurrent() {
	if r.currentTarget != nilTarget {
		r.rotateLock.Lock()
		r.clientCache[r.currentTarget].Close()
		r.currentTarget = nilTarget
	}
}

func (r *rotationManager) StartCurrent() error {
	if err := r.clientCache[r.currentTarget].Start(); err != nil {
		return err
	}
	r.rotateLock.Unlock()
	return nil
}

func (r *rotationManager) FailCurrent() {
	// TODO: Update health state
	r.currentTarget = nilTarget
}

func (r *rotationManager) SelectNext() {
	// TODO: Health check before return available target
	if r.currentTarget == nilTarget {
		for target := range r.targetHealth {
			r.currentTarget = target
			break
		}
	}
}

func (r *rotationManager) Lock() {
	r.rotateLock.Lock()
}

func (r *rotationManager) Unlock() {
	r.rotateLock.Unlock()
}

func (r *rotationManager) RLock() {
	r.rotateLock.RLock()
}

func (r *rotationManager) RUnlock() {
	r.rotateLock.RUnlock()
}
