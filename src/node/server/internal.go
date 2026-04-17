package server

import (
	"errors"
	"fmt"
	"sync"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/spaolacci/murmur3"
)

type internalKeyValueStore struct {
	store []map[Stringable]Stringable
	mut   []sync.RWMutex
}

func newInternalKeyValueStore(shards int) *internalKeyValueStore {
	return &internalKeyValueStore{
		store: make([]map[Stringable]Stringable, shards),
		mut:   make([]sync.RWMutex, shards),
	}
}

func hash(key Stringable) uint64 {
	return murmur3.Sum64([]byte(key.Stringify()))
}

func (kv *internalKeyValueStore) getShard(key Stringable) int {
	return int(hash(key)) % len(kv.store)
}

func (kv *internalKeyValueStore) getSnapShot(shardId int, startHash uint64, endHash uint64) map[Stringable]Stringable {
	kv.mut[shardId].RLock()

	snapshot := make(map[Stringable]Stringable)

	for key, value := range kv.store[shardId] {
		hashNum := hash(key)
		if startHash < endHash && hashNum > startHash && hashNum < endHash {
			snapshot[key] = value
		} else if endHash < startHash && (hashNum > startHash || hashNum < endHash) {
			snapshot[key] = value
		}
	}

	kv.mut[shardId].RUnlock()

	return snapshot
}

func (kv *internalKeyValueStore) get(key Stringable) (Stringable, error) {
	shardId := kv.getShard(key)
	kv.mut[shardId].RLock()
	value, ok := kv.store[shardId][key]

	kv.mut[shardId].RUnlock()

	if ok != true {
		return nil, errors.New("Key not in the store")
	}

	return value, nil
}

func (kv *internalKeyValueStore) write(key Stringable, value Stringable) {
	shardId := kv.getShard(key)
	kv.mut[shardId].Lock()
	fmt.Println(key)
	kv.store[shardId][key] = value

	kv.mut[shardId].Unlock()
}

func (kv *internalKeyValueStore) erase(key Stringable) error {
	shardId := kv.getShard(key)

	kv.mut[shardId].Lock()

	_, ok := kv.store[shardId][key]

	if ok != true {
		kv.mut[shardId].Unlock()
		return errors.New("Key not in the store")
	}

	delete(kv.store[shardId], key)
	kv.mut[shardId].Unlock()

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
