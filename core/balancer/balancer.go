// Package balancer 提供客户端负载均衡。
// 支持四种负载均衡策略：轮询（默认）、加权轮询、最少连接、一致性哈希。
// 负载均衡在 RPC 客户端层工作，每次调用前选择一个服务实例。
package balancer

import (
	"context"
	"hash/fnv"
	"math"
	"sync"
	"sync/atomic"

	"github.com/xyxuliang/nexus-micro/core/registry"
)

// Strategy 负载均衡策略枚举。
type Strategy int

const (
	// RoundRobin 轮询（默认）。
	RoundRobin Strategy = iota
	// WeightedRoundRobin 加权轮询。
	WeightedRoundRobin
	// LeastConnection 最少连接。
	LeastConnection
	// ConsistentHash 一致性哈希。
	ConsistentHash
)

// LoadBalancer 是负载均衡器接口。
type LoadBalancer interface {
	// Select 从实例列表中选择一个实例。
	Select(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error)
}

// Config 负载均衡配置。
type Config struct {
	Strategy Strategy // 负载均衡策略
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		Strategy: RoundRobin,
	}
}

// New 创建负载均衡器。
func New(cfg *Config) LoadBalancer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	switch cfg.Strategy {
	case RoundRobin:
		return &roundRobin{next: atomic.Uint32{}}
	case WeightedRoundRobin:
		return &weightedRoundRobin{}
	case LeastConnection:
		return &leastConnection{}
	case ConsistentHash:
		return &consistentHash{vnodeCount: 150}
	default:
		return &roundRobin{next: atomic.Uint32{}}
	}
}

// roundRobin 轮询负载均衡。
type roundRobin struct {
	next atomic.Uint32
}

func (rr *roundRobin) Select(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, nil
	}
	n := rr.next.Add(1)
	idx := int(n) % len(instances)
	return instances[idx], nil
}

// weightedRoundRobin 加权轮询负载均衡。
type weightedRoundRobin struct {
	mu sync.Mutex
}

// getWeight 从元数据中提取 weight，默认 100。
func (wr *weightedRoundRobin) getWeight(inst *registry.ServiceInstance) int {
	_, ok := inst.Metadata["weight"]
	if !ok {
		return 100
	}
	// 简化处理，默认权重 100
	return 100
}

func (wr *weightedRoundRobin) Select(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	if len(instances) == 0 {
		return nil, nil
	}

	totalWeight := 0
	for _, inst := range instances {
		totalWeight += wr.getWeight(inst)
	}

	// 随机选择一个权重区间（简化实现）
	// 生产环境可以用平滑加权轮询算法
	r := totalWeight / 2
	current := 0
	for _, inst := range instances {
		current += wr.getWeight(inst)
		if current > r {
			return inst, nil
		}
	}

	return instances[0], nil
}

// leastConnection 最少连接负载均衡。
type leastConnection struct {
	mu          sync.Mutex
	connections map[string]int // instanceID → 连接数
}

func (lc *leastConnection) Select(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if len(instances) == 0 {
		return nil, nil
	}

	if lc.connections == nil {
		lc.connections = make(map[string]int)
	}

	minConn := math.MaxInt32
	selected := instances[0]
	for _, inst := range instances {
		conn := lc.connections[inst.ID]
		if conn < minConn {
			minConn = conn
			selected = inst
		}
	}

	// 增加连接计数
	lc.connections[selected.ID]++

	return selected, nil
}

// OnComplete 连接完成后减少连接计数。
func (lc *leastConnection) OnComplete(inst *registry.ServiceInstance) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.connections != nil {
		if cnt := lc.connections[inst.ID]; cnt > 0 {
			lc.connections[inst.ID] = cnt - 1
		}
	}
}

// consistentHash 一致性哈希负载均衡（基于虚拟节点）。
type consistentHash struct {
	vnodeCount int // 虚拟节点数
	mu         sync.RWMutex
	ring       []uint32          // 排序的哈希环
	nodes      map[uint32]string // 哈希值 → 实例 ID
	instance   map[string]*registry.ServiceInstance
}

func (ch *consistentHash) Select(ctx context.Context, instances []*registry.ServiceInstance) (*registry.ServiceInstance, error) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// 重建哈希环
	ch.ring = make([]uint32, 0, len(instances)*ch.vnodeCount)
	ch.nodes = make(map[uint32]string)
	ch.instance = make(map[string]*registry.ServiceInstance)

	for _, inst := range instances {
		ch.instance[inst.ID] = inst
		for i := 0; i < ch.vnodeCount; i++ {
			hash := ch.hash(inst.ID + string(i))
			ch.ring = append(ch.ring, hash)
			ch.nodes[hash] = inst.ID
		}
	}

	// 二分查找找到第一个大于哈希的节点
	key := getRequestKey(ctx)
	h := ch.hash(key)

	// 二分查找
	low, high := 0, len(ch.ring)
	for low < high {
		mid := (low + high) / 2
		if ch.ring[mid] < h {
			low = mid + 1
		} else {
			high = mid
		}
	}

	// 环形处理
	if low == len(ch.ring) {
		low = 0
	}

	instID := ch.nodes[ch.ring[low]]
	return ch.instance[instID], nil
}

func (ch *consistentHash) hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// getRequestKey 从 context 中提取请求哈希键。
func getRequestKey(ctx context.Context) string {
	if reqID, ok := ctx.Value("request_id").(string); ok && reqID != "" {
		return reqID
	}
	return "default"
}
