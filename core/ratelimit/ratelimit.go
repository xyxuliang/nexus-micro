// Package ratelimit 提供令牌桶限流实现。
// 支持服务级、方法级限流，自动注入限流响应头。
// 当请求被限流，返回错误码 8003（CodeRateLimited）。
package ratelimit

import (
	"sync"
	"time"
)

// RateLimiter 限流接口。
type RateLimiter interface {
	// Allow 判断当前请求是否允许通过。
	Allow() bool

	// Remaining 返回剩余令牌数。
	Remaining() int

	// Rate 返回每秒允许请求数。
	Rate() int
}

// Config 限流配置。
type Config struct {
	Rate  int // 每秒令牌数
	Burst int // 最大突发
}

// TokenBucket 令牌桶限流。
type TokenBucket struct {
	cfg      *Config
	mu       sync.Mutex
	tokens   int
	lastTick time.Time
}

// New 创建令牌桶限流。
func New(cfg *Config) *TokenBucket {
	now := time.Now()
	return &TokenBucket{
		cfg:      cfg,
		tokens:   cfg.Burst,
		lastTick: now,
	}
}

// Allow 判断请求是否允许通过。
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTick).Seconds()
	tb.lastTick = now

	// 补充令牌
	tb.tokens += int(elapsed * float64(tb.cfg.Rate))
	if tb.tokens > tb.cfg.Burst {
		tb.tokens = tb.cfg.Burst
	}

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// Remaining 返回剩余令牌数。
func (tb *TokenBucket) Remaining() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

// Rate 返回每秒允许请求数。
func (tb *TokenBucket) Rate() int {
	return tb.cfg.Rate
}

// Burst 返回最大突发。
func (tb *TokenBucket) Burst() int {
	return tb.cfg.Burst
}

// MultiLevel 多级限流（全局 → 服务 → 方法）。
type MultiLevel struct {
	global  *TokenBucket
	service map[string]*TokenBucket
	method  map[string]*TokenBucket
	mu      sync.RWMutex
}

// NewMultiLevel 创建多级限流。
func NewMultiLevel(globalRate, globalBurst int) *MultiLevel {
	return &MultiLevel{
		global:  New(&Config{Rate: globalRate, Burst: globalBurst}),
		service: make(map[string]*TokenBucket),
		method:  make(map[string]*TokenBucket),
	}
}

// AddService 为服务添加限流规则。
func (ml *MultiLevel) AddService(name string, rate, burst int) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.service[name] = New(&Config{Rate: rate, Burst: burst})
}

// AddMethod 为方法添加限流规则。
func (ml *MultiLevel) AddMethod(service, method string, rate, burst int) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	key := service + "." + method
	ml.method[key] = New(&Config{Rate: rate, Burst: burst})
}

// Allow 判断请求是否允许通过（多级检查）。
func (ml *MultiLevel) Allow(service, method string) bool {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	// 方法级优先
	if method != "" {
		key := service + "." + method
		if limiter, ok := ml.method[key]; ok {
			if !limiter.Allow() {
				return false
			}
		}
	}

	// 服务级
	if limiter, ok := ml.service[service]; ok {
		if !limiter.Allow() {
			return false
		}
	}

	// 全局级
	return ml.global.Allow()
}