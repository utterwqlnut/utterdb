package config

type Config struct {
	Nodes              []string `yaml:"nodes"`
	Proxies            []string `yaml:"proxies"`
	Manipulator        string   `yaml:"manipulator"`
	NLBPort            int      `yaml:"nlb_port"`
	Shards             int      `yaml:"shards"`
	ReplicationFactor  int      `yaml:"replication_factor"`
	Memory      struct {
		Swappiness int `yaml:"swappiness"`
	} `yaml:"memory"`
}
