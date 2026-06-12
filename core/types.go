// Package core 定义了 Nexus Micro 框架的核心类型和接口。
// 作为框架的根基，这里定义了所有模块共享的基础抽象。
package core

import "context"

// Server 是框架的核心接口，表示一个可运行的微服务实例。
// 每个通过 nx CLI 创建的服务最终都会实现这个接口。
type Server interface {
	// Name 返回服务名称，用于注册中心和服务发现。
	Name() string

	// Start 启动服务，包括 HTTP/gRPC 监听、注册中心注册等。
	// 该方法会阻塞直到服务收到停止信号。
	Start() error

	// Stop 优雅关闭服务，包括取消注册、关闭监听器、等待处理中的请求完成。
	Stop(ctx context.Context) error
}

// Handler 是业务处理器的统一接口。
// HTTP 和 gRPC 的请求都会经过这个接口处理，
// 框架负责将协议特定的请求转换为统一的 context 和请求体。
type Handler func(ctx context.Context, req interface{}) (interface{}, error)

// Middleware 是中间件接口，用于包装 Handler。
// 中间件可以执行前置处理、后置处理、错误处理等。
// 中间件管道在 HTTP 和 gRPC 之间完全共享。
type Middleware func(next Handler) Handler

// MiddlewareChain 表示一个有序的中间件链。
// 中间件按添加顺序执行，RequestID 最先执行，Metrics 最后执行。
type MiddlewareChain struct {
	middlewares []Middleware
}

// NewMiddlewareChain 创建一个新的中间件链。
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: make([]Middleware, 0),
	}
}

// Use 添加一个中间件到链的末尾。
func (c *MiddlewareChain) Use(mw Middleware) {
	c.middlewares = append(c.middlewares, mw)
}

// Wrap 将中间件链包裹到最终的 Handler 上。
// 中间件按添加顺序执行：第一个添加的中间件最先执行前置处理，最后执行后置处理。
func (c *MiddlewareChain) Wrap(handler Handler) Handler {
	// 从后往前包裹，确保执行顺序正确
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}
	return handler
}

// Empty 检查中间件链是否为空。
func (c *MiddlewareChain) Empty() bool {
	return len(c.middlewares) == 0
}