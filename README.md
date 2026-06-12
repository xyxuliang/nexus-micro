# Nexus Micro

> Lightweight, high-performance, DSL-driven Go microservice framework.  
> 比 Gin 更工程化，比 go-zero 更自由，比 Kratos 更简单。

**28 Go files · 4,781 lines · Zero external dependencies · Go 1.24+**

---

## 目录

1. [设计理念](#设计理念)
2. [架构](#架构)
3. [项目结构](#项目结构)
4. [技术栈](#技术栈)
5. [快速开始](#快速开始)
6. [CLI 命令](#cli-命令)
7. [中间件](#中间件)
8. [服务治理](#服务治理)
9. [统一响应格式](#统一响应格式)
10. [事件系统](#事件系统)
11. [抽象层](#抽象层)
12. [与其他框架对比](#与其他框架对比)
13. [开发路线](#开发路线)
14. [本地开发](#本地开发)

---

## 设计理念

```text
Business First          — 目录围绕业务组织，不是围绕技术组织
Vertical Slice          — 一个功能 = 一个完整切片（command / handler / validator / dto / event）
Monolith First          — 单体起步，按需拆分为微服务，中间件逻辑只写一次
Docker First            — 不强制 Kubernetes，docker compose up 一键启动全部基础设施
Convention > Configuration — 减少样板代码，降低新成员上手成本
AI Friendly             — 结构清晰、上下文完整，Claude / Copilot 友好
```

**六大设计原则：**

| # | 原则 | 说明 |
|---|------|------|
| 01 | **DSL 优先** | 服务接口用 `.api` 文件描述，框架自动生成服务端骨架、客户端 SDK、OpenAPI 文档和 gRPC 定义 |
| 02 | **内核轻量** | 内核只包含 DI 容器、配置管理、生命周期、RPC 通信和基础治理。ORM / 缓存 / 队列由业务方自行集成 |
| 03 | **治理内置** | 服务发现、负载均衡、自适应熔断、令牌桶限流、CPU+内存双层过载保护——启动即拥有 |
| 04 | **协议无关** | HTTP REST 和 gRPC 共享同一套服务定义和中间件管道，业务代码只写一次 |
| 05 | **单体优先** | 支持单体 → 模块化单体 → 微服务的渐进式演进 |
| 06 | **约定优于配置** | 默认目录结构、默认命名规范、默认中间件管道 |

---

## 架构

```text
┌─────────────────────────────────────────────────┐
│                  CLI 工具层                      │
│         nx new / gen / module / slice / run      │
├─────────────────────────────────────────────────┤
│  接口层         │  HTTP Gateway (REST+OpenAPI)   │
│                 │  gRPC Server (Protobuf)        │
├─────────────────────────────────────────────────┤
│  服务治理层     │  Registry  │  LoadBalancer     │
│                 │  CircuitBreaker  │  RateLimiter│
│                 │  Shedding  │  Timeout  │  Retry│
├─────────────────────────────────────────────────┤
│  中间件管道     │  RequestID → Tracing → Logger  │
│  (HTTP+gRPC     │  → Recovery → CORS → Auth      │
│   完全共享)     │  → RateLimit → Timeout         │
│                 │  → Shedding → Metrics           │
├─────────────────────────────────────────────────┤
│  传输层         │  RPC Client  │  Serializer     │
├─────────────────────────────────────────────────┤
│  内核           │  DI Container  │  Config        │
│                 │  Lifecycle  │  Health Check     │
└─────────────────────────────────────────────────┘
```

---

## 项目结构

```
nexus-micro/
├── cmd/nx/                    # CLI 工具入口（12 个命令）
├── core/
│   ├── types.go               # 核心类型（Server / Handler / Middleware / MiddlewareChain）
│   ├── balancer/              # 客户端负载均衡（4 种策略）
│   ├── circuitbreaker/        # 自适应熔断器（三态 + 滑动窗口）
│   ├── client/                # RPC 客户端（服务发现 + LB + 熔断 + 重试）
│   ├── config/                # 配置管理（多源 + 热更新 + 默认值）
│   ├── context/               # 统一请求上下文（RequestID / TraceID / UserID / TenantID / Language）
│   ├── di/                    # 依赖注入容器（拓扑排序 + 循环依赖检测）
│   ├── lifecycle/             # 生命周期管理（原子状态机 + 信号监听 + 钩子）
│   ├── middleware/             # 内置中间件（7 个，HTTP/gRPC 共享）
│   ├── ratelimit/             # 令牌桶限流（全局 / 服务 / 方法三级）
│   ├── registry/              # 服务注册发现（Static / etcd / Consul / K8s DNS 四级）
│   ├── response/              # 统一响应格式（{code, msg, data, request_id}）
│   ├── server/                # 核心 Server（HTTP/gRPC 混合 + 健康检查）
│   └── shedding/              # 过载保护（CPU + 内存双层）
├── dsl/
│   ├── ast/                   # 抽象语法树定义
│   ├── lexer/                 # 词法分析器（手工实现，零依赖）
│   └── parser/                # 语法分析器（递归下降）
├── generator/                 # 代码生成器（11 种生成物）
├── event/                     # 领域事件系统（发布/订阅 + 内存总线）
├── cache/                     # 三级缓存抽象接口（Memory → Redis → Database）
├── storage/                   # 对象存储抽象接口（S3 兼容）
├── queue/                     # 异步任务队列抽象接口（延迟 / 优先级 / 重试）
├── internal/
│   ├── errors/                # 错误码体系（8 段，0-8999）
│   ├── reflect/               # 反射工具 + 结构体校验
│   └── util/                  # 通用工具（ID 生成 / 大小写转换 / 类型映射）
├── nexus.go                   # 框架统一入口
├── user.api                   # 示例 DSL 文件
├── configs/
│   ├── config.yaml            # 服务配置示例
│   └── prometheus.yml         # Prometheus 抓取配置
├── docker-compose.yml         # 本地开发基础设施（PostgreSQL + Redis + NATS + MinIO + Jaeger + Grafana + Prometheus）
├── go.mod                     # 零外部依赖
└── README.md
```

---

## 技术栈

| 组件 | 选型 | 框架态度 | 实现状态 |
|------|------|----------|----------|
| HTTP 路由 | Gin（后续） | 内置 | 当前使用 `net/http`（零依赖） |
| RPC | Connect RPC（后续） | 内置 | 接口已定义 |
| 服务发现 | Static / etcd / Consul / K8s DNS | 内置 | Static 已实现 |
| 负载均衡 | 轮询 / 加权轮询 / 最少连接 / 一致性哈希 | 四种策略内置 | ✅ 全部实现 |
| 熔断器 | 自适应三态滑动窗口 | 内置 | ✅ 已实现 |
| 限流 | 令牌桶 + 多级限流 | 内置 | ✅ 已实现 |
| 过载保护 | CPU + 内存双层 | 内置 | ✅ 已实现 |
| 配置管理 | 自研 | 内置 | ✅ 已实现（多源 + 默认值 + ENV 覆盖） |
| 日志 | Zerolog（后续） | 内置 | 当前使用 `log`（零依赖） |
| 链路追踪 | OpenTelemetry（后续） | 内置 | 预留 trace_id 注入 |
| 监控 | Prometheus（后续） | 内置 | 预留 `/metrics` 端点 |
| 消息队列 | NATS | 推荐 | 接口已定义 |
| 数据库 | PostgreSQL 18+ | 推荐 | 不强制 |
| 缓存 | Redis | 推荐 | 三级缓存接口已定义 |
| 对象存储 | MinIO (S3 兼容) | 推荐 | 接口已定义 |
| 任务队列 | Asynq | 推荐 | 接口已定义 |
| ORM | 无内置（可选 GORM） | 不绑架 | 不强制 |

> **设计决策：** 框架内核保持零外部依赖，所有组件通过统一接口接入。当前阶段使用标准库实现，后续版本逐步接入 Gin、Connect RPC、Zerolog、OpenTelemetry 等社区标准库，但接口不变，业务代码无需修改。

---

## 快速开始

### 1. 安装 CLI

```bash
go install github.com/xyxuliang/nexus-micro/cmd/nx@latest
```

### 2. 创建新服务

```bash
nx new myapp
cd myapp
```

### 3. 定义 DSL

```api
// user.api
syntax = "v1"

info (
    title:   "User Service"
    desc:    "用户管理"
    version: "v1.0.0"
)

type (
    User {
        Id       string `json:"id"`
        Name     string `json:"name"`
        Email    string `json:"email" validate:"required,email"`
    }
    CreateUserReq {
        Name  string `json:"name" validate:"required"`
        Email string `json:"email" validate:"required,email"`
    }
    CreateUserResp {
        Id string `json:"id"`
    }
)

@server (
    prefix: /api/v1
    service: user
)
service UserService {
    @handler CreateUser
    @doc "创建用户"
    @grpc
    post /users (CreateUserReq) returns (CreateUserResp)
}
```

### 4. 生成代码

```bash
nx gen api user.api
```

自动生成 11 种产物：

| 生成物 | 说明 |
|--------|------|
| `internal/config/config.go` | 配置结构体 |
| `internal/handler/*.go` | HTTP + gRPC Handler |
| `internal/logic/*.go` | 业务逻辑骨架（**填充你的实现**） |
| `internal/svc/service_context.go` | 依赖注入上下文 |
| `internal/middleware/auth.go` | 自定义中间件 |
| `internal/server/http.go` | HTTP 路由注册 |
| `internal/server/grpc.go` | gRPC 服务注册 |
| `client/http_client.go` | 客户端 SDK |
| `docs/openapi.yaml` | OpenAPI v3 文档 |
| `*.proto` | Protobuf 定义 |
| `main.go` | 入口文件 |

### 5. 启动

```bash
nx run
# 或
go run main.go
```

### 6. 用代码写一个服务

```go
package main

import (
    "github.com/xyxuliang/nexus-micro"
)

func main() {
    srv := nexus.NewServer(
        nexus.WithName("user-service"),
        nexus.WithConfig("etc/config.yaml"),
        nexus.WithHTTP(":8080"),
        nexus.WithGRPC(":9090"),
    )

    srv.Start()
}
```

---

## CLI 命令

| 命令 | 说明 | 状态 |
|------|------|------|
| `nx new <name>` | 创建新服务 | ✅ |
| `nx gen api <file.api>` | 从 .api 生成完整服务 | ✅ |
| `nx gen proto <file.proto>` | 从 .proto 生成服务 | 🚧 |
| `nx gen client <name>` | 生成客户端 SDK | 🚧 |
| `nx gen doc` | 生成 OpenAPI 文档 | 🚧 |
| `nx module <name>` | 创建业务模块（Vertical Slice） | ✅ |
| `nx slice <module> <name>` | 创建 Command 切片 | ✅ |
| `nx query <module> <name>` | 创建 Query 切片 | ✅ |
| `nx run` | 启动开发服务器 | 🚧 |
| `nx build` | 构建生产二进制 | 🚧 |
| `nx test` | 运行测试 | 🚧 |
| `nx lint` | 代码检查 | 🚧 |

---

## 中间件

所有中间件在 HTTP 和 gRPC 协议之间**完全共享**，只需定义一次。

| # | 中间件 | 功能 |
|---|--------|------|
| 1 | **RequestID** | 为每个请求注入唯一 `request_id`，支持从请求头复用 |
| 2 | **Tracing** | 注入 `trace_id`，为 OpenTelemetry 铺路 |
| 3 | **Logger** | 记录请求耗时、错误信息 |
| 4 | **Recovery** | 捕获 panic，记录堆栈，防止服务崩溃 |
| 5 | **CORS** | 跨域请求处理 |
| 6 | **Timeout** | 请求超时控制（可配置） |
| 7 | **Metrics** | 指标采集（QPS / 延迟 / 状态码） |

**自定义中间件：**

```go
srv := nexus.NewServer(
    nexus.WithMiddleware(
        middleware.RequestID(),
        middleware.Tracing(),
        middleware.Logger(),
        middleware.Recovery(),
        middleware.CORS(),
        // 你的自定义中间件
        func(next core.Handler) core.Handler {
            return func(ctx context.Context, req interface{}) (interface{}, error) {
                // 前置处理
                resp, err := next(ctx, req)
                // 后置处理
                return resp, err
            }
        },
    ),
)
```

---

## 服务治理

所有治理组件通过 `core.Server` 的函数式选项注入，启动即拥有。治理组件独立于业务代码，可单独测试。

### 服务发现

```go
// 四级注册中心，配置切换仅需一行
registry := discovery.New(
    discovery.WithStatic(map[string][]string{
        "user": {"localhost:8081", "localhost:8082"},
    }),
    // discovery.WithEtcd("localhost:2379"),
    // discovery.WithConsul("localhost:8500"),
    // discovery.WithK8s(),
)
```

### 负载均衡

```go
lb := balancer.New(&balancer.Config{
    Strategy: balancer.RoundRobin,       // 轮询（默认）
    // Strategy: balancer.WeightedRoundRobin, // 加权轮询
    // Strategy: balancer.LeastConnection,    // 最少连接
    // Strategy: balancer.ConsistentHash,     // 一致性哈希
})
```

### 自适应熔断

```go
cb := circuitbreaker.New(&circuitbreaker.Config{
    WindowSize:   10 * time.Second,   // 滑动窗口
    BucketCount:  10,                 // 10 个桶
    MinRequests:  20,                 // 最小请求数
    ErrorRate:    0.5,                // 错误率阈值 50%
    SlowThreshold: 500 * time.Millisecond, // 慢请求阈值
    SleepWindow:  30 * time.Second,   // 熔断休眠
    HalfOpenMax:  3,                  // 半开状态最大试探
})
```

### 令牌桶限流

```go
limiter := ratelimit.NewMultiLevel(1000, 2000) // 全局 1000 req/s
limiter.AddService("user", 500, 1000)           // 服务级 500 req/s
limiter.AddMethod("user", "CreateUser", 100, 200) // 方法级 100 req/s
```

### 过载保护

```go
shedder := shedding.New(&shedding.Config{
    CPUThreshold: 0.9,   // CPU 90% 触发保护
    MemThreshold: 0.85,  // 内存 85% 触发保护
})
```

### RPC 客户端

```go
client := client.New("user-service",
    client.WithRegistry(reg),
    client.WithBalancer(lb),
    client.WithTimeout(3 * time.Second),
    client.WithRetries(3, 100 * time.Millisecond),
    client.WithCircuitBreaker(nil), // 使用默认配置
)

// 一行调用，自动处理：发现 → LB → 熔断 → 超时 → 重试 → 链路追踪
resp, err := client.Call(ctx, "CreateUser", req)
```

---

## 统一响应格式

所有 HTTP 接口强制使用三段式结构，由框架自动封装，开发者**零手动构建**。

```json
// 成功
{"code": 0, "msg": "success", "data": { ... }, "request_id": "..."}

// 分页
{"code": 0, "msg": "success", "data": {"items": [...], "total": 100, "page": 1, "page_size": 20}, "request_id": "..."}

// 错误
{"code": 1003, "msg": "资源不存在", "request_id": "..."}

// 参数校验错误
{"code": 1001, "msg": "参数校验失败", "data": {"errors": [{"field": "email", "message": "邮箱格式不正确"}]}, "request_id": "..."}
```

**错误码分段：**

| 范围 | 分类 | 示例 |
|------|------|------|
| 0 | 成功 | — |
| 1000-1999 | 通用错误 | 参数校验(1001)、资源不存在(1003) |
| 2000-2999 | 认证授权 | 未登录(2001)、Token 过期(2002)、权限不足(2003) |
| 3000-3999 | 用户模块 | 邮箱已注册(3001)、用户不存在(3003) |
| 4000-4999 | 订单模块 | 订单不存在(4001) |
| 5000-5999 | 支付模块 | 支付失败(5001) |
| 6000-6999 | 存储模块 | 文件上传失败(6001) |
| 7000-7999 | 通知模块 | 发送失败(7001) |
| 8000-8999 | 系统错误 | 内部错误(8000)、熔断拒绝(8002)、限流拒绝(8003) |

**Handler 中零手动封装：**

```go
// 业务代码只需返回 (data, error)，框架自动封装
func (h *UserHandler) CreateUser(ctx context.Context, req *CreateUserReq) (*CreateUserResp, error) {
    user, err := h.svc.CreateUser(ctx, req)
    if err != nil {
        return nil, errors.NewCode(3001, "邮箱已注册")  // 框架自动转为 {"code": 3001, "msg": "邮箱已注册"}
    }
    return &CreateUserResp{Id: user.ID}, nil  // 框架自动封装为 {"code": 0, "msg": "success", "data": {...}}
}
```

---

## 事件系统

```go
// 发布事件
event.Publish(ctx, bus, &UserCreatedEvent{
    UserID: user.ID,
    Email:  user.Email,
})

// 订阅事件
event.Subscribe(ctx, bus, "user.created", func(ctx context.Context, evt Event) error {
    return emailService.SendWelcome(evt.(*UserCreatedEvent).Email)
})
```

Topic 命名约定：`{domain}.{verb}`（如 `user.created`、`order.paid`、`payment.success`）。

---

## 抽象层

框架提供缓存、存储、队列的统一接口，但不强制绑定实现。业务方可以选择默认实现，也可以完全替换。

```go
// 三级缓存：L1 内存 → L2 Redis → L3 数据库
cache := cache.NewMultiLevel(memoryCache, redisCache)
cache.Get(ctx, "user:123")

// 对象存储：MinIO / S3 / OSS / COS
storage := storage.NewMinIO(storage.Config{Endpoint: "localhost:9000", Bucket: "nexus"})
storage.Upload(ctx, "avatars/user-123.png", reader)

// 任务队列：Asynq
task := queue.NewTask("send_welcome_email", payload).WithMaxRetry(3).WithPriority(10)
queue.Dispatch(ctx, task)
```

---

## 与其他框架对比

| 特性 | Nexus Micro | go-zero | Kratos |
|------|-------------|---------|--------|
| 定位 | 微服务运行时 | 全家桶应用平台 | 微服务框架 |
| 设计哲学 | 内核轻量，不绑架 | 全家桶，拿来即用 | 分层架构，DDD 强制 |
| DSL | .api（自研） | .api（自研） | .proto（Protobuf） |
| HTTP + gRPC 混合 | ✅ 同进程共享管道 | ❌ 独立服务 | ✅ 同进程 |
| 服务发现 | Static / etcd / Consul / K8s DNS | 强绑 etcd | etcd / Consul / Nacos |
| 零依赖单体模式 | ✅ 纯 stdlib 即可启动 | ❌ 需要 etcd | ❌ 需要注册中心 |
| 内置 ORM | ❌ 不绑架 | ✅ sqlx-based | ❌ |
| 内置缓存抽象 | ✅ 三级缓存接口 | ✅ 内置实现 | ❌ |
| 内置事件系统 | ✅ 发布/订阅 | ❌ | ❌ |
| 内置存储抽象 | ✅ S3 兼容接口 | ❌ | ❌ |
| 内置队列抽象 | ✅ 延迟/优先级/重试 | ❌ | ❌ |
| Vertical Slice | ✅ 推荐实践 + CLI 生成 | ❌ | ❌ |
| 渐进式演进 | ✅ 单体 → 微服务 | ❌ 微服务起步 | ❌ 微服务起步 |
| 依赖注入 | 拓扑排序 + 循环检测 | 无 | Wire（编译时） |
| 学习曲线 | 低 | 中 | 高 |

---

## 开发路线

| 阶段 | 内容 | 状态 |
|------|------|------|
| **Phase 1** | 内核（DI + Config + Lifecycle）+ DSL（Lexer + Parser + AST）+ 代码生成器 + CLI + 基础中间件 | ✅ 完成 |
| **Phase 2** | 服务治理（发现/负载均衡/熔断/限流/过载保护）+ RPC Client + 统一 Context | ✅ 完成 |
| **Phase 3** | 领域事件系统 + 缓存/存储/队列抽象接口 + 统一响应格式 + 错误码体系 | ✅ 完成 |
| **Phase 4** | 可观测性：OpenTelemetry 集成 + Prometheus 指标 + Grafana Dashboard | 🔜 规划中 |
| **Phase 5** | 部署：Docker 多阶段构建 + K8s Deployment + Helm Chart | 🔜 规划中 |
| **Phase 6** | 生态：Gin 接入 + Connect RPC 接入 + NATS/Asynq/MinIO 默认实现 + 多语言 SDK | 🔜 规划中 |

---

## 本地开发

```bash
# 克隆
git clone https://github.com/xyxuliang/nexus-micro.git
cd nexus-micro

# 编译 CLI
go build -o nx ./cmd/nx
mv nx /usr/local/bin/

# 启动基础设施
docker compose up -d
# 会启动：PostgreSQL + Redis + NATS + MinIO + Jaeger + Grafana + Prometheus

# 创建新项目
cd /tmp
nx new demo
cd demo
nx gen api ../nexus-micro/user.api
nx run
```

---

## License

MIT