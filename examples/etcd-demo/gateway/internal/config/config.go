package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig  `yaml:"server"`
	Etcd   EtcdConfig    `yaml:"etcd"`
	Proxy  ProxyConfig   `yaml:"proxy"`
	Routes []RouteConfig `yaml:"routes"`
}

type ServerConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints"`
}

type ProxyConfig struct {
	Timeout time.Duration `yaml:"timeout"`
	Retries int           `yaml:"retries"`
}

type RouteConfig struct {
	Name     string `yaml:"name"`
	Path     string `yaml:"path"`
	Method   string `yaml:"method"`
	Prefix   string `yaml:"prefix"`
	Upstream string `yaml:"upstream"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{
		Server: ServerConfig{Name: "apigw", Port: 8888},
		Etcd:   EtcdConfig{Endpoints: []string{"localhost:2379"}},
		Proxy:  ProxyConfig{Timeout: 5 * time.Second, Retries: 3},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}
