package client

import "sync"

type rotationManager struct {
	rotateLock sync.RWMutex
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
