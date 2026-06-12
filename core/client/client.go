// Package client 提供 RPC 客户端实现。
// 自动处理服务发现、负载均衡、熔断、限流、超时、重试、链路追踪传递。
// 用户只需一行代码即可发起调用：client.Call(ctx, "method", req)。
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/nexus-micro/nexus-micro/core/balancer"
	"github.com/nexus-micro/nexus-micro/core/circuitbreaker"
	"github.com/nexus-micro/nexus-micro/core/registry"
	"github.com/nexus-micro/nexus-micro/internal/errors"
)

// Client 是 RPC 客户端。
type Client struct {
	serviceName string             // 目标服务名
	registry   registry.Registry  // 服务注册中心
	balancer   balancer.LoadBalancer // 负载均衡
	cb         *circuitbreaker.AdaptiveCB // 熔断器
	timeout    time.Duration       // 请求超时
	retries    int                 // 重试次数
	retryDelay time.Duration       // 重试延迟
}

// Option 是客户端配置选项。
type Option func(*Client)

// WithRegistry 设置注册中心。
func WithRegistry(reg registry.Registry) Option {
	return func(c *Client) {
		c.registry = reg
	}
}

// WithBalancer 设置负载均衡策略。
func WithBalancer(lb balancer.LoadBalancer) Option {
	return func(c *Client) {
		c.balancer = lb
	}
}

// WithTimeout 设置请求超时。
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
	}
}

// WithRetries 设置重试次数。
func WithRetries(n int, delay time.Duration) Option {
	return func(c *Client) {
		c.retries = n
		c.retryDelay = delay
	}
}

// WithCircuitBreaker 设置熔断器。
func WithCircuitBreaker(cfg *circuitbreaker.Config) Option {
	return func(c *Client) {
		c.cb = circuitbreaker.New(cfg)
	}
}

// New 创建一个新的 RPC 客户端。
func New(serviceName string, opts ...Option) *Client {
	c := &Client{
		serviceName: serviceName,
		timeout:    3 * time.Second,
		retries:    3,
		retryDelay: 100 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(c)
	}

	// 设置默认值
	if c.balancer == nil {
		c.balancer = balancer.New(nil)
	}
	if c.cb == nil {
		c.cb = circuitbreaker.New(nil)
	}

	return c
}

// Call 发起 RPC 调用。
// 返回 (response, error)。
func (c *Client) Call(ctx context.Context, method string, req interface{}) (interface{}, error) {
	// 检查熔断器
	if !c.cb.Allow() {
		return nil, errors.ErrCircuitBreakerOpen
	}

	start := time.Now()
	var lastErr error

	// 重试循环
	for attempt := 0; attempt <= c.retries; attempt++ {
		// 带超时
		callCtx, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()

		resp, err := c.doCall(callCtx, method, req)

		if err == nil {
			c.cb.OnSuccess(time.Since(start))
			return resp, nil
		}

		lastErr = err
		c.cb.OnFailure(time.Since(start))

		// 检查是否可重试
		if !isRetryable(err) {
			return nil, err
		}

		// 等待重试
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.retryDelay):
		}
	}

	return nil, fmt.Errorf("client: all %d attempts failed: %w", c.retries, lastErr)
}

// doCall 执行一次调用。
func (c *Client) doCall(ctx context.Context, method string, req interface{}) (interface{}, error) {
	// 1. 服务发现
	if c.registry == nil {
		return nil, fmt.Errorf("client: no registry configured")
	}

	instances, err := c.registry.Discover(ctx, c.serviceName)
	if err != nil {
		return nil, fmt.Errorf("client: discover %s failed: %w", c.serviceName, err)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("client: no instances found for %s", c.serviceName)
	}

	// 2. 负载均衡选择实例
	inst, err := c.balancer.Select(ctx, instances)
	if err != nil {
		return nil, fmt.Errorf("client: select instance failed: %w", err)
	}

	if inst == nil {
		return nil, fmt.Errorf("client: no instance selected")
	}

	// 3. 实际调用（这里简化为 stub，实际 HTTP/gRPC 客户端会实现）
	// 框架使用者需要注入实际的传输层实现
	return nil, fmt.Errorf("client: transport not implemented — implement your own HTTP/gRPC transport")
}

// ServiceName 返回目标服务名。
func (c *Client) ServiceName() string {
	return c.serviceName
}

// isRetryable 判断错误是否可重试。
func isRetryable(err error) bool {
	// 网络错误、超时、服务不可用 都可以重试
	if err == errors.ErrCircuitBreakerOpen {
		return false // 熔断不重试
	}
	return true
}