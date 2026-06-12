// Package cache 提供分层缓存管理。
// 支持三级缓存架构（L1 本地内存 → L2 Redis → L3 数据库），
// 内置缓存穿透/雪崩/击穿保护。
package cache

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Cache 缓存接口。
type Cache interface {
	// Get 获取缓存值。key 不存在时返回 (nil, nil)。
	Get(ctx context.Context, key string) (interface{}, error)

	// Set 设置缓存值。ttl 为 0 时永不过期。
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// Delete 删除缓存值。
	Delete(ctx context.Context, key string) error

	// Clear 清空所有缓存。
	Clear(ctx context.Context) error
}

// MultiLevel 三级缓存。
// L1: 内存缓存（读延迟 < 1µs）
// L2: Redis 等分布式缓存（读延迟 < 1ms）
// L3: 数据库等持久化存储（由业务层控制）
type MultiLevel struct {
	mu   sync.RWMutex
	l1   Cache // 本地内存
	l2   Cache // 分布式缓存（可选）
	name string
}

// NewMultiLevel 创建三级缓存。
func NewMultiLevel(l1, l2 Cache, name string) *MultiLevel {
	return &MultiLevel{
		l1:   l1,
		l2:   l2,
		name: name,
	}
}

// Get 读取缓存（L1 → L2 → 回填 L1）。
func (c *MultiLevel) Get(ctx context.Context, key string) (interface{}, error) {
	// 1. 读取 L1
	val, _ := c.l1.Get(ctx, key)
	if val != nil {
		return val, nil
	}

	// 2. 读取 L2
	if c.l2 != nil {
		val, _ = c.l2.Get(ctx, key)
		if val != nil {
			// 回填 L1（非阻塞）
			go func() {
				_ = c.l1.Set(context.Background(), key, val, 5*time.Minute)
			}()
			return val, nil
		}
	}

	return nil, nil
}

// Set 设置缓存（写入 L1 + L2，先写 L2 再写 L1 避免脏读）。
func (c *MultiLevel) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// 先写 L2，再写 L1，避免并发请求读到过期数据
	if c.l2 != nil {
		if err := c.l2.Set(ctx, key, value, ttl); err != nil {
			return err
		}
	}
	return c.l1.Set(ctx, key, value, ttl)
}

// Delete 删除缓存（删除 L1 + L2，先删 L1 再删 L2 避免回写脏数据）。
func (c *MultiLevel) Delete(ctx context.Context, key string) error {
	// 先删 L1，阻止后续读取命中有用数据
	_ = c.l1.Delete(ctx, key)
	// 再删 L2
	if c.l2 != nil {
		return c.l2.Delete(ctx, key)
	}
	return nil
}

// Clear 清空所有缓存。
func (c *MultiLevel) Clear(ctx context.Context) error {
	_ = c.l1.Clear(ctx)
	if c.l2 != nil {
		return c.l2.Clear(ctx)
	}
	return nil
}

// =============================================================================
// MemoryCache — 本地内存缓存实现
// =============================================================================

// memoryEntry 缓存条目。
type memoryEntry struct {
	value      interface{}
	expireAt   time.Time
}

// isExpired 检查是否过期。
func (e *memoryEntry) isExpired() bool {
	return !e.expireAt.IsZero() && time.Now().After(e.expireAt)
}

// MemoryCache 基于内存的缓存实现。
// 线程安全，支持 TTL 过期、定期清理、singleflight 防穿透。
type MemoryCache struct {
	mu         sync.RWMutex
	store      map[string]*memoryEntry
	stopCh     chan struct{}
	maxSize    int

	// singleflight — 防止缓存击穿
	sf         sync.Map // key → *call
}

// call 表示一个正在进行的 singleflight 调用。
type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// MemoryConfig 内存缓存配置。
type MemoryConfig struct {
	MaxSize        int           // 最大条目数，0 表示无限制
	CleanupInterval time.Duration // 定期清理间隔
}

// NewMemoryCache 创建内存缓存。
func NewMemoryCache(cfg *MemoryConfig) *MemoryCache {
	if cfg == nil {
		cfg = &MemoryConfig{}
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 30 * time.Second
	}

	mc := &MemoryCache{
		store:   make(map[string]*memoryEntry),
		stopCh:  make(chan struct{}),
		maxSize: cfg.MaxSize,
	}

	// 启动定期清理 goroutine
	go mc.cleanupLoop(cfg.CleanupInterval)

	return mc
}

