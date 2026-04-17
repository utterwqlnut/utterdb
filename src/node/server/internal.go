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
	log    []Function
	shards int
}

func newInternalKeyValueStore(shards int) *internalKeyValueStore {
	return &internalKeyValueStore{
		store:  make([]map[Stringable]Stringable, shards),
		mut:    make([]sync.RWMutex, shards),
		shards: shards,
	}
}

func hash(key Stringable) uint64 {
	return murmur3.Sum64([]byte(key.Stringify()))
}

func (kv *internalKeyValueStore) getShard(key Stringable) int {
	return int(hash(key)) % len(kv.store)
}

func (kv *internalKeyValueStore) clearLog() {
	kv.log = kv.log[:0]
}

func withinHashRange(startHash uint64, endHash uint64, hash uint64) bool {
	return startHash < endHash && hash > startHash && hash < endHash ||
		endHash < startHash && (hash > startHash || hash < endHash)
}

func (kv *internalKeyValueStore) getSnapShot(shardId int, startHash uint64, endHash uint64) map[Stringable]Stringable {
	kv.mut[shardId].RLock()

	snapshot := make(map[Stringable]Stringable)

	for key, value := range kv.store[shardId] {
		hashNum := hash(key)
		if withinHashRange(startHash, endHash, hashNum) {
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

func (kv *internalKeyValueStore) write(key Stringable, value Stringable, toLog bool, shardRestrict int, startHash uint64, endHash uint64) {
	shardId := kv.getShard(key)
	kv.mut[shardId].Lock()

	kv.store[shardId][key] = value

	if toLog && shardId <= shardRestrict && withinHashRange(startHash, endHash, hash(key)) {
		kv.log = append(kv.log, Function{"write", key, value})
	}

	kv.mut[shardId].Unlock()
}

func (kv *internalKeyValueStore) erase(key Stringable, toLog bool, shardRestrict int, startHash uint64, endHash uint64) error {
	shardId := kv.getShard(key)

	kv.mut[shardId].Lock()

	_, ok := kv.store[shardId][key]

	if ok != true {
		kv.mut[shardId].Unlock()
		return errors.New("Key not in the store")
	}

	delete(kv.store[shardId], key)

	if toLog && shardId <= shardRestrict && withinHashRange(startHash, endHash, hash(key)) {
		kv.log = append(kv.log, Function{"erase", key, nil})
	}

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
