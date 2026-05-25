# UtterDB

UtterDB is a distributed, in-memory key-value store that shards data across nodes with consistent hashing. Clients connect over TCP to proxies, which route reads and writes using a shared hash ring. Storage nodes serve data over gRPC and rebalance with streaming migration and write-ahead logs when the cluster scales. A manipulator owns cluster membership: it heartbeats nodes, handles `ADDNODE` / `REMOVENODE`, and pushes updated rings to every proxy via `NEWRING`. An AWS Network Load Balancer can sit in front of the proxies. Keys are replicated to `replication_factor` consecutive nodes on the ring (configured in `config.yaml`).

## AWS Benchmarking

Deploy with multiple Locust benchmark nodes using `--benchmark-nodes`; node 0 runs the Locust master and the remaining nodes run workers.

```bash
./deploy/deploy.sh --benchmark-nodes 4 --users 500 --spawn-rate 100 --duration 60s
```

## Commands

```
WRITE|key|keyType|value|valueType
eg. WRITE|foo|string|bar|string
```
```
GET|key|keyType
eg. GET|foo|string
```
```
ERASE|key|keyType
eg. ERASE|foo|string
```
```
ADDNODE|ip
eg. ADDNODE|localhost:8001
```
```
REMOVENODE|ip
eg. REMOVENODE|localhost:8001
```
```
GETRAM
```
```
GETCPU
```
