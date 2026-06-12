// Package server 提供框架的核心服务启动和管理能力。
// Server 是 Nexus Micro 的入口，负责组装所有组件（DI 容器、配置、中间件、注册中心、HTTP/gRPC 监听器)
// 并协调它们的生命周期。开发者通过 Server 创建和运行微服务。
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/xyxuliang/nexus-micro/core"
	"github.com/xyxuliang/nexus-micro/core/config"
	"github.com/xyxuliang/nexus-micro/core/di"
	"github.com/xyxuliang/nexus-micro/core/lifecycle"
	"github.com/xyxuliang/nexus-micro/core/metrics"
	"github.com/xyxuliang/nexus-micro/core/middleware"
	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/core/response"
)

// Option 是 Server 的函数式配置选项。
type Option func(*Server)

// Server 是 Nexus Micro 框架的核心服务实例。
// 每个服务都是一个 Server 实例，它组装了 HTTP/gRPC 监听器、中间件管道、注册中心等组件。
type Server struct {
	mu sync.Mutex

	name       string                // 服务名称
	version    string                // 服务版本
	container  *di.Container         // DI 容器
	config     *config.Config        // 配置管理器
	lifecycle  *lifecycle.Manager    // 生命周期管理器
	registry   registry.Registry     // 服务注册中心
	chain      *core.MiddlewareChain // 中间件链
	httpServer *http.Server          // HTTP 服务器
	grpcServer interface{}           // gRPC 服务器（预留）

	httpAddr  string        // HTTP 监听地址
	grpcAddr  string        // gRPC 监听地址
	httpPort  int           // HTTP 端口
	grpcPort  int           // gRPC 端口
	rateLimit int           // 限流速率
	rateBurst int           // 限流突发
	timeout   time.Duration // 请求超时

	started     bool          // 是否已启动
	services    []interface{} // 注册的业务服务
	httpHandler http.Handler  // HTTP 处理器
	sseHandlers []struct {    // SSE 端点（延迟注册）
		pattern string
		handler http.Handler
	}
}

// New 创建一个新的 Server 实例。
// 使用函数式选项模式配置 Server，支持链式调用。
func New(opts ...Option) *Server {
	s := &Server{
		name:      "nexus-service",
		version:   "v1.0.0",
		container: di.New(),
		chain:     middleware.DefaultChain(),
		httpAddr:  ":8080",
		grpcAddr:  ":9090",
		rateLimit: 1000,
		rateBurst: 2000,
		timeout:   30 * time.Second,
		services:  make([]interface{}, 0),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// WithConfig 设置配置文件路径。
func WithConfig(path string) Option {
	return func(s *Server) {
		s.config = config.New(path, false)
	}
}

// WithHTTP 设置 HTTP 监听地址。
func WithHTTP(addr string) Option {
	return func(s *Server) {
		s.httpAddr = addr
	}
}

// WithGRPC 设置 gRPC 监听地址。
func WithGRPC(addr string) Option {
	return func(s *Server) {
		s.grpcAddr = addr
	}
}

// WithMiddleware 设置自定义中间件列表。
func WithMiddleware(mws ...core.Middleware) Option {
	return func(s *Server) {
		chain := core.NewMiddlewareChain()
		for _, mw := range mws {
			chain.Use(mw)
		}
		s.chain = chain
	}
}

// WithRateLimit 设置限流参数。
func WithRateLimit(rate, burst int) Option {
	return func(s *Server) {
		s.rateLimit = rate
		s.rateBurst = burst
	}
}

// WithTimeout 设置请求超时时间。
func WithTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.timeout = d
	}
}

// WithName 设置服务名称。
func WithName(name string) Option {
	return func(s *Server) {
		s.name = name
	}
}

// WithVersion 设置服务版本。
func WithVersion(version string) Option {
	return func(s *Server) {
		s.version = version
	}
}

// RegisterService 注册业务服务到 Server。
// 业务服务的方法会被自动暴露为 HTTP 和 gRPC 接口。
func (s *Server) RegisterService(svc interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services = append(s.services, svc)
}

// HandleSSE 注册 SSE 端点。
// 与普通 HTTP 端点不同，SSE 端点绕过 JSON 响应包装，直接写入 HTTP 响应流。
// handler 是标准的 http.Handler，通常使用 sse.NewChannelHandler 或 sse.NewHandler 创建。
//
// 使用方式：
//
//	srv.HandleSSE("/events", sse.NewChannelHandler(func(ch chan<- sse.Event, r *http.Request) {
//	    ch <- sse.Event{Data: "hello"}
//	}))
func (s *Server) HandleSSE(pattern string, handler http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 包装 SSE 中间件，确保 context 被标记为 SSE 请求
	sseHandler := middleware.SSEAware(handler)
	s.sseHandlers = append(s.sseHandlers, struct {
		pattern string
		handler http.Handler
	}{pattern, sseHandler})
}

// HandleSSEFunc 注册 SSE 端点（函数形式）。
// 便捷方法，等价于 HandleSSE(pattern, http.HandlerFunc(handler))。
//
// 使用方式：
//
//	srv.HandleSSEFunc("/events", func(w http.ResponseWriter, r *http.Request) {
//	    sw, _ := sse.NewWriter(w)
//	    sw.SendEvent(sse.Event{Data: "hello"})
//	})
func (s *Server) HandleSSEFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	s.HandleSSE(pattern, http.HandlerFunc(handler))
}

