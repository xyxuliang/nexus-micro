// Package grpc 提供 gRPC 服务器实现。
// 将标准 gRPC server 集成到 Nexus Micro 框架中，与 HTTP server 共享中间件管道和治理组件。
package grpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/xyxuliang/nexus-micro/core"
	"github.com/xyxuliang/nexus-micro/core/balancer"
	"github.com/xyxuliang/nexus-micro/core/circuitbreaker"
	"github.com/xyxuliang/nexus-micro/core/ratelimit"
	"github.com/xyxuliang/nexus-micro/core/shedding"
)

// Server 是 gRPC 服务器。
// 封装了 google.golang.org/grpc.Server，提供治理组件集成和优雅关闭。
type Server struct {
	mu      sync.Mutex
	name    string
	addr    string
	server  *grpc.Server

	// 治理组件
	limiter  *ratelimit.TokenBucket
	breaker  *circuitbreaker.AdaptiveCB
	shedder  shedding.Shedder
	lb       balancer.LoadBalancer

	// 中间件链
	chain *core.MiddlewareChain

	// 健康检查
	healthServer *health.Server

	started bool
}

// Option 是 Server 的函数式配置选项。
type Option func(*Server)

// WithName 设置服务名称。
func WithName(name string) Option {
	return func(s *Server) { s.name = name }
}

// WithAddr 设置监听地址。
func WithAddr(addr string) Option {
	return func(s *Server) { s.addr = addr }
}

// WithRateLimit 设置限流器。
func WithRateLimit(rate, burst int) Option {
	return func(s *Server) {
		s.limiter = ratelimit.New(&ratelimit.Config{Rate: rate, Burst: burst})
	}
}

// WithCircuitBreaker 设置熔断器。
func WithCircuitBreaker(cfg *circuitbreaker.Config) Option {
	return func(s *Server) {
		s.breaker = circuitbreaker.New(cfg)
	}
}

// WithShedder 设置降级器。
func WithShedder(sd shedding.Shedder) Option {
	return func(s *Server) { s.shedder = sd }
}

// WithMiddleware 设置中间件链。
func WithMiddleware(chain *core.MiddlewareChain) Option {
	return func(s *Server) { s.chain = chain }
}

// WithGRPCOptions 设置 gRPC Server 选项。
func WithGRPCOptions(opts ...grpc.ServerOption) Option {
	return func(s *Server) {
		s.server = grpc.NewServer(opts...)
	}
}

// New 创建一个新的 gRPC Server。
func New(opts ...Option) *Server {
	s := &Server{
		name:    "nexus-grpc",
		addr:    ":9090",
		chain:   core.NewMiddlewareChain(),
		shedder: &shedding.Dummy{},
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.server == nil {
		s.server = grpc.NewServer()
	}

	// 注册健康检查
	s.healthServer = health.NewServer()
	grpc_health_v1.RegisterHealthServer(s.server, s.healthServer)
	reflection.Register(s.server)

	return s
}

// GrpcServer 返回底层的 gRPC Server 实例（用于注册 proto 服务）。
func (s *Server) GrpcServer() *grpc.Server {
	return s.server
}

// Start 启动 gRPC 服务器。
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("grpc: server already started")
	}
	s.started = true
	s.mu.Unlock()

	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("grpc: failed to listen on %s: %w", s.addr, err)
	}

	// 设置运行状态
	s.healthServer.SetServingStatus(s.name, grpc_health_v1.HealthCheckResponse_SERVING)

	log.Printf("[grpc] %s server starting on %s", s.name, s.addr)
	return s.server.Serve(lis)
}

// Stop 优雅关闭 gRPC 服务器。
func (s *Server) Stop(ctx context.Context) error {
	// 设置未就绪状态
	s.healthServer.SetServingStatus(s.name, grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[grpc] %s server stopped gracefully", s.name)
		return nil
	case <-ctx.Done():
		s.server.Stop()
		log.Printf("[grpc] %s server stopped forcefully", s.name)
		return ctx.Err()
	}
}

// =============================================================================
// gRPC 拦截器（Unary Server Interceptor）
// =============================================================================

// UnaryInterceptor 是 gRPC 一元拦截器。
// 集成治理流水线：限流 → 降级 → 熔断 → 业务处理。
func (s *Server) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 1. 限流检查
		if s.limiter != nil && !s.limiter.Allow() {
			return nil, fmt.Errorf("rate limit exceeded")
		}

		// 2. 降级检查
		if s.shedder != nil && !s.shedder.Allow() {
			return nil, fmt.Errorf("service overloaded")
		}

		// 3. 熔断检查
		if s.breaker != nil && !s.breaker.Allow() {
			return nil, fmt.Errorf("circuit breaker open")
		}

		// 4. 执行业务逻辑
		start := time.Now()
		resp, err := handler(ctx, req)

		// 5. 熔断状态更新
		if s.breaker != nil {
			if err != nil {
				s.breaker.OnFailure(time.Since(start))
			} else {
				s.breaker.OnSuccess(time.Since(start))
			}
		}

		return resp, err
	}
}

// WithUnaryInterceptor 注册一元拦截器到 gRPC Server。
func (s *Server) WithUnaryInterceptor() {
	if s.server != nil {
		// 注意：需要创建一个新的 Server 并复制已注册的服务
		// 此方法在创建时调用，确保拦截器在服务注册前设置
		opts := []grpc.ServerOption{
			grpc.UnaryInterceptor(s.UnaryInterceptor()),
		}
		s.server = grpc.NewServer(opts...)
		grpc_health_v1.RegisterHealthServer(s.server, s.healthServer)
		reflection.Register(s.server)
	}
}