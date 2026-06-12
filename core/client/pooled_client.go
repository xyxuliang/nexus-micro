// Package client RPC 长连接客户端实现。
// 提供 HTTP/gRPC 连接池，复用 TCP 连接，支持：
//   - 连接池（HTTP Keep-Alive / gRPC 连接复用）
//   - etcd 服务发现 + Watch 实时更新
//   - 负载均衡（轮询 / 加权 / 最少连接 / 一致性哈希）
//   - 熔断、重试、超时
//   - 健康检查与连接淘汰
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/xyxuliang/nexus-micro/core/balancer"
	"github.com/xyxuliang/nexus-micro/core/circuitbreaker"
	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/internal/errors"
)

// PooledClient 带连接池的 RPC 客户端。
// 维护 HTTP Keep-Alive 连接池，复用 TCP 连接减少握手开销。
// 通过 etcd Watch 实时感知服务实例上下线。
type PooledClient struct {
	serviceName string
	registry    registry.Registry
	lb          balancer.LoadBalancer
	cb          *circuitbreaker.AdaptiveCB
	timeout     time.Duration
	retries     int
	retryDelay  time.Duration

	// HTTP 连接池（每个 upstream 实例一个）
	// 使用 http.Transport 内置连接池实现 Keep-Alive
	pool   map[string]*http.Client // instanceID → HTTP client
	poolMu sync.RWMutex

	// 实例列表（由 Watch 实时更新）
	instances   []*registry.ServiceInstance
	instancesMu sync.RWMutex
}

// PooledOption 客户端配置选项。
type PooledOption func(*PooledClient)

// PooledWithRegistry 设置注册中心。
func PooledWithRegistry(reg registry.Registry) PooledOption {
	return func(c *PooledClient) { c.registry = reg }
}

// PooledWithBalancer 设置负载均衡。
func PooledWithBalancer(lb balancer.LoadBalancer) PooledOption {
	return func(c *PooledClient) { c.lb = lb }
}

// PooledWithTimeout 设置超时。
func PooledWithTimeout(d time.Duration) PooledOption {
	return func(c *PooledClient) { c.timeout = d }
}

// PooledWithRetries 设置重试。
func PooledWithRetries(n int, delay time.Duration) PooledOption {
	return func(c *PooledClient) { c.retries = n; c.retryDelay = delay }
}

// PooledWithCircuitBreaker 设置熔断器。
func PooledWithCircuitBreaker(cfg *circuitbreaker.Config) PooledOption {
	return func(c *PooledClient) {
		if cfg != nil {
			c.cb = circuitbreaker.New(cfg)
		}
	}
}

// NewPooled 创建带连接池的 RPC 客户端。
func NewPooled(serviceName string, opts ...PooledOption) *PooledClient {
	c := &PooledClient{
		serviceName: serviceName,
		timeout:     5 * time.Second,
		retries:     3,
		retryDelay:  100 * time.Millisecond,
		pool:        make(map[string]*http.Client),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.lb == nil {
		c.lb = balancer.New(nil)
	}
	if c.cb == nil {
		c.cb = circuitbreaker.New(nil)
	}

	return c
}

// httpTransport 创建优化的 HTTP Transport（连接池配置）。
func httpTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second, // TCP Keep-Alive
		}).DialContext,
		MaxIdleConns:        100,              // 最大空闲连接数
		MaxIdleConnsPerHost: 10,               // 每个 host 最大空闲连接
		IdleConnTimeout:     90 * time.Second, // 空闲连接超时
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false, // 启用 Keep-Alive
	}
}

// getOrCreateClient 获取或创建到指定实例的 HTTP 客户端。
// 每个 upstream 实例一个 http.Client（含独立连接池）。
func (c *PooledClient) getOrCreateClient(inst *registry.ServiceInstance) *http.Client {
	c.poolMu.RLock()
	cli, ok := c.pool[inst.ID]
	c.poolMu.RUnlock()
	if ok {
		return cli
	}

	c.poolMu.Lock()
	defer c.poolMu.Unlock()

	// double-check
	if cli, ok = c.pool[inst.ID]; ok {
		return cli
	}

	cli = &http.Client{
		Transport: httpTransport(),
		Timeout:   c.timeout,
	}
	c.pool[inst.ID] = cli
	return cli
}

// removeClient 从连接池移除指定实例的客户端。
func (c *PooledClient) removeClient(instanceID string) {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()

	if cli, ok := c.pool[instanceID]; ok {
		cli.CloseIdleConnections()
		delete(c.pool, instanceID)
	}
}

