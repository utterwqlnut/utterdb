# UtterDB
This is a project to create a distributed key-value store called UtterDB. 
UtterDB shares key-value pairs equally across data nodes via consistent hashing. For data safety, 
data nodes have replicas and enforce a leaderless replication.
Internally UtterDB uses grpc, while offering an HTTP api for external clients. 
UtterDB will be able to be deployed easily via a script onto EC2 instances and have an auto-scaling feature.