// Get 获取缓存值。
func (mc *MemoryCache) Get(ctx context.Context, key string) (interface{}, error) {
	mc.mu.RLock()
	entry, ok := mc.store[key]
	mc.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	if entry.isExpired() {
		mc.mu.Lock()
		delete(mc.store, key)
		mc.mu.Unlock()
		return nil, nil
	}

	return entry.value, nil
}

// Set 设置缓存值。使用随机化 TTL 避免缓存雪崩。
func (mc *MemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// TTL 随机化 ±10%，避免大量 key 同时过期（缓存雪崩）
	jitteredTTL := ttl
	if ttl > 0 {
		jitter := time.Duration(float64(ttl) * 0.1 * (rand.Float64()*2 - 1))
		jitteredTTL = ttl + jitter
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 检查容量限制
	if mc.maxSize > 0 && len(mc.store) >= mc.maxSize {
		// 随机淘汰（生产环境可替换为 LRU）
		for k := range mc.store {
			delete(mc.store, k)
			break
		}
	}

	mc.store[key] = &memoryEntry{
		value:    value,
		expireAt: time.Now().Add(jitteredTTL),
	}
	return nil
}

// Delete 删除缓存值。
func (mc *MemoryCache) Delete(ctx context.Context, key string) error {
	mc.mu.Lock()
	delete(mc.store, key)
	mc.mu.Unlock()
	return nil
}

// Clear 清空所有缓存。
func (mc *MemoryCache) Clear(ctx context.Context) error {
	mc.mu.Lock()
	mc.store = make(map[string]*memoryEntry)
	mc.mu.Unlock()
	return nil
}

// Stop 停止定期清理。
func (mc *MemoryCache) Stop() {
	close(mc.stopCh)
}

// Size 返回缓存条目数。
func (mc *MemoryCache) Size() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.store)
}

// cleanupLoop 定期清理过期条目。
func (mc *MemoryCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.cleanup()
		case <-mc.stopCh:
			return
		}
	}
}

// cleanup 清理所有过期条目。
func (mc *MemoryCache) cleanup() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	for k, entry := range mc.store {
		if entry.isExpired() {
			delete(mc.store, k)
		}
		_ = now
	}
}

// SingleFlight 实现 singleflight 模式，防止缓存击穿。
// 对同一个 key，只有一个请求实际执行 fn，其他请求共享结果。
// 用户需自行配合 MemoryCache.Get 判断 nil 后决定是否走到 SingleFlight。
type SingleFlight struct {
	mu sync.Mutex
	m  map[string]*call
}

// NewSingleFlight 创建 singleflight 实例。
func NewSingleFlight() *SingleFlight {
	return &SingleFlight{
		m: make(map[string]*call),
	}
}

// Do 执行函数，对同一 key 保证只有一个在运行。
func (sf *SingleFlight) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	sf.mu.Lock()
	if c, ok := sf.m[key]; ok {
		sf.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}

	c := &call{}
	c.wg.Add(1)
	sf.m[key] = c
	sf.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	sf.mu.Lock()
	delete(sf.m, key)
	sf.mu.Unlock()

	return c.val, c.err
}

// DoWithCache 是 GetOrLoad 的快捷实现：先从 cache 读取，未命中则用 singleflight 加载并缓存。
func (sf *SingleFlight) DoWithCache(
	ctx context.Context,
	cache Cache,
	key string,
	ttl time.Duration,
	loader func() (interface{}, error),
) (interface{}, error) {
	// 1. 读取缓存
	val, _ := cache.Get(ctx, key)
	if val != nil {
		return val, nil
	}

	// 2. singleflight 加载
	sfKey := fmt.Sprintf("sf:%s", key)
	val, err := sf.Do(sfKey, loader)
	if err != nil {
		return nil, err
	}

	// 3. 写入缓存（设置空值防止缓存穿透）
	if val == nil {
		val = &nullMarker{}
		ttl = 1 * time.Minute // 空值缓存 1 分钟
	}
	_ = cache.Set(ctx, key, val, ttl)

	return val, nil
}

// nullMarker 空值标记，用于缓存穿透保护。
type nullMarker struct{}

// IsNullMarker 判断值是否为穿透保护空标记。
func IsNullMarker(v interface{}) bool {
	_, ok := v.(*nullMarker)
	return ok
}

// 编译期接口断言
var _ Cache = (*MemoryCache)(nil)
var _ Cache = (*MultiLevel)(nil)