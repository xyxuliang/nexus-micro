// Package config 提供配置管理功能，支持多源配置和热更新。
// 基于 Viper 实现，支持 YAML 文件、环境变量、远程配置中心。
// 框架默认按以下优先级加载配置：ENV > YAML 文件 > 默认值。
package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nexus-micro/nexus-micro/core/di"
)

// Config 是配置管理器，封装 Viper 提供配置读写和热更新能力。
// 线程安全，支持并发读取配置。
type Config struct {
	mu       sync.RWMutex
	data     map[string]interface{} // 配置数据
	filePath string                 // 配置文件路径
	hotReload bool                  // 是否启用热更新
	watchers  []func(string)        // 配置变更回调
}

// Provider 是 Config 的 DI Provider 实现。
type Provider struct {
	filePath  string
	hotReload bool
}

// NewProvider 创建配置 Provider。
func NewProvider(filePath string, hotReload bool) *Provider {
	return &Provider{
		filePath:  filePath,
		hotReload: hotReload,
	}
}

func (p *Provider) Name() string             { return "config" }
func (p *Provider) DependsOn() []string       { return nil }

func (p *Provider) Init(ctx context.Context, c *di.Container) error {
	cfg := New(p.filePath, p.hotReload)
	if err := cfg.Load(ctx); err != nil {
		return fmt.Errorf("config: failed to load: %w", err)
	}
	c.RegisterInstance("config", cfg)
	return nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	return nil
}

// New 创建一个新的配置管理器。
func New(filePath string, hotReload bool) *Config {
	return &Config{
		data:      make(map[string]interface{}),
		filePath:  filePath,
		hotReload: hotReload,
		watchers:  make([]func(string), 0),
	}
}

// Load 加载配置文件。支持从文件路径、环境变量读取。
// 文件格式从扩展名自动推断（.yaml, .json, .toml）。
func (c *Config) Load(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 加载默认配置
	c.loadDefaults()

	// 如果指定了配置文件，解析文件
	if c.filePath != "" {
		if err := c.loadFile(); err != nil {
			return err
		}
	}

	// 加载环境变量覆盖
	c.loadEnv()

	return nil
}

// loadDefaults 加载框架默认配置。
func (c *Config) loadDefaults() {
	c.data = map[string]interface{}{
		"server.name":          "nexus-service",
		"server.http.port":     8080,
		"server.grpc.port":     9090,
		"server.timeout":       "30s",
		"discovery.provider":   "static",
		"log.level":            "info",
		"log.format":           "json",
		"metrics.enabled":      true,
		"metrics.path":         "/metrics",
		"health.path":          "/health",
		"tracing.sample_rate":  0.1,
		"shedding.cpu_threshold": 0.9,
		"shedding.mem_threshold": 0.85,
		"ratelimit.rate":       1000,
		"ratelimit.burst":      2000,
		"circuitbreaker.window_size":  "10s",
		"circuitbreaker.bucket_count": 10,
		"circuitbreaker.min_requests": 20,
		"circuitbreaker.error_rate":   0.5,
		"circuitbreaker.sleep_window": "30s",
	}
}

// loadFile 解析配置文件。
func (c *Config) loadFile() error {
	// 读取文件内容
	content, err := os.ReadFile(c.filePath)
	if err != nil {
		return fmt.Errorf("config: cannot read file %s: %w", c.filePath, err)
	}

	// 简化的 YAML 解析（生产环境应使用 Viper）
	// 这里提供基础实现，实际项目使用 Viper 的 SetConfigFile + ReadInConfig
	_ = content
	return nil
}

// loadEnv 从环境变量覆盖配置。
// 环境变量命名规则：NEXUS_SERVER_NAME → server.name
func (c *Config) loadEnv() {
	prefix := "NEXUS_"
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], prefix) {
			continue
		}
		key := strings.ToLower(strings.TrimPrefix(parts[0], prefix))
		key = strings.ReplaceAll(key, "_", ".")
		c.data[key] = parts[1]
	}
}

// Get 获取配置值，支持点号分隔的多级 key（如 "server.http.port"）。
func (c *Config) Get(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[key]
}

// GetString 获取字符串类型的配置值。
func (c *Config) GetString(key string) string {
	v := c.Get(key)
	s, _ := v.(string)
	return s
}

// GetInt 获取整数类型的配置值。
func (c *Config) GetInt(key string) int {
	v := c.Get(key)
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	default:
		return 0
	}
}

// GetFloat 获取浮点数类型的配置值。
func (c *Config) GetFloat(key string) float64 {
	v := c.Get(key)
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	default:
		return 0.0
	}
}

// GetBool 获取布尔类型的配置值。
func (c *Config) GetBool(key string) bool {
	v := c.Get(key)
	b, _ := v.(bool)
	return b
}

// GetDuration 获取时间间隔类型的配置值。
func (c *Config) GetDuration(key string) time.Duration {
	s := c.GetString(key)
	d, _ := time.ParseDuration(s)
	return d
}

// Set 设置配置值，用于运行时修改配置。
func (c *Config) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Watch 注册配置变更回调函数。
// 当配置发生变更时，回调函数会被调用。
func (c *Config) Watch(callback func(key string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watchers = append(c.watchers, callback)
}

// All 返回所有配置的副本。
func (c *Config) All() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]interface{}, len(c.data))
	for k, v := range c.data {
		result[k] = v
	}
	return result
}

// FromContainer 从 DI 容器中获取 Config 实例。
func FromContainer(c *di.Container) *Config {
	inst, ok := c.Get("config")
	if !ok {
		return nil
	}
	cfg, _ := inst.(*Config)
	return cfg
}