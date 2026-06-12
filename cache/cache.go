// Package cache 提供三级缓存抽象接口。
// L1: 内存缓存，L2: Redis 分布式缓存，L3: 数据库。
// 框架不强制使用具体实现，只定义接口，业务可以自行实现。
// 默认提供 MultiLevel 实现：Memory → Redis → Database。
package cache

import (
	"context"
	"fmt"
	"time"
)

// Cache 缓存接口。
type Cache interface {
	// Get 获取缓存值，如果不存在返回 (nil, false)。
	Get(ctx context.Context, key string) ([]byte, error)

	// Set 设置缓存值，TTL 为过期时间。
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete 删除缓存值。
	Delete(ctx context.Context, key string) error

	// Exists 检查 key 是否存在。
	Exists(ctx context.Context, key string) (bool, error)
}

// MultiLevel 多级缓存：L1 内存 → L2 Redis → L3 数据库。
type MultiLevel struct {
	l1 Cache // 内存缓存（比如 freecache 或 ristretto）
	l2 Cache // Redis 缓存
}

// NewMultiLevel 创建多级缓存。
func NewMultiLevel(l1, l2 Cache) *MultiLevel {
	return &MultiLevel{
		l1: l1,
		l2: l2,
	}
}

// Get 获取缓存，按 L1 → L2 顺序查找。
func (ml *MultiLevel) Get(ctx context.Context, key string) ([]byte, error) {
	// 查 L1
	data, err := ml.l1.Get(ctx, key)
	if err == nil && data != nil {
		return data, nil
	}

	// 查 L2
	data, err = ml.l2.Get(ctx, key)
	if err == nil && data != nil {
		// 回填到 L1
		// 不等待回填完成
		go func() {
			ml.l1.Set(context.Background(), key, data, 5*time.Minute)
		}()
		return data, nil
	}

	return nil, fmt.Errorf("cache: key %q not found", key)
}

// Set 设置缓存，同时设置到 L1 和 L2。
func (ml *MultiLevel) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ml.l1.Set(ctx, key, value, ttl); err != nil {
		// L1 失败不影响 L2
	}
	return ml.l2.Set(ctx, key, value, ttl)
}

// Delete 删除缓存，同时从 L1 和 L2 删除。
func (ml *MultiLevel) Delete(ctx context.Context, key string) error {
	if err := ml.l1.Delete(ctx, key); err != nil {
		// ignore
	}
	return ml.l2.Delete(ctx, key)
}

// Exists 检查 key 是否存在（检查 L1 或 L2）。
func (ml *MultiLevel) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := ml.l1.Exists(ctx, key)
	if err == nil && exists {
		return true, nil
	}
	return ml.l2.Exists(ctx, key)
}

// MemoryCache 内存缓存接口。
type MemoryCache interface {
	Cache
}

// RedisCache Redis 缓存接口。
type RedisCache interface {
	Cache
}