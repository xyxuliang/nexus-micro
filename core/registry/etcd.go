// Package registry etcd 服务注册与发现实现。
// 基于 etcd v3 的 lease + watch 机制实现：
//   - 服务注册：以租约 (lease) 注册，定期续约，实例宕机后自动注销
//   - 服务发现：etcd get + watch，实时感知实例上下线
//   - 长连接复用：单个 etcd client 实例贯穿进程生命周期
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// etcd 键空间前缀
const (
	etcdServicesPrefix = "/nexus-micro/services/"
)

// EtcdRegistry 基于 etcd v3 的服务注册中心。
// 使用 lease 实现自动心跳和故障检测。
// 每个服务实例在 etcd 中存储为 /nexus-micro/services/{name}/{id}。
type EtcdRegistry struct {
	client  *clientv3.Client
	timeout time.Duration

	mu        sync.RWMutex
	leaseID   clientv3.LeaseID            // 本实例的租约
	instances map[string]*ServiceInstance // local cache: instanceID → instance
	watchers  map[string]context.CancelFunc
}

// NewEtcdRegistry 创建 etcd 注册中心。
// endpoints: etcd 集群地址，如 ["localhost:2379"]
// timeout: 操作超时
func NewEtcdRegistry(endpoints []string, timeout time.Duration) (*EtcdRegistry, error) {
	if len(endpoints) == 0 {
		endpoints = []string{"localhost:2379"}
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd: connect failed: %w", err)
	}

	return &EtcdRegistry{
		client:    cli,
		timeout:   timeout,
		instances: make(map[string]*ServiceInstance),
		watchers:  make(map[string]context.CancelFunc),
	}, nil
}

// clientContext 返回带超时的 context。
func (r *EtcdRegistry) clientContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), r.timeout)
}

// serviceKey 返回服务在 etcd 中的键。
func serviceKey(name, id string) string {
	return fmt.Sprintf("%s%s/%s", etcdServicesPrefix, name, id)
}

// servicePrefix 返回服务前缀（用于查询和 watch）。
func servicePrefix(name string) string {
	return fmt.Sprintf("%s%s/", etcdServicesPrefix, name)
}

// Register 注册服务实例到 etcd。
// 创建 lease（默认 TTL 30s），将实例信息 JSON 序列化后写入 etcd。
// 启动后台 goroutine 定期续约（keepalive）。
func (r *EtcdRegistry) Register(ctx context.Context, instance *ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. 创建租约（TTL 30s）
	leaseCtx, cancel := r.clientContext()
	defer cancel()

	lease, err := r.client.Grant(leaseCtx, 30)
	if err != nil {
		return fmt.Errorf("etcd: grant lease failed: %w", err)
	}
	r.leaseID = lease.ID

	// 2. 序列化实例信息
	data, err := json.Marshal(instance)
	if err != nil {
		return fmt.Errorf("etcd: marshal instance failed: %w", err)
	}

	// 3. 写入 etcd
	putCtx, putCancel := r.clientContext()
	defer putCancel()

	key := serviceKey(instance.Name, instance.ID)
	_, err = r.client.Put(putCtx, key, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("etcd: put key %s failed: %w", key, err)
	}

	// 4. 启动 keepalive（长连接续约）
	kaCh, err := r.client.KeepAlive(context.Background(), lease.ID)
	if err != nil {
		return fmt.Errorf("etcd: keepalive failed: %w", err)
	}

	// 后台消费 keepalive 响应，保持 lease 活跃
	go func() {
		for range kaCh {
			// keepalive 响应，无需处理
		}
	}()

	// 5. 本地缓存
	r.instances[instance.ID] = instance

	return nil
}

// Deregister 从 etcd 注销服务实例。
// 撤销租约会自动删除对应的 key。
func (r *EtcdRegistry) Deregister(ctx context.Context, instance *ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.leaseID != 0 {
		// 撤销租约 → etcd 自动删除所有关联 key
		revokeCtx, cancel := r.clientContext()
		defer cancel()
		if _, err := r.client.Revoke(revokeCtx, r.leaseID); err != nil {
			return fmt.Errorf("etcd: revoke lease failed: %w", err)
		}
		r.leaseID = 0
	}

	// 如果 lease 撤销失败，手动删除 key
	delCtx, delCancel := r.clientContext()
	defer delCancel()

	key := serviceKey(instance.Name, instance.ID)
	r.client.Delete(delCtx, key)

	delete(r.instances, instance.ID)
	return nil
}

