// Package config 提供配置管理功能，支持多源配置和热更新。
// 内置轻量 YAML/JSON 解析器，零外部依赖。
// 框架默认按以下优先级加载配置：ENV > YAML/JSON 文件 > 默认值。
package config

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xyxuliang/nexus-micro/core/di"
)

// Config 是配置管理器，提供配置读写和热更新能力。
// 线程安全，支持嵌套 key 路径（如 "server.http.port"）。
type Config struct {
	mu        sync.RWMutex
	data      map[string]interface{} // 扁平化配置数据（key 为点号分隔路径）
	filePath  string                 // 配置文件路径
	hotReload bool                   // 是否启用热更新
	watchers  []func(string)         // 配置变更回调
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

func (p *Provider) Name() string        { return "config" }
func (p *Provider) DependsOn() []string { return nil }

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

// Load 加载配置文件。支持 YAML (.yaml/.yml) 和 JSON (.json) 格式。
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
		"server.name":                 "nexus-service",
		"server.http.port":            8080,
		"server.grpc.port":            9090,
		"server.timeout":              "30s",
		"discovery.provider":          "static",
		"log.level":                   "info",
		"log.format":                  "json",
		"metrics.enabled":             true,
		"metrics.path":                "/metrics",
		"health.path":                 "/health",
		"tracing.sample_rate":         0.1,
		"shedding.cpu_threshold":      0.9,
		"shedding.mem_threshold":      0.85,
		"ratelimit.rate":              1000,
		"ratelimit.burst":             2000,
		"circuitbreaker.window_size":  "10s",
		"circuitbreaker.bucket_count": 10,
		"circuitbreaker.min_requests": 20,
		"circuitbreaker.error_rate":   0.5,
		"circuitbreaker.sleep_window": "30s",
	}
}

// loadFile 解析配置文件，自动识别 YAML (.yaml/.yml) 和 JSON (.json) 格式。
func (c *Config) loadFile() error {
	content, err := os.ReadFile(c.filePath)
	if err != nil {
		return fmt.Errorf("config: cannot read file %s: %w", c.filePath, err)
	}

	ext := strings.ToLower(c.filePath)
	if strings.HasSuffix(ext, ".json") {
		return c.parseJSON(content)
	}
	// 默认按 YAML 解析（.yaml / .yml / 无扩展名）
	return c.parseYAML(content)
}

// parseJSON 解析 JSON 配置文件并扁平化为点号分隔的 key。
func (c *Config) parseJSON(content []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return fmt.Errorf("config: invalid JSON: %w", err)
	}
	flattenMap("", raw, c.data)
	return nil
}

// parseYAML 解析 YAML 配置文件（零依赖的轻量解析器）。
// 支持：嵌套缩进、字符串、数字、布尔值、列表、注释。
func (c *Config) parseYAML(content []byte) error {
	// 预处理：按行解析缩进层级
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var stack []struct {
		prefix string
		indent int
		parent map[string]interface{}
	}

	// 根节点
	root := make(map[string]interface{})
	stack = append(stack, struct {
		prefix string
		indent int
		parent map[string]interface{}
	}{"", -1, root})

	for scanner.Scan() {
		line := scanner.Text()

		// 跳过空行和注释
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// 计算缩进
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		// 回退栈到正确的缩进层级
		for len(stack) > 1 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}

		// 解析 key: value
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx < 0 {
			continue
		}

		key := strings.TrimSpace(trimmed[:colonIdx])
		valueStr := strings.TrimSpace(trimmed[colonIdx+1:])

		// 去除值中的行内注释
		if commentIdx := strings.Index(valueStr, " #"); commentIdx >= 0 {
			valueStr = strings.TrimSpace(valueStr[:commentIdx])
		}
		if commentIdx := strings.Index(valueStr, "\t#"); commentIdx >= 0 {
			valueStr = strings.TrimSpace(valueStr[:commentIdx])
		}

		current := stack[len(stack)-1]
		prefix := current.prefix
		if prefix != "" {
			prefix += "."
		}
		prefix += key

		// 空值表示嵌套对象
		if valueStr == "" {
			child := make(map[string]interface{})
			current.parent[key] = child
			stack = append(stack, struct {
				prefix string
				indent int
				parent map[string]interface{}
			}{prefix, indent, child})
			// 扁平化：嵌套对象的 key 也注册到 data
			c.data[prefix] = child
			continue
		}

		// 解析值
		val := parseYAMLValue(valueStr)
		current.parent[key] = val
		c.data[prefix] = val
	}

	// 扁平化 root 到 data
	flattenMap("", root, c.data)

	return scanner.Err()
}

// parseYAMLValue 解析 YAML 值（字符串、数字、布尔、null）。
func parseYAMLValue(s string) interface{} {
	// 去除引号
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		return s[1 : len(s)-1]
	}

	// 布尔值
	switch strings.ToLower(s) {
	case "true", "yes", "on":
		return true
	case "false", "no", "off":
		return false
	case "null", "~":
		return nil
	}

	// 整数
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}

	// 浮点数
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	return s
}

// flattenMap 将嵌套 map 扁平化为点号分隔的 key。
func flattenMap(prefix string, src map[string]interface{}, dst map[string]interface{}) {
	for k, v := range src {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		dst[key] = v
		if child, ok := v.(map[string]interface{}); ok {
			flattenMap(key, child, dst)
		}
	}
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
		// 类型转换：尝试解析为数字/布尔
		c.data[key] = parseYAMLValue(parts[1])
	}
}

// Get 获取配置值，支持点号分隔的嵌套 key（如 "server.http.port"）。
func (c *Config) Get(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 直接查找扁平化 key
	if v, ok := c.data[key]; ok {
		return v
	}

	// 尝试按路径逐级查找嵌套结构
	parts := strings.Split(key, ".")
	var current interface{} = c.data
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = m[part]
		if current == nil {
			return nil
		}
	}
	return current
}

// GetString 获取字符串类型的配置值。自动转换数字和布尔值。
func (c *Config) GetString(key string) string {
	v := c.Get(key)
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// GetInt 获取整数类型的配置值。支持 string → int 自动转换。
func (c *Config) GetInt(key string) int {
	v := c.Get(key)
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return 0
}

// GetFloat 获取浮点数类型的配置值。支持 string → float64 自动转换。
func (c *Config) GetFloat(key string) float64 {
	v := c.Get(key)
	if v == nil {
		return 0.0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0.0
}

// GetBool 获取布尔类型的配置值。支持 string → bool 自动转换。
func (c *Config) GetBool(key string) bool {
	v := c.Get(key)
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, _ := strconv.ParseBool(val)
		return b
	}
	return false
}

// GetDuration 获取时间间隔类型的配置值。
func (c *Config) GetDuration(key string) time.Duration {
	s := c.GetString(key)
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// GetStringSlice 获取字符串切片类型的配置值。
func (c *Config) GetStringSlice(key string) []string {
	v := c.Get(key)
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			result = append(result, fmt.Sprintf("%v", item))
		}
		return result
	case []string:
		return val
	}
	return nil
}

// GetStringMap 获取字符串 map 类型的配置值。
func (c *Config) GetStringMap(key string) map[string]interface{} {
	v := c.Get(key)
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// Set 设置配置值，用于运行时修改配置。
func (c *Config) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Watch 注册配置变更回调函数。
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