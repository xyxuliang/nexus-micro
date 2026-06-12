// Package circuitbreaker 提供自适应熔断器实现。
// 基于滑动窗口统计错误率和慢请求比例，支持三态（Closed/Open/HalfOpen）。
// 当错误率超过阈值，熔断器打开，快速拒绝请求，避免故障扩散。
package circuitbreaker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// State 熔断器状态。
type State int32

const (
	// StateClosed 熔断器关闭 —— 允许请求通过。
	StateClosed State = iota
	// StateOpen 熔断器打开 —— 快速拒绝请求。
	StateOpen
	// StateHalfOpen 熔断器半开 —— 允许少量试探请求。
	StateHalfOpen
)

// String 返回状态字符串表示。
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker 是熔断器接口。
type CircuitBreaker interface {
	// Allow 判断当前请求是否允许通过。
	Allow() bool

	// OnSuccess 记录请求成功。
	OnSuccess(duration time.Duration)

	// OnFailure 记录请求失败。
	OnFailure(duration time.Duration)

	// State 返回当前状态。
	State() State
}

// Config 熔断器配置。
type Config struct {
	WindowSize   time.Duration // 滑动窗口大小（默认 10s）
	BucketCount  int           // 桶数量（默认 10）
	MinRequests  int           // 触发熔断的最小请求数（默认 20）
	ErrorRate    float64       // 错误率阈值（默认 0.5 = 50%）
	SlowRate    float64       // 慢请求比例阈值（默认 0.5 = 50%）
	SlowThreshold time.Duration // 慢请求判定（默认 500ms）
	SleepWindow  time.Duration // 熔断休眠时间（默认 30s）
	HalfOpenMax  int           // 半开状态最大试探请求数（默认 3）
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		WindowSize:   10 * time.Second,
		BucketCount:  10,
		MinRequests:  20,
		ErrorRate:    0.5,
		SlowRate:     0.5,
		SlowThreshold: 500 * time.Millisecond,
		SleepWindow:  30 * time.Second,
		HalfOpenMax:  3,
	}
}

// AdaptiveCB 是自适应熔断器实现。
type AdaptiveCB struct {
	cfg    *Config
	state  atomic.Int32
	mu     sync.Mutex
	buckets []bucket
	window []bucket // 滑动窗口

	openSince time.Time     // 打开时间
	halfOpenAttempts int   // 半开状态已尝试次数
}

// bucket 滑动窗口的一个桶。
type bucket struct {
	total    int         // 总请求数
	success  int         // 成功请求数
	slow     int         // 慢请求数
}

// New 创建自适应熔断器。
func New(cfg *Config) *AdaptiveCB {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	cb := &AdaptiveCB{
		cfg:    cfg,
		buckets: make([]bucket, cfg.BucketCount),
		window: make([]bucket, 0, cfg.BucketCount),
	}
	cb.state.Store(int32(StateClosed))
	return cb
}

// Allow 判断请求是否允许通过。
func (cb *AdaptiveCB) Allow() bool {
	state := State(cb.state.Load())

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// 检查是否已过休眠窗口
		if time.Since(cb.openSince) >= cb.cfg.SleepWindow {
			cb.mu.Lock()
			cb.state.Store(int32(StateHalfOpen))
			cb.halfOpenAttempts = 0
			cb.mu.Unlock()
			return true
		}
		return false
	case StateHalfOpen:
		cb.mu.Lock()
		if cb.halfOpenAttempts >= cb.cfg.HalfOpenMax {
			cb.mu.Unlock()
			return false
		}
		cb.halfOpenAttempts++
		cb.mu.Unlock()
		return true
	default:
		return true
	}
}

// OnSuccess 记录请求成功。
func (cb *AdaptiveCB) OnSuccess(duration time.Duration) {
	cb.record(true, duration)
	cb.checkState()
}

// OnFailure 记录请求失败。
func (cb *AdaptiveCB) OnFailure(duration time.Duration) {
	cb.record(false, duration)
	cb.checkState()
}

// record 记录本次请求。
func (cb *AdaptiveCB) record(success bool, duration time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// 写入当前桶
	bucket := &cb.buckets[time.Now().Second()%len(cb.buckets)]
	bucket.total++
	if success {
		if duration >= cb.cfg.SlowThreshold {
			bucket.slow++
		}
		bucket.success++
	}
}

// checkState 检查是否需要改变状态。
func (cb *AdaptiveCB) checkState() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := State(cb.state.Load())

	// 统计滑动窗口
	total, errors, slow := cb.collectStats()
	if total < cb.cfg.MinRequests {
		return // 请求量太少，不触发熔断
	}

	errRate := float64(errors) / float64(total)
	slowRate := float64(slow) / float64(total)

	switch state {
	case StateClosed:
		// 如果错误率或慢请求比例超过阈值，打开熔断器
		if errRate >= cb.cfg.ErrorRate || slowRate >= cb.cfg.SlowRate {
			cb.state.Store(int32(StateOpen))
			cb.openSince = time.Now()
			// 清空统计
			cb.resetBuckets()
		}

	case StateHalfOpen:
		// 半开状态，如果这次成功，说明服务已恢复，关闭熔断器
		if errors == 0 {
			cb.state.Store(int32(StateClosed))
			cb.resetBuckets()
		} else {
			// 仍然失败，重新打开
			cb.state.Store(int32(StateOpen))
			cb.openSince = time.Now()
		}
	}
}

// collectStats 收集滑动窗口统计。
func (cb *AdaptiveCB) collectStats() (total, errors, slow int) {
	for _, b := range cb.buckets {
		if b.total == 0 {
			continue
		}
		total += b.total
		errors += b.total - b.success
		slow += b.slow
	}
	return
}

// resetBuckets 清空所有桶。
func (cb *AdaptiveCB) resetBuckets() {
	for i := range cb.buckets {
		cb.buckets[i] = bucket{}
	}
}

// State 返回当前状态。
func (cb *AdaptiveCB) State() State {
	return State(cb.state.Load())
}

// GetErrorRate 返回当前错误率。
func (cb *AdaptiveCB) GetErrorRate() float64 {
	total, errors, _ := cb.collectStats()
	if total == 0 {
		return 0
	}
	return float64(errors) / float64(total)
}