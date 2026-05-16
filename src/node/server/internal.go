package server

import (
	"errors"
	"sync"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/spaolacci/murmur3"
)

type internalKeyValueStore struct {
	store  []map[Stringable]Stringable
	mut    []sync.RWMutex
	shards int
}

func newInternalKeyValueStore(shards int) *internalKeyValueStore {
	store := make([]map[Stringable]Stringable, shards)
	for i := range store {
		store[i] = make(map[Stringable]Stringable)
	}
	return &internalKeyValueStore{
		store:  store,
		mut:    make([]sync.RWMutex, shards),
		shards: shards,
	}
}

func hash(key Stringable) uint64 {
	return murmur3.Sum64([]byte(key.Stringify()))
}

func (kv *internalKeyValueStore) getShard(key Stringable) int {
	return int(hash(key) % uint64(len(kv.store)))
}

func withinHashRange(startHash uint64, endHash uint64, hash uint64) bool {
	return startHash < endHash && hash > startHash && hash < endHash ||
		endHash < startHash && (hash > startHash || hash < endHash)
}

func (kv *internalKeyValueStore) lockShard(shardId int) {
	kv.mut[shardId].RLock()
}

func (kv *internalKeyValueStore) unlockShard(shardId int) {
	kv.mut[shardId].RUnlock()
}

func (kv *internalKeyValueStore) getSnapShot(shardId int, startHash uint64, endHash uint64) map[Stringable]Stringable {
	// THIS FUNCTION IS NOT THREAD SAFE CALLER MUST LOCK FOR IT
	snapshot := make(map[Stringable]Stringable)

	for key, value := range kv.store[shardId] {
		hashNum := hash(key)
		if withinHashRange(startHash, endHash, hashNum) {
			snapshot[key] = value
		}
	}
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
