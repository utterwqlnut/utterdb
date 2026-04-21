# UtterDB

> UtterDB is a distributed key-value store designed for scalability using consistent hashing to split data across nodes. Each node maintains a local > in-memory (sharded) key-value map. The system performs streaming-based data migration along with write-ahead logs for efficient shard rebalancingduring scaling events, with write-ahead logs helping preserve consistency throughout migration. Internally, UtterDB uses gRPC for inter-node communication and coordination, while exposing a TCP-based external client API for lightweight access. The system is designed for cloud deployment and was benchmarked under load using Locust across a multi-node cluster deployed on AWS EC2 instances, evaluating throughput, latency, and stability under stress conditions.

## Usage/Commands
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
