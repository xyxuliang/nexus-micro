package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Etcd   EtcdConfig   `yaml:"etcd"`
}

type ServerConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints"`
	TTL       int      `yaml:"ttl"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{
		Server: ServerConfig{Name: "home-service", Port: 8083},
		Etcd:   EtcdConfig{Endpoints: []string{"localhost:2379"}, TTL: 30},
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
