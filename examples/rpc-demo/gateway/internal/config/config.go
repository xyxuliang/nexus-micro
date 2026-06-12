// Package config 提供 gRPC 网关配置。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config gRPC 网关配置。
type Config struct {
	Server ServerConfig `yaml:"server"`
	Etcd   EtcdConfig   `yaml:"etcd"`
}

// ServerConfig 服务器配置。
type ServerConfig struct {
	Name     string `yaml:"name"`
	Port     int    `yaml:"port"`
	GrpcPort int    `yaml:"grpc_port"`
}

// EtcdConfig etcd 配置。
type EtcdConfig struct {
	Endpoints []string `yaml:"endpoints"`
}

// Load 从 YAML 文件加载配置。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{
		Server: ServerConfig{Name: "apigw-rpc", Port: 8889, GrpcPort: 8890},
		Etcd:   EtcdConfig{Endpoints: []string{"localhost:2379"}},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}
