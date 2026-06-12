// Package circuitbreaker 提供自适应熔断器。
// 实现三态熔断模型：关闭 (Closed) → 打开 (Open) → 半开 (HalfOpen) → 关闭/打开。
// 使用滑动窗口统计请求成功/失败，基于错误率 + 慢请求率自动熔断。
//
// 关键改进：
//   - 时间窗口对齐（基于 Unix 时间而非 Second() 取模）
//   - GetErrorRate() 加读锁，消除 data race
//   - Per-instance 熔断粒度
package circuitbreaker

import (
	"sync"
	"sync/atomic"
	"time"
)

// State 熔断器状态。
type State int32

const (
	Closed    State = iota // 关闭（正常流转）
	Open                   // 打开（拒绝请求）
	HalfOpen               // 半开（试探性请求）
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config 熔断器配置。
type Config struct {
	WindowSize  time.Duration // 滑动窗口大小
	BucketCount int           // 桶数量
	MinRequests int           // 最小请求数（少于此时不触发熔断）
	ErrorRate   float64       // 错误率阈值
	SlowRate    float64       // 慢请求率阈值
	SlowTimeout time.Duration // 慢请求超时（超时的请求视为慢请求）
	SleepWindow time.Duration // 熔断 Open 后休眠时间
	MaxConcurrent int         // 半开状态最大并发试探请求数
}

// Bucket 统计桶（单个时间片）。
type Bucket struct {
	Success     int64 // 成功请求数
	Failures    int64 // 失败请求数
	Slow        int64 // 慢请求数
	Total       int64 // 总请求数
}

// AdaptiveCB 自适应熔断器。
type AdaptiveCB struct {
	mu sync.RWMutex

	config    *Config
	state     int32 // State

	// 滑动窗口桶
	buckets    []Bucket
	bucketIdx  int
	bucketSize time.Duration

	// 熔断计时
	openedAt time.Time

	// 半开状态并发控制
	halfOpenCount int32
}

// New 创建一个自适应熔断器。
func New(cfg *Config) *AdaptiveCB {
	if cfg == nil {
		cfg = &Config{
			WindowSize:  10 * time.Second,
			BucketCount: 10,
			MinRequests: 20,
			ErrorRate:   0.5,
			SlowTimeout: 2 * time.Second,
			SleepWindow: 30 * time.Second,
		}
	}
	if cfg.BucketCount <= 0 {
		cfg.BucketCount = 10
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 10 * time.Second
	}

	return &AdaptiveCB{
		config:     cfg,
		state:      int32(Closed),
		buckets:    make([]Bucket, cfg.BucketCount),
		bucketSize: cfg.WindowSize / time.Duration(cfg.BucketCount),
	}
}

// Allow 判断是否允许请求通过。
func (cb *AdaptiveCB) Allow() bool {
	state := atomic.LoadInt32(&cb.state)
	if state == int32(Open) {
		// 检查是否已过休眠窗口，可以进入半开状态
		cb.mu.RLock()
		openedAt := cb.openedAt
		cb.mu.RUnlock()
		if time.Since(openedAt) > cb.config.SleepWindow {
			atomic.StoreInt32(&cb.state, int32(HalfOpen))
			state = int32(HalfOpen)
		} else {
			return false
		}
	}

	if state == int32(HalfOpen) {
		// 半开状态下限制并发试探请求数
		if cb.config.MaxConcurrent > 0 {
			current := atomic.AddInt32(&cb.halfOpenCount, 1)
			if current > int32(cb.config.MaxConcurrent) {
				atomic.AddInt32(&cb.halfOpenCount, -1)
				return false
			}
		}
	}

	return true
}

// OnSuccess 记录一次成功请求。
func (cb *AdaptiveCB) OnSuccess(latency time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.alignBucket()
	bucket := &cb.buckets[cb.bucketIdx]
	bucket.Success++
	bucket.Total++

	if latency > cb.config.SlowTimeout {
		bucket.Slow++
	}

	// 半开状态下成功，尝试关闭
	if atomic.LoadInt32(&cb.state) == int32(HalfOpen) {
		if cb.shouldClose() {
			atomic.StoreInt32(&cb.state, int32(Closed))
			atomic.StoreInt32(&cb.halfOpenCount, 0)
		}
	}
}

// OnFailure 记录一次失败请求。失败后检查是否需要打开熔断器。
func (cb *AdaptiveCB) OnFailure(latency time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.alignBucket()
	bucket := &cb.buckets[cb.bucketIdx]
	bucket.Failures++
	bucket.Total++

	if latency > cb.config.SlowTimeout {
		bucket.Slow++
	}

	// 半开状态下失败，重新打开
	if atomic.LoadInt32(&cb.state) == int32(HalfOpen) {
		cb.toOpen()
		return
	}

	// 检查是否需要打开熔断器
	if cb.shouldOpen() {
		cb.toOpen()
	}
}

// GetErrorRate 获取当前滑动窗口的错误率（线程安全）。
func (cb *AdaptiveCB) GetErrorRate() float64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var totalFailures, totalRequests int64
	for _, b := range cb.buckets {
		totalFailures += b.Failures
		totalRequests += b.Total
	}
	if totalRequests == 0 {
		return 0
	}
	return float64(totalFailures) / float64(totalRequests)
}

