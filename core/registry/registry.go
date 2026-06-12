// Package registry 提供服务注册与发现的核心接口。
// 支持四级服务发现：Static → etcd → Consul → K8s DNS。
// 默认使用 static 模式，零依赖即可启动。
package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xyxuliang/nexus-micro/core/di"
)

// ServiceInstance 表示一个服务实例的完整信息。
// 用于服务注册和发现，包含实例的唯一标识、网络端点、元数据等。
type ServiceInstance struct {
	ID        string            // 实例唯一 ID（UUID）
	Name      string            // 服务名（如 "user", "order"）
	Version   string            // 版本号（如 "v1.0.0"）
	Metadata  map[string]string // 元数据（机房、权重、标签等）
	Endpoints []string          // 端点列表（如 ["http://localhost:8080", "grpc://localhost:9090"]）
}

// Registry 是服务注册发现的统一接口。
// 所有注册中心实现（Static、etcd、Consul、K8s DNS）都必须实现此接口。
type Registry interface {
	// Register 注册服务实例到注册中心。
	Register(ctx context.Context, instance *ServiceInstance) error

	// Deregister 从注册中心注销服务实例。
	Deregister(ctx context.Context, instance *ServiceInstance) error

	// Discover 发现指定服务的所有实例。
	Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error)

	// Watch 监听服务实例变更，返回变更通知 channel。
	// 当服务实例列表发生变化时，channel 会收到最新的实例列表。
	Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error)
}

// StaticRegistry 是静态注册中心实现。
// 实例列表通过配置文件静态指定，不依赖外部注册中心。
// 适用于开发环境、测试环境和单体部署场景。
type StaticRegistry struct {
	mu        sync.RWMutex
	instances map[string][]*ServiceInstance // serviceName → instances
	watchers  map[string][]chan []*ServiceInstance
}

// NewStaticRegistry 创建静态注册中心。
// staticEndpoints 是从服务名到端点列表的映射，如：
// {"user": {"localhost:8081", "localhost:8082"}, "order": {"localhost:8091"}}
func NewStaticRegistry(staticEndpoints map[string][]string) *StaticRegistry {
	r := &StaticRegistry{
		instances: make(map[string][]*ServiceInstance),
		watchers:  make(map[string][]chan []*ServiceInstance),
	}

	for name, endpoints := range staticEndpoints {
		instances := make([]*ServiceInstance, len(endpoints))
		for i, ep := range endpoints {
			instances[i] = &ServiceInstance{
				ID:        fmt.Sprintf("%s-%d", name, i),
				Name:      name,
				Version:   "v1.0.0",
				Endpoints: []string{ep},
			}
		}
		r.instances[name] = instances
	}

	return r
}

// Register 在静态注册中心中注册实例。
func (r *StaticRegistry) Register(ctx context.Context, instance *ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.instances[instance.Name] = append(r.instances[instance.Name], instance)
	r.notifyWatchers(instance.Name)
	return nil
}

// Deregister 从静态注册中心注销实例。
func (r *StaticRegistry) Deregister(ctx context.Context, instance *ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	instances := r.instances[instance.Name]
	for i, inst := range instances {
		if inst.ID == instance.ID {
			r.instances[instance.Name] = append(instances[:i], instances[i+1:]...)
			break
		}
	}
	r.notifyWatchers(instance.Name)
	return nil
}

// Discover 发现指定服务的所有实例。
func (r *StaticRegistry) Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances, ok := r.instances[serviceName]
	if !ok {
		return nil, fmt.Errorf("registry: service %s not found", serviceName)
	}
	return instances, nil
}

// Watch 监听服务实例变更。
func (r *StaticRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan []*ServiceInstance, 10)
	r.watchers[serviceName] = append(r.watchers[serviceName], ch)

	// 立即发送当前实例列表
	instances := r.instances[serviceName]
	ch <- instances

	return ch, nil
}

// notifyWatchers 通知所有 watcher 实例列表已变更。
func (r *StaticRegistry) notifyWatchers(serviceName string) {
	instances := r.instances[serviceName]
	for _, ch := range r.watchers[serviceName] {
		select {
		case ch <- instances:
		default:
			// channel 满，丢弃通知
		}
	}
}

// HealthCheck 是健康检查接口。
type HealthCheck func(ctx context.Context, instance *ServiceInstance) bool

// Config 是注册中心的配置。
type Config struct {
	Provider        string              // 注册中心类型：static, etcd, consul, k8s
	StaticEndpoints map[string][]string // static 模式的端点配置
	EtcdEndpoints   []string            // etcd 端点
	ConsulAddr      string              // Consul 地址
	HealthCheck     HealthCheck         // 健康检查函数
	TTL             time.Duration       // 健康检查超时
}

// Provider 是 Registry 的 DI Provider 实现。
type Provider struct {
	config *Config
}

// NewProvider 创建注册中心 Provider。
func NewProvider(cfg *Config) *Provider {
	return &Provider{config: cfg}
}

func (p *Provider) Name() string        { return "registry" }
func (p *Provider) DependsOn() []string { return []string{"config"} }

func (p *Provider) Init(ctx context.Context, c *di.Container) error {
	var reg Registry

	switch p.config.Provider {
	case "static", "":
		reg = NewStaticRegistry(p.config.StaticEndpoints)
	case "etcd":
		var err error
		reg, err = NewEtcdRegistry(p.config.EtcdEndpoints, p.config.TTL)
		if err != nil {
			return fmt.Errorf("registry: etcd init failed: %w", err)
		}
	default:
		return fmt.Errorf("registry: unknown provider %s", p.config.Provider)
	}

	c.RegisterInstance("registry", reg)
	return nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	return nil
}

// FromContainer 从 DI 容器中获取 Registry 实例。
func FromContainer(c *di.Container) Registry {
	inst, ok := c.Get("registry")
	if !ok {
		return nil
	}
	reg, _ := inst.(Registry)
	return reg
}