// Discover 从 etcd 发现指定服务的所有实例。
// 使用 prefix get 查询 /nexus-micro/services/{name}/ 下的所有键。
func (r *EtcdRegistry) Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error) {
	getCtx, cancel := r.clientContext()
	defer cancel()

	resp, err := r.client.Get(getCtx, servicePrefix(serviceName), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd: discover %s failed: %w", serviceName, err)
	}

	instances := make([]*ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var inst ServiceInstance
		if err := json.Unmarshal(kv.Value, &inst); err != nil {
			continue
		}
		instances = append(instances, &inst)
	}

	// 更新本地缓存
	r.mu.Lock()
	for _, inst := range instances {
		r.instances[inst.ID] = inst
	}
	r.mu.Unlock()

	return instances, nil
}

// Watch 监听服务实例变更。
// 使用 etcd watch 实现实时推送，实例上线/下线时 channel 会收到最新列表。
func (r *EtcdRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan []*ServiceInstance, 10)

	// 立即发送当前实例列表
	instances := r.getInstances(serviceName)
	ch <- instances

	// 后台 goroutine：监听 etcd key 变更
	ctx, cancel := context.WithCancel(context.Background())
	r.watchers[serviceName] = cancel

	go r.watchLoop(ctx, serviceName, ch)

	return ch, nil
}

// watchLoop 是 watch 的后台循环。
// 监听 etcd prefix 变更，实例列表变化时推送到 channel。
func (r *EtcdRegistry) watchLoop(ctx context.Context, serviceName string, ch chan<- []*ServiceInstance) {
	defer close(ch)

	// 使用 etcd 分布式锁保证只有一个 watcher 推送
	// 简化实现：直接 watch prefix
	watchChan := r.client.Watch(ctx, servicePrefix(serviceName), clientv3.WithPrefix())

	// 首次拉取全量
	r.pushInstances(ch, serviceName)

	for {
		select {
		case <-ctx.Done():
			return
		case wresp, ok := <-watchChan:
			if !ok {
				return
			}
			// 检查是否有变更
			for _, ev := range wresp.Events {
				_ = ev // 有事件即触发推送
				break
			}
			r.pushInstances(ch, serviceName)
		}
	}
}

// pushInstances 拉取并推送最新实例列表。
func (r *EtcdRegistry) pushInstances(ch chan<- []*ServiceInstance, serviceName string) {
	ctx, cancel := r.clientContext()
	defer cancel()

	instances, err := r.Discover(ctx, serviceName)
	if err != nil {
		return
	}

	select {
	case ch <- instances:
	default:
		// channel 满，丢弃
	}
}

// getInstances 从本地缓存获取实例列表。
func (r *EtcdRegistry) getInstances(serviceName string) []*ServiceInstance {
	result := make([]*ServiceInstance, 0)
	for _, inst := range r.instances {
		if inst.Name == serviceName {
			result = append(result, inst)
		}
	}
	return result
}

// Close 关闭 etcd 连接。
func (r *EtcdRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 取消所有 watcher
	for _, cancel := range r.watchers {
		cancel()
	}
	r.watchers = make(map[string]context.CancelFunc)

	return r.client.Close()
}

// =============================================================================
// etcd 分布式锁（可选）
// =============================================================================

// NewMutex 创建 etcd 分布式锁。
// 基于 etcd concurrency 包实现。
func (r *EtcdRegistry) NewMutex(ctx context.Context, name string) (*concurrency.Mutex, error) {
	session, err := concurrency.NewSession(r.client)
	if err != nil {
		return nil, fmt.Errorf("etcd: create session failed: %w", err)
	}
	return concurrency.NewMutex(session, "/nexus-micro/locks/"+strings.TrimPrefix(name, "/")), nil
}