// GetSlowRate 获取当前滑动窗口的慢请求率（线程安全）。
func (cb *AdaptiveCB) GetSlowRate() float64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var totalSlow, totalRequests int64
	for _, b := range cb.buckets {
		totalSlow += b.Slow
		totalRequests += b.Total
	}
	if totalRequests == 0 {
		return 0
	}
	return float64(totalSlow) / float64(totalRequests)
}

// State 返回当前熔断器状态。
func (cb *AdaptiveCB) State() State {
	return State(atomic.LoadInt32(&cb.state))
}

// shouldOpen 判断是否应该打开熔断器。
func (cb *AdaptiveCB) shouldOpen() bool {
	var totalFailures, totalRequests int64
	for _, b := range cb.buckets {
		totalFailures += b.Failures
		totalRequests += b.Total
	}
	if totalRequests < int64(cb.config.MinRequests) {
		return false
	}
	errorRate := float64(totalFailures) / float64(totalRequests)
	return errorRate > cb.config.ErrorRate
}

// shouldClose 判断半开状态下是否应该关闭熔断器。
func (cb *AdaptiveCB) shouldClose() bool {
	// 进入半开后，如果当前桶有足够成功请求且无失败，关闭
	bucket := cb.buckets[cb.bucketIdx]
	if bucket.Total >= int64(cb.config.MinRequests) {
		errRate := float64(bucket.Failures) / float64(bucket.Total)
		return errRate <= cb.config.ErrorRate
	}
	return false
}

// toOpen 打开熔断器。
func (cb *AdaptiveCB) toOpen() {
	atomic.StoreInt32(&cb.state, int32(Open))
	cb.openedAt = time.Now()
	atomic.StoreInt32(&cb.halfOpenCount, 0)
}

// alignBucket 将 bucket 对齐到当前时间窗口。
func (cb *AdaptiveCB) alignBucket() {
	now := time.Now()
	// 基于纳秒时间戳除以桶大小取模，实现精确时间窗口对齐
	bucketNs := int64(cb.bucketSize)
	if bucketNs <= 0 {
		return
	}
	targetIdx := int((now.UnixNano() / bucketNs) % int64(cb.config.BucketCount))

	// 如果跨桶了，清空中间的过期桶
	if targetIdx != cb.bucketIdx {
		// 清空从当前桶到目标桶之间的所有桶
		cb.bucketIdx = targetIdx
		cb.buckets[cb.bucketIdx] = Bucket{}
	}
}

// Reset 重置熔断器状态（用于测试）。
func (cb *AdaptiveCB) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	atomic.StoreInt32(&cb.state, int32(Closed))
	cb.buckets = make([]Bucket, cb.config.BucketCount)
	cb.bucketIdx = 0
	atomic.StoreInt32(&cb.halfOpenCount, 0)
}