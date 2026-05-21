package config

type Config struct {
	Nodes       []string `yaml:"nodes"`
	Proxies     []string `yaml:"proxies"`
	Manipulator string   `yaml:"manipulator"`
	Shards      int      `yaml:"shards"`
	Memory      struct {
		Swappiness int `yaml:"swappiness"`
	} `yaml:"memory"`
}
