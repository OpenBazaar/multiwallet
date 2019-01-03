package client

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/OpenBazaar/multiwallet/client/blockbook"
	"golang.org/x/net/proxy"
)

var maximumBackoff = 30 * time.Second

type healthState struct {
	lastFailedAt    time.Time
	backoffDuration time.Duration
}

func (h *healthState) markUnhealthy() {
	var now = time.Now()
	if now.Before(h.nextAvilable()) {
		// can't be unhealthy before it's available
		return
	}
	if now.Before(h.lastFailedAt.Add(5 * time.Minute)) {
		h.backoffDuration *= 2
		if h.backoffDuration > maximumBackoff {
			h.backoffDuration = maximumBackoff
		}
	} else {
		h.backoffDuration = 2 * time.Second
	}
	h.lastFailedAt = now
}

func (h *healthState) isHealthy() bool {
	return time.Now().After(h.nextAvailable())
}

func (h *healthState) nextAvailable() time.Time {
	return h.lastFailedAt.Add(h.backoffDuration)
}

const nilTarget = RotationTarget("")

type (
	RotationTarget  string
	rotationManager struct {
		currentTarget RotationTarget
		targetHealth  map[RotationTarget]*healthState
		clientCache   map[RotationTarget]*blockbook.BlockBookClient
		rotateLock    sync.RWMutex
		waiter        sync.WaitGroup
	}
	reqFunc func(string, string, []byte, url.Values) (*http.Response, error)
)

func newRotationManager(targets []string, proxyDialer proxy.Dialer, doReq reqFunc) (*rotationManager, error) {
	var (
		targetHealth = make(map[RotationTarget]*healthState)
		clients      = make(map[RotationTarget]*blockbook.BlockBookClient)
	)
	for _, apiUrl := range targets {
		c, err := blockbook.NewBlockBookClient(apiUrl, proxyDialer)
		if err != nil {
			return nil, err
		}
		c.RequestFunc = doReq
		clients[RotationTarget(apiUrl)] = c
		targetHealth[RotationTarget(apiUrl)] = &healthState{}
	}
	m := &rotationManager{
		clientCache:   clients,
		currentTarget: nilTarget,
		targetHealth:  targetHealth,
	}
	m.rotateLock.Lock()
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
		r.waiter.Done()
	}
}

func (r *rotationManager) StartCurrent() error {
	if err := r.clientCache[r.currentTarget].Start(); err != nil {
		return err
	}
	r.rotateLock.Unlock()
	return nil
}

func (r *rotationManager) WaitUntilClosed() { r.waiter.Wait() }

func (r *rotationManager) FailCurrent() {
	r.targetHealth[r.currentTarget].markUnhealthy()
	r.currentTarget = nilTarget
}

func (r *rotationManager) SelectNext() {
	if r.currentTarget == nilTarget {
		var nextAvailableAt time.Time
		for {
			if time.Now().Before(nextAvailableAt) {
				continue
			}
			for target, health := range r.targetHealth {
				if health.isHealthy() {
					r.currentTarget = target
					r.waiter.Add(1)
					return
				}
				if health.nextAvailable().After(nextAvailableAt) {
					nextAvailableAt = health.nextAvailable()
				}
			}
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
