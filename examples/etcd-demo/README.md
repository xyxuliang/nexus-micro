# Nexus Micro — etcd RPC 长连接示例

演示完整的微服务架构：**单网关 + 多服务实例 + etcd 服务发现 + HTTP 长连接池**。

## 架构

```
                    ┌─────────────┐
                    │   Client    │
                    └──────┬──────┘
                           │ HTTP
                    ┌──────▼──────┐
                    │   Gateway   │ :8888
                    │  (apigw)    │
                    │  路由匹配 +  │
                    │  长连接池 +  │
                    │  负载均衡   │
                    └──┬───┬───┬──┘
           发现实例    │   │   │  HTTP Keep-Alive 代理
         ┌────────────┘   │   └────────────┐
         ▼                ▼                ▼
    ┌─────────┐    ┌──────────┐    ┌──────────┐
    │  etcd   │    │user-svc-1│    │user-svc-2│
    │ :2379   │◄───│  :8081   │    │  :8082   │
    │         │注册│          │    │          │
    └─────────┘    └──────────┘    └──────────┘
```

## 核心特性

| 特性 | 实现 |
|------|------|
| **服务注册** | 启动时注册到 etcd，lease 30s 自动续约 |
| **服务发现** | etcd prefix get + watch 实时更新 |
| **长连接** | HTTP Keep-Alive 连接池 (MaxIdleConns: 100) |
| **负载均衡** | RoundRobin 轮询（支持加权/最少连接/一致性哈希） |
| **熔断** | 自适应熔断器（滑动窗口） |
| **重试** | 超时 + 3 次重试 |
| **高可用** | etcd 3 节点集群 |

## 快速启动

### 1. 启动 etcd（单节点）

```bash
# 使用 Docker
docker run -d --name etcd \
  -p 2379:2379 -p 2380:2380 \
  quay.io/coreos/etcd:v3.5.18 \
  etcd \
  --name=etcd \
  --data-dir=/etcd-data \
  --listen-client-urls=http://0.0.0.0:2379 \
  --advertise-client-urls=http://localhost:2379 \
  --listen-peer-urls=http://0.0.0.0:2380 \
  --initial-advertise-peer-urls=http://localhost:2380 \
  --initial-cluster=etcd=http://localhost:2380

# 验证 etcd 运行
etcdctl endpoint health
```

### 2. 启动 user-service 实例

```bash
# 终端 1：启动 user-service 实例 1 (:8081)
cd examples/etcd-demo/user-service
go run main.go

# 终端 2：启动 user-service 实例 2 (:8082)
PORT=8082 go run main.go
```

### 3. 启动 API 网关

```bash
# 终端 3
cd examples/etcd-demo/gateway
go run main.go
```

### 4. 测试

```bash
# 健康检查
curl http://localhost:8888/health

# 创建用户
curl -X POST http://localhost:8888/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Charile","email":"charlie@example.com"}'

# 获取用户
curl http://localhost:8888/api/v1/users/1

# 查看 etcd 中的注册信息
etcdctl get /nexus-micro/services/ --prefix
```

## 多实例验证

启动两个 user-service 实例后，连续请求会轮流打到不同实例：

```bash
# 终端 1：实例 1
cd examples/etcd-demo/user-service && go run main.go

# 终端 2：实例 2（不同端口）
cd examples/etcd-demo/user-service && INSTANCE_PORT=8082 go run main.go
```

## 关键文件

```
etcd-demo/
├── docker-compose.yml              # etcd 3 节点集群
├── user-service/
│   ├── main.go                     # 服务入口（etcd 注册/注销）
│   ├── etc/user.yaml               # 服务配置
│   └── internal/
│       ├── config/config.go        # 配置加载
│       └── handler/handler.go      # 业务逻辑
└── gateway/
    ├── main.go                     # 网关入口（etcd 发现/长连接池）
    ├── etc/gateway.yaml            # 网关配置（路由 + 上游）
    └── internal/
        └── config/config.go        # 配置加载
```

## 核心代码路径

| 组件 | 文件 |
|------|------|
| etcd 注册中心 | [core/registry/etcd.go](../../core/registry/etcd.go) |
| 注册中心接口 | [core/registry/registry.go](../../core/registry/registry.go) |
| 长连接池客户端 | [core/client/pooled_client.go](../../core/client/pooled_client.go) |
| 负载均衡 | [core/balancer/balancer.go](../../core/balancer/balancer.go) |
