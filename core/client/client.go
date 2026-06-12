// Package client 提供 RPC 客户端实现。
// 自动处理服务发现、负载均衡、熔断、限流、超时、重试、链路追踪传递。
// 用户只需一行代码即可发起调用：resp, err := client.Call(ctx, "method", req)。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/xyxuliang/nexus-micro/core/balancer"
	"github.com/xyxuliang/nexus-micro/core/circuitbreaker"
	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/internal/errors"
)

// Client 是 RPC 客户端，封装了完整的服务调用链路。
type Client struct {
	serviceName string                     // 目标服务名
	registry    registry.Registry          // 服务注册中心
	balancer    balancer.LoadBalancer      // 负载均衡
	cb          *circuitbreaker.AdaptiveCB // 熔断器
	timeout     time.Duration              // 请求超时
	retries     int                        // 重试次数
	retryDelay  time.Duration              // 初始重试延迟（后续指数退避）
	httpClient  *http.Client               // HTTP 传输层
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

// WithRetries 设置重试次数和初始延迟（指数退避）。
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

// WithHTTPClient 设置自定义 HTTP 客户端。
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// New 创建一个新的 RPC 客户端。
func New(serviceName string, opts ...Option) *Client {
	c := &Client{
		serviceName: serviceName,
		timeout:     3 * time.Second,
		retries:     3,
		retryDelay:  100 * time.Millisecond,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
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

// Call 发起 RPC 调用（HTTP 传输）。
// method 格式为 "VERB /path"，如 "POST /api/v1/users" 或 "GET /api/v1/users/:id"。
// req 为请求体（会被 JSON 序列化），GET 请求时可为 nil。
// 返回 JSON 反序列化后的响应体。
func (c *Client) Call(ctx context.Context, method string, req interface{}) (interface{}, error) {
	if !c.cb.Allow() {
		return nil, errors.ErrCircuitBreakerOpen
	}

	start := time.Now()
	var lastErr error

	for attempt := 0; attempt <= c.retries; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, c.timeout)
		resp, err := c.doCall(callCtx, method, req)
		cancel() // 必须在每次循环中 cancel，避免泄露

		if err == nil {
			c.cb.OnSuccess(time.Since(start))
			return resp, nil
		}

		lastErr = err
		c.cb.OnFailure(time.Since(start))

		// 不可重试的错误直接返回
		if !isRetryable(err) {
			return nil, err
		}

		// 最后一次不再等待
		if attempt >= c.retries {
			break
		}

		// 指数退避：delay * 2^attempt，最大 5 秒
		backoff := time.Duration(float64(c.retryDelay) * math.Pow(2, float64(attempt)))
		if backoff > 5*time.Second {
			backoff = 5 * time.Second
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, fmt.Errorf("client: all %d attempts failed: %w", c.retries, lastErr)
}

// doCall 执行一次 HTTP 调用（服务发现 → 负载均衡 → HTTP 请求）。
func (c *Client) doCall(ctx context.Context, method string, req interface{}) (interface{}, error) {
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

	inst, err := c.balancer.Select(ctx, instances)
	if err != nil {
		return nil, fmt.Errorf("client: select instance failed: %w", err)
	}
	if inst == nil {
		return nil, fmt.Errorf("client: no instance selected")
	}

	// 解析 HTTP 方法和路径
	httpMethod, path := parseMethod(method)

	// 构建 URL
	url := fmt.Sprintf("http://%s:%d%s", inst.Address, inst.Port, path)

	// 序列化请求体
	var body io.Reader
	if req != nil && httpMethod != "GET" {
		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("client: marshal request failed: %w", err)
		}
		body = bytes.NewReader(data)
	}

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, httpMethod, url, body)
	if err != nil {
		return nil, fmt.Errorf("client: create request failed: %w", err)
	}

	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Accept", "application/json")

	// 注入链路追踪头
	injectTracingHeaders(ctx, httpReq)

	// 执行 HTTP 请求
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("client: http call failed: %w", err)
	}
	defer httpResp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("client: read response failed: %w", err)
	}

	// 反序列化响应
	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("client: unmarshal response failed: %w", err)
	}

	// 检查 HTTP 状态码
	if httpResp.StatusCode >= 500 {
		return nil, fmt.Errorf("client: server error %d: %s", httpResp.StatusCode, string(respBody))
	}

	return result, nil
}

// ServiceName 返回目标服务名。
func (c *Client) ServiceName() string {
	return c.serviceName
}

// parseMethod 解析 "VERB /path" 格式的 method 参数。
func parseMethod(method string) (verb, path string) {
	parts := splitTwo(method, " ")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// 默认 POST，method 作为路径
	return "POST", method
}

// splitTwo 按分隔符切分为最多两部分。
func splitTwo(s, sep string) []string {
	idx := indexOf(s, sep)
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+len(sep):]}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// injectTracingHeaders 注入链路追踪头到 HTTP 请求。
func injectTracingHeaders(ctx context.Context, req *http.Request) {
	// 从 context 中提取 tracing 信息
	type traceGetter interface {
		TraceID() string
		SpanID() string
	}

	// 尝试从 context 提取 span context
	if sc, ok := ctx.Value(struct{}{}).(traceGetter); ok {
		if tid := sc.TraceID(); tid != "" {
			req.Header.Set("X-Trace-ID", tid)
		}
		if sid := sc.SpanID(); sid != "" {
			req.Header.Set("X-Span-ID", sid)
		}
	}
}

// isRetryable 判断错误是否可重试。
func isRetryable(err error) bool {
	if err == errors.ErrCircuitBreakerOpen {
		return false
	}
	// 网络错误、超时、5xx 服务端错误可以重试
	return true
}

// NoRegistryClient 创建一个无需注册中心的客户端（直接指定地址）。
// 适用于测试或已知目标地址的场景。
type NoRegistryClient struct {
	address    string
	httpClient *http.Client
	timeout    time.Duration
}

// NewNoRegistryClient 创建直接地址客户端。
func NewNoRegistryClient(address string, timeout time.Duration) *NoRegistryClient {
	return &NoRegistryClient{
		address: address,
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Call 直接调用指定地址的 HTTP 接口。
func (c *NoRegistryClient) Call(ctx context.Context, method, path string, req interface{}) (interface{}, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	url := fmt.Sprintf("http://%s%s", c.address, path)

	httpMethod := "POST"
	if method != "" {
		httpMethod = method
	}

	var body io.Reader
	if req != nil && httpMethod != "GET" {
		data, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequestWithContext(callCtx, httpMethod, url, body)
	if err != nil {
		return nil, err
	}

	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}