// Start 启动客户端，连接到 etcd 并开始 watch。
// 调用后客户端会自动保持实例列表同步。
func (c *PooledClient) Start(ctx context.Context) error {
	if c.registry == nil {
		return fmt.Errorf("pooled client: no registry configured")
	}

	// 首次发现
	instances, err := c.registry.Discover(ctx, c.serviceName)
	if err != nil {
		return fmt.Errorf("pooled client: discover %s failed: %w", c.serviceName, err)
	}
	c.instancesMu.Lock()
	c.instances = instances
	c.instancesMu.Unlock()

	// Watch 实时更新
	ch, err := c.registry.Watch(ctx, c.serviceName)
	if err != nil {
		return fmt.Errorf("pooled client: watch %s failed: %w", c.serviceName, err)
	}

	go c.watchLoop(ch)

	return nil
}

// watchLoop 后台监听实例变更。
func (c *PooledClient) watchLoop(ch <-chan []*registry.ServiceInstance) {
	for newInstances := range ch {
		c.instancesMu.Lock()
		oldInstances := c.instances
		c.instances = newInstances
		c.instancesMu.Unlock()

		// 清理已下线实例的连接
		oldIDs := make(map[string]bool)
		for _, inst := range oldInstances {
			oldIDs[inst.ID] = true
		}
		for _, inst := range newInstances {
			delete(oldIDs, inst.ID)
		}
		for id := range oldIDs {
			c.removeClient(id)
		}
	}
}

// Call 发起 RPC 调用（使用长连接池）。
// path: 请求路径（如 "/api/v1/users"）
// req:  请求体（JSON 序列化）
// 返回: 响应 JSON 反序列化后的 map
func (c *PooledClient) Call(ctx context.Context, path string, req interface{}) (map[string]interface{}, error) {
	// 1. 熔断检查
	if !c.cb.Allow() {
		return nil, errors.ErrCircuitBreakerOpen
	}

	start := time.Now()
	var lastErr error

	for attempt := 0; attempt <= c.retries; attempt++ {
		resp, err := c.doCall(ctx, path, req)

		if err == nil {
			c.cb.OnSuccess(time.Since(start))
			return resp, nil
		}

		lastErr = err
		c.cb.OnFailure(time.Since(start))

		if !isRetryable(err) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.retryDelay):
		}
	}

	return nil, fmt.Errorf("pooled client: all %d attempts failed: %w", c.retries, lastErr)
}

// CallRaw 发起 RPC 调用并返回原始 HTTP 响应。
func (c *PooledClient) CallRaw(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	if !c.cb.Allow() {
		return nil, errors.ErrCircuitBreakerOpen
	}

	start := time.Now()
	var lastErr error

	for attempt := 0; attempt <= c.retries; attempt++ {
		resp, err := c.doCallRaw(ctx, method, path, body)
		if err == nil {
			c.cb.OnSuccess(time.Since(start))
			return resp, nil
		}

		lastErr = err
		c.cb.OnFailure(time.Since(start))

		if !isRetryable(err) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.retryDelay):
		}
	}

	return nil, fmt.Errorf("pooled client: all %d attempts failed: %w", c.retries, lastErr)
}

// doCall 执行一次 JSON 调用。
func (c *PooledClient) doCall(ctx context.Context, path string, req interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("pooled client: marshal failed: %w", err)
	}

	resp, err := c.doCallRaw(ctx, "POST", path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pooled client: upstream returned %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("pooled client: decode response failed: %w", err)
	}

	return result, nil
}

// doCallRaw 执行一次原始 HTTP 调用。
func (c *PooledClient) doCallRaw(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	// 1. 获取实例列表
	c.instancesMu.RLock()
	instances := c.instances
	c.instancesMu.RUnlock()

	if len(instances) == 0 {
		return nil, fmt.Errorf("pooled client: no instances for %s", c.serviceName)
	}

	// 2. 负载均衡选择实例
	inst, err := c.lb.Select(ctx, instances)
	if err != nil {
		return nil, fmt.Errorf("pooled client: select instance failed: %w", err)
	}
	if inst == nil {
		return nil, fmt.Errorf("pooled client: no instance selected")
	}
	log.Printf("[pooled] %s %s -> %s (%s)", method, path, inst.ID, inst.Endpoints[0])

	// 3. 获取或创建到该实例的长连接客户端
	httpCli := c.getOrCreateClient(inst)

	// 4. 构造请求 URL
	url := fmt.Sprintf("http://%s%s", inst.Endpoints[0], path)

	// 5. 发起请求
	var httpReq *http.Request
	if body != nil {
		httpReq, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	} else {
		httpReq, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("pooled client: create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	return httpCli.Do(httpReq)
}

// Close 关闭所有连接。
func (c *PooledClient) Close() {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()

	for id, cli := range c.pool {
		cli.CloseIdleConnections()
		delete(c.pool, id)
	}
}

// ServiceName 返回目标服务名。
func (c *PooledClient) ServiceName() string {
	return c.serviceName
}
