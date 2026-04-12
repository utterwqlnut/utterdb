package server

import "sync"

type internalKeyValueStore struct {
	store  map[string]any
	ramUse float32
	mut    sync.RWMutex
}

func newInternalKeyValueStore() *internalKeyValueStore {
	return &internalKeyValueStore{
		store: make(map[string]any),
	}
}
