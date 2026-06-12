// Package di 提供依赖注入容器，是 Nexus Micro 的服务组装核心。
// 容器负责管理所有 Provider 的生命周期，按依赖顺序初始化组件。
// 所有服务组件（Config、Registry、Middleware、Transport）都通过容器注册和获取。
package di

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Provider 是依赖注入提供者接口。
// 每个需要注入的组件都实现这个接口，通过 Name() 声明自己的身份，
// 通过 DependsOn() 声明依赖关系，容器会按依赖拓扑排序初始化。
type Provider interface {
	// Name 返回 Provider 的唯一名称，用于依赖声明和查找。
	Name() string

	// DependsOn 返回 Provider 依赖的其他 Provider 名称列表。
	// 容器会在初始化本 Provider 之前先初始化所有依赖项。
	DependsOn() []string

	// Init 初始化 Provider。
	// 容器保证所有依赖项已经初始化完成后再调用此方法。
	// ctx 是框架的生命周期 context，在服务关闭时会被取消。
	Init(ctx context.Context, c *Container) error

	// Shutdown 关闭 Provider，释放资源。
	// 容器按初始化顺序的逆序调用此方法。
	Shutdown(ctx context.Context) error
}

// Container 是依赖注入容器。
// 线程安全，支持并发注册和获取。
type Container struct {
	mu        sync.RWMutex
	providers map[string]Provider
	instances map[string]interface{} // 存储已初始化的实例
	order     []string               // 初始化顺序
}

// New 创建一个新的容器实例。
func New() *Container {
	return &Container{
		providers: make(map[string]Provider),
		instances: make(map[string]interface{}),
	}
}

// Register 注册一个 Provider 到容器中。
// 如果同名 Provider 已存在，会被覆盖。
func (c *Container) Register(p Provider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.providers[p.Name()] = p
}

// RegisterInstance 直接注册一个已初始化的实例到容器中。
// 用于外部组件（如 Viper、Zerolog）的注入。
func (c *Container) RegisterInstance(name string, instance interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instances[name] = instance
}

// Get 从容器中获取指定名称的实例。
// 如果实例不存在，返回 nil 和 false。
func (c *Container) Get(name string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	inst, ok := c.instances[name]
	return inst, ok
}

// MustGet 从容器中获取实例，如果不存在则 panic。
func (c *Container) MustGet(name string) interface{} {
	inst, ok := c.Get(name)
	if !ok {
		panic(fmt.Sprintf("di: provider %s not found in container", name))
	}
	return inst
}

// InitAll 按依赖顺序初始化所有已注册的 Provider。
// 使用拓扑排序确定初始化顺序，检测循环依赖。
func (c *Container) InitAll(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 拓扑排序
	sorted, err := c.topoSort()
	if err != nil {
		return fmt.Errorf("di: failed to resolve dependencies: %w", err)
	}

	// 按顺序初始化
	for _, name := range sorted {
		p, ok := c.providers[name]
		if !ok {
			continue
		}
		if err := p.Init(ctx, c); err != nil {
			return fmt.Errorf("di: failed to init provider %s: %w", name, err)
		}
		c.order = append(c.order, name)
	}

	return nil
}

// ShutdownAll 按初始化顺序的逆序关闭所有 Provider。
func (c *Container) ShutdownAll(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 逆序关闭
	for i := len(c.order) - 1; i >= 0; i-- {
		name := c.order[i]
		p, ok := c.providers[name]
		if !ok {
			continue
		}
		if err := p.Shutdown(ctx); err != nil {
			return fmt.Errorf("di: failed to shutdown provider %s: %w", name, err)
		}
	}

	return nil
}

// topoSort 对 Provider 进行拓扑排序，检测循环依赖。
func (c *Container) topoSort() ([]string, error) {
	// 构建邻接表
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	for name := range c.providers {
		graph[name] = nil
		inDegree[name] = 0
	}

	for name, p := range c.providers {
		for _, dep := range p.DependsOn() {
			if _, ok := c.providers[dep]; !ok {
				return nil, fmt.Errorf("provider %s depends on unknown provider %s", name, dep)
			}
			graph[dep] = append(graph[dep], name)
			inDegree[name]++
		}
	}

	// Kahn 算法
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		for _, neighbor := range graph[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// 检测循环依赖
	if len(sorted) != len(c.providers) {
		return nil, fmt.Errorf("circular dependency detected")
	}

	return sorted, nil
}

// GetString 从容器中获取字符串类型的实例。
func GetString(c *Container, name string) string {
	inst := c.MustGet(name)
	s, _ := inst.(string)
	return s
}

