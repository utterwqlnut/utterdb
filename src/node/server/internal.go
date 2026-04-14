package server

import (
	"errors"
	"sync"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type internalKeyValueStore struct {
	store map[Stringable]Stringable
	mut   sync.RWMutex
}

func newInternalKeyValueStore() *internalKeyValueStore {
	return &internalKeyValueStore{
		store: make(map[Stringable]Stringable),
	}
}

func (kv *internalKeyValueStore) get(key Stringable) (Stringable, error) {
	kv.mut.RLock()

	value, ok := kv.store[key]

	kv.mut.RUnlock()

	if ok != true {
		return nil, errors.New("Key not in the store")
	}

	return value, nil
}

func (kv *internalKeyValueStore) write(key Stringable, value Stringable) {
	kv.mut.Lock()

	kv.store[key] = value

	kv.mut.Unlock()
}

func (kv *internalKeyValueStore) erase(key Stringable) error {
	kv.mut.Lock()

	_, ok := kv.store[key]

	if ok != true {
		kv.mut.Unlock()
		return errors.New("Key not in the store")
	}

	delete(kv.store, key)
	kv.mut.Unlock()

	return nil
}

func (kv *internalKeyValueStore) getRamUse() float32 {
	ramUse, _ := mem.VirtualMemory()
	return float32(ramUse.UsedPercent)
}

func (kv *internalKeyValueStore) getCpuUse() float32 {
	cpuUse, _ := cpu.Percent(0, false)
	return float32(cpuUse[0])
}
