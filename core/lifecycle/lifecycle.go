// Package lifecycle 提供服务生命周期管理。
// 负责管理服务从启动到优雅关闭的完整生命周期，
// 包括信号监听、健康检查、资源清理等。
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/xyxuliang/nexus-micro/core/di"
)

// State 表示服务的当前生命周期状态。
type State int32

const (
	// StateInitialized 服务已初始化，尚未启动。
	StateInitialized State = iota

	// StateStarting 服务正在启动中。
	StateStarting

	// StateRunning 服务正常运行中。
	StateRunning

	// StateStopping 服务正在优雅关闭中。
	StateStopping

	// StateStopped 服务已停止。
	StateStopped
)

// String 返回状态的字符串表示。
func (s State) String() string {
	switch s {
	case StateInitialized:
		return "initialized"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Manager 是生命周期管理器。
// 负责协调服务的启动、运行和关闭流程。
type Manager struct {
	state     atomic.Int32       // 当前状态
	ctx       context.Context    // 生命周期 context
	cancel    context.CancelFunc // 取消函数
	container *di.Container      // DI 容器
	hooks     []Hook             // 生命周期钩子
	mu        sync.Mutex
}

// Hook 是生命周期钩子函数。
// OnStart 在服务启动后调用，OnStop 在服务关闭前调用。
type Hook struct {
	Name    string
	OnStart func(ctx context.Context) error
	OnStop  func(ctx context.Context) error
}

// Provider 是 Manager 的 DI Provider 实现。
type Provider struct {
	hooks []Hook
}

// NewProvider 创建生命周期管理器 Provider。
func NewProvider(hooks ...Hook) *Provider {
	return &Provider{hooks: hooks}
}

func (p *Provider) Name() string        { return "lifecycle" }
func (p *Provider) DependsOn() []string { return []string{"config"} }

func (p *Provider) Init(ctx context.Context, c *di.Container) error {
	lifeCtx, cancel := context.WithCancel(ctx)
	mgr := &Manager{
		ctx:       lifeCtx,
		cancel:    cancel,
		container: c,
		hooks:     p.hooks,
	}
	mgr.state.Store(int32(StateInitialized))
	c.RegisterInstance("lifecycle", mgr)
	return nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	return nil
}

// Run 启动服务并等待停止信号。
// 该方法会阻塞直到收到 SIGINT 或 SIGTERM 信号。
func (m *Manager) Run(startFn func(ctx context.Context) error) error {
	// 状态转换：Initialized → Starting
	m.state.Store(int32(StateStarting))

	// 启动服务
	if startFn != nil {
		if err := startFn(m.ctx); err != nil {
			m.state.Store(int32(StateStopped))
			return fmt.Errorf("lifecycle: failed to start: %w", err)
		}
	}

	// 状态转换：Starting → Running
	m.state.Store(int32(StateRunning))

	// 执行 OnStart 钩子
	for _, hook := range m.hooks {
		if hook.OnStart != nil {
			if err := hook.OnStart(m.ctx); err != nil {
				return fmt.Errorf("lifecycle: hook %s OnStart failed: %w", hook.Name, err)
			}
		}
	}

	// 等待停止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-m.ctx.Done():
	}

	// 优雅关闭
	return m.Shutdown()
}

// Shutdown 执行优雅关闭流程。
func (m *Manager) Shutdown() error {
	// 状态转换：Running → Stopping
	m.state.Store(int32(StateStopping))

	// 执行 OnStop 钩子（逆序）
	for i := len(m.hooks) - 1; i >= 0; i-- {
		hook := m.hooks[i]
		if hook.OnStop != nil {
			if err := hook.OnStop(m.ctx); err != nil {
				return fmt.Errorf("lifecycle: hook %s OnStop failed: %w", hook.Name, err)
			}
		}
	}

	// 取消 context
	m.cancel()

	// 状态转换：Stopping → Stopped
	m.state.Store(int32(StateStopped))
	return nil
}

// State 返回当前生命周期状态。
func (m *Manager) State() State {
	return State(m.state.Load())
}

// Context 返回生命周期 context。
// 当服务关闭时，此 context 会被取消。
func (m *Manager) Context() context.Context {
	return m.ctx
}

// IsRunning 检查服务是否正在运行。
func (m *Manager) IsRunning() bool {
	return m.State() == StateRunning
}

// Stop 主动触发服务关闭。
func (m *Manager) Stop() {
	m.cancel()
}

// AddHook 添加生命周期钩子。
func (m *Manager) AddHook(hook Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// FromContainer 从 DI 容器中获取 Manager 实例。
func FromContainer(c *di.Container) *Manager {
	inst, ok := c.Get("lifecycle")
	if !ok {
		return nil
	}
	mgr, _ := inst.(*Manager)
	return mgr
}
