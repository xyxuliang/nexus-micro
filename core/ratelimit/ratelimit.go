// Package ratelimit 提供令牌桶限流器。
// 支持全局、服务和接口三级限流，使用 atomic 操作保证高并发性能。
package ratelimit

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Limiter 限流器接口。
type Limiter interface {
	Allow() bool
}

// TokenBucket 令牌桶限流器。
// 使用 float64 令牌 + atomic 操作，精度足够且并发安全。
type TokenBucket struct {
	rate      float64       // 每秒生成的令牌数
	burst     float64       // 桶容量（最大突发数）
	tokens    atomic.Uint64 // 当前令牌数（乘以 1e6 存储为整数）
	lastRefill int64        // 上次填充时间（纳秒时间戳）
}

// New 创建令牌桶限流器。
// rate: 每秒生成的令牌数
// burst: 桶容量（最大突发令牌数）
func New(rate, burst int) *TokenBucket {
	if burst <= 0 {
		burst = rate
	}
	if rate <= 0 {
		rate = 1
	}

	tb := &TokenBucket{
		rate:  float64(rate),
		burst: float64(burst),
	}
	tb.tokens.Store(uint64(burst * 1e6)) // 初始满桶
	tb.lastRefill = time.Now().UnixNano()
	return tb
}

// Allow 消耗一个令牌。返回 true 表示请求成功，false 表示被限流。
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN 消耗 n 个令牌。返回 true 表示请求成功。
func (tb *TokenBucket) AllowN(n int) bool {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&tb.lastRefill)

	// 计算应补充的令牌数
	elapsed := float64(now-last) / 1e9
	toAdd := elapsed * tb.rate

	if toAdd > 0 {
		// 尝试 CAS 更新时间戳并补充令牌
		if atomic.CompareAndSwapInt64(&tb.lastRefill, last, now) {
			newTokens := tb.tokensFloat() + toAdd
			if newTokens > tb.burst {
				newTokens = tb.burst
			}
			tb.setTokensFloat(newTokens)
		}
		// CAS 失败说明其他 goroutine 已经更新，直接重试
	}

	// 消耗令牌
	need := float64(n) * 1e6
	for {
		current := tb.tokens.Load()
		if float64(current) < need {
			return false
		}
		if tb.tokens.CompareAndSwap(current, current-uint64(need)) {
			return true
		}
	}
}

// tokensFloat 返回当前令牌数（float64）。
func (tb *TokenBucket) tokensFloat() float64 {
	return float64(tb.tokens.Load()) / 1e6
}

// setTokensFloat 设置令牌数（float64）。
func (tb *TokenBucket) setTokensFloat(t float64) {
	tb.tokens.Store(uint64(t * 1e6))
}

// MultiLevel 多级限流器。
// 支持全局、服务级、接口级三级限流。
type MultiLevel struct {
	mu        sync.RWMutex
	global    *TokenBucket
	services  map[string]*TokenBucket
	methods   map[string]*TokenBucket
}

// NewMultiLevel 创建多级限流器。
func NewMultiLevel(globalRate, globalBurst int) *MultiLevel {
	return &MultiLevel{
		global:   New(globalRate, globalBurst),
		services: make(map[string]*TokenBucket),
		methods:  make(map[string]*TokenBucket),
	}
}

// AddService 添加服务级限流器。
func (ml *MultiLevel) AddService(name string, rate, burst int) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.services[name] = New(rate, burst)
}

// Allow 判断请求是否通过。（全局 → 服务 → 接口）
func (ml *MultiLevel) Allow(service, method string) bool {
	// 全局限流
	if !ml.global.Allow() {
		return false
	}

	// 服务级限流
	ml.mu.RLock()
	svcLimiter, hasSvc := ml.services[service]
	ml.mu.RUnlock()
	if hasSvc && !svcLimiter.Allow() {
		return false
	}

	// 接口级限流
	ml.mu.RLock()
	methodLimiter, hasMethod := ml.methods[method]
	ml.mu.RUnlock()
	if hasMethod && !methodLimiter.Allow() {
		return false
	}

	return true
}

// Rate 返回当前令牌生成速率。
func (tb *TokenBucket) Rate() float64 {
	return tb.rate
}

// Available 返回当前可用令牌数。
func (tb *TokenBucket) Available() float64 {
	tb.refill()
	return tb.tokensFloat()
}

// refill 强制补充令牌。
func (tb *TokenBucket) refill() {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&tb.lastRefill)
	elapsed := float64(now-last) / 1e9
	if elapsed <= 0 {
		return
	}
	toAdd := elapsed * tb.rate
	if atomic.CompareAndSwapInt64(&tb.lastRefill, last, now) {
		newTokens := tb.tokensFloat() + toAdd
		newTokens = math.Min(newTokens, tb.burst)
		tb.setTokensFloat(newTokens)
	}
}

// 编译期接口断言
var _ Limiter = (*TokenBucket)(nil)