// Start 启动服务。
// 执行顺序：初始化 DI 容器 → 注册到注册中心 → 启动 HTTP/gRPC → 等待停止信号 → 优雅关闭
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("server: already started")
	}
	s.started = true
	s.mu.Unlock()

	ctx := context.Background()

	// 1. 初始化配置
	if s.config == nil {
		s.config = config.New("", false)
	}
	if err := s.config.Load(ctx); err != nil {
		return fmt.Errorf("server: config load failed: %w", err)
	}
	s.container.RegisterInstance("config", s.config)

	// 2. 初始化注册中心
	if s.registry == nil {
		cfg := &registry.Config{
			Provider: s.config.GetString("discovery.provider"),
			StaticEndpoints: map[string][]string{
				s.name: {s.httpAddr},
			},
		}
		regProvider := registry.NewProvider(cfg)
		s.container.Register(regProvider)
	}

	// 3. 初始化生命周期管理器
	lifeProvider := lifecycle.NewProvider(
		lifecycle.Hook{
			Name: "registry",
			OnStop: func(ctx context.Context) error {
				return s.deregister(ctx)
			},
		},
	)
	s.container.Register(lifeProvider)

	// 4. 初始化 DI 容器
	if err := s.container.InitAll(ctx); err != nil {
		return fmt.Errorf("server: di init failed: %w", err)
	}

	// 5. 获取生命周期管理器
	inst, ok := s.container.Get("lifecycle")
	if !ok {
		return fmt.Errorf("server: lifecycle manager not found")
	}
	s.lifecycle = inst.(*lifecycle.Manager)

	// 6. 获取注册中心
	inst, ok = s.container.Get("registry")
	if ok {
		s.registry = inst.(registry.Registry)
	}

	// 7. 注册 HTTP 路由
	mux := http.NewServeMux()
	s.setupRoutes(mux)
	s.httpHandler = mux

	// 8. 启动 HTTP 服务器
	s.httpServer = &http.Server{
		Addr:         s.httpAddr,
		Handler:      mux,
		ReadTimeout:  s.timeout,
		WriteTimeout: s.timeout,
		IdleTimeout:  120 * time.Second,
	}

	// 9. 运行服务
	return s.lifecycle.Run(func(ctx context.Context) error {
		// 注册到注册中心
		if s.registry != nil {
			instance := &registry.ServiceInstance{
				ID:        fmt.Sprintf("%s-%s", s.name, s.version),
				Name:      s.name,
				Version:   s.version,
				Endpoints: []string{fmt.Sprintf("http://localhost%s", s.httpAddr)},
			}
			if err := s.registry.Register(ctx, instance); err != nil {
				return fmt.Errorf("server: register failed: %w", err)
			}
		}

		// 启动 HTTP 服务器
		go func() {
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				// 记录 HTTP 服务器启动错误
				fmt.Fprintf(os.Stderr, "server: HTTP ListenAndServe error: %v\n", err)
			}
		}()

		return nil
	})
}

// Stop 优雅关闭服务。
func (s *Server) Stop(ctx context.Context) error {
	// 注销服务
	if err := s.deregister(ctx); err != nil {
		return err
	}

	// 关闭 HTTP 服务器
	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}

	// 关闭 DI 容器
	return s.container.ShutdownAll(ctx)
}

// deregister 从注册中心注销服务。
func (s *Server) deregister(ctx context.Context) error {
	if s.registry == nil {
		return nil
	}
	instance := &registry.ServiceInstance{
		ID:   fmt.Sprintf("%s-%s", s.name, s.version),
		Name: s.name,
	}
	return s.registry.Deregister(ctx, instance)
}

// setupRoutes 注册 HTTP 路由。
func (s *Server) setupRoutes(mux *http.ServeMux) {
	// 注册 SSE 端点（延迟注册，绕过 JSON 响应包装）
	for _, sh := range s.sseHandlers {
		mux.Handle(sh.pattern, sh.handler)
	}

	// 健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		resp := response.Success(map[string]string{
			"status": "ok",
			"name":   s.name,
		})
		resp.WriteTo(w)
	})

	// 存活检查（K8s liveness probe）
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		resp := response.Success(map[string]string{"status": "alive"})
		resp.WriteTo(w)
	})

	// 就绪检查（K8s readiness probe）
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		resp := response.Success(map[string]string{"status": "ready"})
		resp.WriteTo(w)
	})

	// 指标端点（Prometheus 格式）
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = metrics.DefaultRegistry.WriteTo(w)
	})

	// 调试端点
	mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pprof endpoint (integrate with net/http/pprof)\n"))
	})
}

// Name 返回服务名称。
func (s *Server) Name() string {
	return s.name
}

// Addr 返回 HTTP 监听地址。
func (s *Server) Addr() string {
	return s.httpAddr
}

// ListenAddr 返回实际的监听地址（用于测试）。
func (s *Server) ListenAddr() string {
	if s.httpServer != nil {
		return s.httpServer.Addr
	}
	return s.httpAddr
}

// HTTPListener 获取 HTTP 监听器（用于测试）。
func (s *Server) HTTPListener() (net.Listener, error) {
	return net.Listen("tcp", s.httpAddr)
}

// Config 返回配置管理器。
func (s *Server) Config() *config.Config {
	return s.config
}

// Container 返回 DI 容器。
func (s *Server) Container() *di.Container {
	return s.container
}
