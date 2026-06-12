// Package pool 提供 gRPC 连接池管理。
// 通过 etcd 发现后端 gRPC 服务，维护连接池，支持轮询负载均衡。
package pool

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/xyxuliang/nexus-micro/core/registry"
)

// Services 所有后端服务的连接池集合。
type Services struct {
	User    *GrpcPool
	Home    *GrpcPool
	Payment *GrpcPool
}

// GrpcPool 管理到一组 gRPC 后端的连接池。
// 每个后端实例维护一个独立的 grpc.ClientConn，通过轮询选择。
type GrpcPool struct {
	serviceName string // 目标服务名
	registry    registry.Registry

	mu        sync.RWMutex
	conns     map[string]*grpc.ClientConn // instanceID → conn
	instances []*registry.ServiceInstance // 当前实例列表
	next      atomic.Uint32               // 轮询计数器
}

// New 创建一个 gRPC 连接池。
func New(serviceName string, reg registry.Registry) *GrpcPool {
	return &GrpcPool{
		serviceName: serviceName,
		registry:    reg,
		conns:       make(map[string]*grpc.ClientConn),
	}
}

// Start 启动连接池：从 etcd 发现实例并建立连接。
func (p *GrpcPool) Start(ctx context.Context) error {
	instances, err := p.registry.Discover(ctx, p.serviceName)
	if err != nil {
		return fmt.Errorf("pool %s discover: %w", p.serviceName, err)
	}

	p.mu.Lock()
	for _, inst := range instances {
		if _, ok := p.conns[inst.ID]; !ok {
			conn, err := p.dial(inst)
			if err != nil {
				log.Printf("[pool] %s: dial %s: %v", p.serviceName, inst.Endpoints[0], err)
				continue
			}
			p.conns[inst.ID] = conn
		}
	}
	p.instances = instances
	p.mu.Unlock()

	// Watch 实时更新
	ch, err := p.registry.Watch(ctx, p.serviceName)
	if err != nil {
		return fmt.Errorf("pool %s watch: %w", p.serviceName, err)
	}
	go p.watchLoop(ch)

	return nil
}

// dial 建立到单个实例的 gRPC 连接。
func (p *GrpcPool) dial(inst *registry.ServiceInstance) (*grpc.ClientConn, error) {
	addr := inst.Endpoints[0]
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		return nil, err
	}
	log.Printf("[pool] %s connected -> %s (%s)", p.serviceName, addr, inst.ID)
	return conn, nil
}

// watchLoop 监听实例变更，自动增删连接。
func (p *GrpcPool) watchLoop(ch <-chan []*registry.ServiceInstance) {
	for instances := range ch {
		p.mu.Lock()
		p.instances = instances

		newIDs := make(map[string]bool)
		for _, inst := range instances {
			newIDs[inst.ID] = true
			if _, ok := p.conns[inst.ID]; !ok {
				conn, err := p.dial(inst)
				if err != nil {
					log.Printf("[pool] %s: dial %s: %v", p.serviceName, inst.Endpoints[0], err)
					continue
				}
				p.conns[inst.ID] = conn
			}
		}

		// 清理已下线实例的连接
		for id, conn := range p.conns {
			if !newIDs[id] {
				conn.Close()
				delete(p.conns, id)
				log.Printf("[pool] %s: removed %s", p.serviceName, id)
			}
		}
		p.mu.Unlock()
	}
}

// Select 轮询选择一个 gRPC 连接。
// 使用 atomic 计数器实现无锁轮询。
func (p *GrpcPool) Select() (*grpc.ClientConn, string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.instances) == 0 {
		return nil, "", fmt.Errorf("pool %s: no instances", p.serviceName)
	}

	n := p.next.Add(1)
	idx := int(n) % len(p.instances)
	inst := p.instances[idx]

	conn, ok := p.conns[inst.ID]
	if !ok {
		return nil, "", fmt.Errorf("pool %s: conn not found for %s", p.serviceName, inst.ID)
	}

	log.Printf("[pool] %s -> %s (%s)", p.serviceName, inst.ID, inst.Endpoints[0])
	return conn, inst.ID, nil
}

// Close 关闭所有连接。
func (p *GrpcPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, conn := range p.conns {
		conn.Close()
		delete(p.conns, id)
	}
}
