# Nexus Micro 快速开始 —— 开箱即用

本文档帮助你从零开始使用 Nexus Micro 框架创建微服务和 API 网关。

---

## 📋 前置要求

- Go 1.24+
- Git
- （可选）Docker + Docker Compose

---

## 🚀 第一步：安装 nx CLI 工具

```bash
# 克隆框架仓库
git clone https://github.com/xyxuliang/nexus-micro.git
cd nexus-micro

# 安装 nx CLI
go install ./cmd/nx/
```

安装完成后检查：
```bash
nx help
```

你应该能看到完整的帮助信息。

---

## 💡 核心概念

| 概念 | 说明 |
|------|------|
| **nx** | 命令行工具，用于创建项目和代码生成 |
| **.api DSL** | 领域特定语言，描述 API 接口 |
| **nx gen api** | 从 .api 文件自动生成服务代码 |
| **nx gen gateway** | 从 .api 文件自动生成网关代码 |
| **Vertical Slice** | 垂直切片架构，按业务模块组织代码 |
| **治理流水线** | 限流 → 降级 → 熔断 → 负载均衡 → 反向代理 |

---

## 🎯 创建第一个微服务

### 1. 创建服务骨架

```bash
# 创建名为 user 的微服务
nx new user
```

输出：
```
✓ Service user created

Directory structure:
  user/
  user/etc/
  user/internal/config/
  user/internal/handler/
  user/internal/logic/
  user/internal/svc/
  user/internal/middleware/
  user/internal/server/
  user/client/
  user/docs/
  user/etc/user.yaml

Next steps:
  1. Write your .api DSL in user/user.api
  2. Run: nx gen api user/user.api
  3. Run: cd user && go run main.go
```

### 2. 编写 .api DSL 文件

编辑 `user/user.api`：

```api
info {
    title "User Service"
    desc "用户管理服务"
    version "1.0.0"
}

// 定义数据类型
type (
    User {
        Id       int64
        Name     string
        Email    string
        Phone    string
        Avatar   string
        Created  int64
    }

    CreateUserReq {
        Name  string
        Email string
        Phone string
    }

    CreateUserResp {
        Id int64
    }

    GetUserReq {
        Id int64
    }

    GetUserResp {
        User User
    }

    ListUsersReq {
        Page  int
        Size  int
    }

    ListUsersResp {
        Users []User
        Total int64
    }
}

@server(
    prefix: /api/v1
    service: user
)

service UserService {
    @handler CreateUser
    @doc "创建用户"
    post /users

    @handler GetUser
    @doc "获取用户信息"
    get /users/:id

    @handler ListUsers
    @doc "获取用户列表"
    get /users
}
```

### 3. 生成完整服务代码

```bash
nx gen api user/user.api
```

生成后的目录结构：

```
user/
├── etc/
│   └── user.yaml              # 服务配置
├── internal/
│   ├── config/
│   │   └── config.go           # 配置结构体
│   ├── handler/
│   │   ├── create_user.go      # CreateUser handler
│   │   ├── get_user.go         # GetUser handler
│   │   ├── list_users.go       # ListUsers handler
│   │   └── routes.go           # 路由集中注册 ✨
│   ├── logic/
│   │   ├── create_user.go      # CreateUser 业务逻辑
│   │   ├── get_user.go         # GetUser 业务逻辑
│   │   └── list_users.go       # ListUsers 业务逻辑
│   ├── svc/
│   │   └── service_context.go  # 依赖注入容器
│   ├── middleware/
│   │   └── auth.go             # 自定义中间件（示例）
│   └── server/
│       ├── http.go             # HTTP 路由注册
│       └── grpc.go             # gRPC 路由注册（预留）
├── client/
│   └── http_client.go         # HTTP 客户端 SDK ✨
├── docs/
│   └── openapi.yaml           # OpenAPI v3 文档 ✨
├── user.pb                    # Protobuf 定义 ✨
├── main.go                    # 服务入口（完整中间件链）✨
└── user.api                   # 你编写的 DSL
```

### 4. 实现业务逻辑

打开 `user/internal/logic/create_user.go`，编写业务逻辑：

```go
func (l *CreateUserLogic) Execute() (interface{}, error) {
    // 1. 类型断言获取请求参数
    req, ok := req.(*dto.CreateUserReq)
    if !ok {
        return nil, fmt.Errorf("invalid request type")
    }

    // 2. 参数校验
    if req.Name == "" {
        return nil, errors.ErrBadRequest("name is required")
    }

    // 3. 插入数据库（假设已经在 svcCtx 初始化了 DB）
    id, err := l.svcCtx.DB.InsertUser(req.Name, req.Email, req.Phone)
    if err != nil {
        return nil, err
    }

    // 4. 返回结果
    return &dto.CreateUserResp{
        Id: id,
    }, nil
}
```

### 5. 启动服务

```bash
cd user
go run main.go
```

你会看到：
```
[user] starting on :8080
```

测试接口：
```bash
curl http://localhost:8080/health
```

返回：
```json
{
  "code": 0,
  "msg": "",
  "data": {
    "status": "ok",
    "service": "user",
    "version": "1.0.0",
    "time": "2025-01-01T12:00:00Z"
  },
  "request_id": "..."
}
```

🎉 恭喜！你的第一个微服务已经跑起来了！

---

## 🚪 创建 API 网关

Nexus Micro 自带完整的 API 网关实现，包含完整的治理流水线：

**限流 → 降级 → 熔断 → 服务发现 → 负载均衡 → 反向代理**

### 1. 创建网关项目

```bash
nx new gateway apigw
```

输出：
```
✓ Gateway apigw created

Directory structure:
  apigw/
  apigw/main.go               — 网关入口（完整治理流水线）
  apigw/etc/apigw.yaml        — 网关配置
  apigw/apigw.api            — 路由 DSL 模板
  apigw/internal/config/     — 配置结构体
```

### 2. 配置上游服务

编辑 `apigw/etc/apigw.yaml`，添加你的微服务路由：

```yaml
routes:
  - name: GetUser
    path: /users/:id
    method: GET
    prefix: /api/v1
    upstream: user-service
    rate: 100
    burst: 200
    timeout: 5s
  - name: CreateUser
    path: /users
    method: POST
    prefix: /api/v1
    upstream: user-service
    rate: 100
    burst: 200
    timeout: 5s

registry:
  provider: static
  static_endpoints:
    user-service:
      - localhost:8080   # 对应你的 user 服务地址
```

### 3. 从 DSL 生成网关代码

编辑 `apigw/apigw.api` 定义所有路由：

```api
info {
    title "API Gateway"
    desc "Nexus Micro API Gateway"
    version "1.0.0"
}

@server(
    prefix: "/api/v1"
    service: apigw
)

service ApigwGateway {
    @handler GetUser
    @doc "获取用户信息"
    get /users/:id

    @handler CreateUser
    @doc "创建用户"
    post /users
}
```

生成代码：
```bash
nx gen gateway apigw/apigw.api
```

### 4. 启动网关

```bash
cd apigw
go run main.go
```

网关监听 `:8888`，现在可以通过网关访问服务：

```bash
curl http://localhost:8888/api/v1/users/1
```

🎉 网关自动完成：
- ✅ 限流（按路由配置）
- ✅ CPU/内存过载保护
- ✅ 熔断（错误率过高自动熔断）
- ✅ 服务发现（从注册中心获取实例）
- ✅ 负载均衡（round-robin / least-connection / consistent-hash）
- ✅ 反向代理（转发到上游微服务）

---

## 🏢 创建多服务 Workspace（Monorepo）

如果你要开发多个微服务，可以创建一个 Workspace 来统一管理：

```bash
nx new workspace myproject
cd myproject
```

生成的结构：
```
myproject/
├── services/        # 所有微服务放这里
├── gateway/         # API 网关
├── shared/          # 共享库（类型定义、工具函数）
├── deploy/          # 部署配置（K8s YAML、Helm）
├── scripts/         # 自动化脚本
├── go.work          # Go workspace 文件
├── Makefile         # 常用命令
├── docker-compose.yml # 本地开发环境（PostgreSQL, Redis, etc...）
└── .gitignore
```

创建服务和网关到 workspace：

```bash
# 在 workspace 根目录
nx new services/user
nx new gateway gateway
```

构建所有服务：
```bash
make build
```

启动网关：
```bash
make run-gateway
```

启动单个服务：
```bash
make run-user
```

---

## 📐 架构说明

### 六层架构

```
┌─────────────────────────────────────────┐
│   CLI (nx) — 项目创建 + 代码生成        │
├─────────────────────────────────────────┤
│   Interface — HTTP + gRPC               │
├─────────────────────────────────────────┤
│   Governance — 限流 + 熔断 + 降级 + LB   │
├─────────────────────────────────────────┤
│   Pipeline — 中间件链                  │
├─────────────────────────────────────────┤
│   Transport — HTTP + gRPC 服务器       │
├─────────────────────────────────────────┤
│   Core — 配置、DI、上下文、注册发现     │
└─────────────────────────────────────────┘
```

### 治理流水线（网关）

```
请求
  │
  ├─→ 限流 (RateLimit)
  │    超过限流 → 拒绝请求 → 返回 429
  │
  ├─→ 降级 (Shedding)
  │    CPU/内存过载 → 拒绝请求 → 返回 503
  │
  ├─→ 熔断 (CircuitBreaker)
  │    上游错误率过高 → 快速失败
  │
  ├─→ 服务发现 (Discovery)
  │    获取上游实例列表
  │
  ├─→ 负载均衡 (Balancer)
  │    选择一个健康实例
  │
  ├─→ 反向代理 (Reverse Proxy)
  │    转发请求到上游
  │
  ↓
  响应
```

---

## ⚙️ 配置说明

### 微服务配置 (`etc/<service>.yaml`)

```yaml
# 服务器配置
server:
  name: user-service
  http:
    port: 8080
  grpc:
    port: 9090

# 服务治理
governance:
  ratelimit:
    rate: 1000      # 每秒允许 1000 请求
    burst: 2000     # 突发允许 2000
  circuitbreaker:
    window_size: 10s
    bucket_count: 10
    min_requests: 20
    error_rate: 0.5
    sleep_window: 30s
  shedding:
    cpu_threshold: 0.9
    mem_threshold: 0.85
  timeout: 30s

# 中间件
middleware:
  cors: true
  request_id: true
  tracing: true
  logger: true
  recovery: true
  metrics: true

# 注册中心
registry:
  provider: static  # static | etcd | consul | k8s
```

### 网关配置 (`etc/gateway.yaml`)

```yaml
server:
  name: my-gateway
  port: 8888

# 路由表
routes:
  - name: GetUser
    path: /users/:id
    method: GET
    prefix: /api/v1
    upstream: user-service
    rate: 100       # 该路由每秒限流
    burst: 200
    timeout: 5s

# 治理配置
ratelimit:
  global_rate: 10000    # 网关全局每秒限流
  global_burst: 20000

circuit_breaker:
  window_size: 10s
  bucket_count: 10
  min_requests: 20
  error_rate: 0.5
  sleep_window: 30s

shedding:
  cpu_threshold: 0.9
  mem_threshold: 0.85

balancer:
  strategy: round_robin  # round_robin | least_conn | consistent_hash

registry:
  provider: static
  static_endpoints:
    user-service:
      - localhost:8080
      - localhost:8081
```

---

## 🧩 可用 CLI 命令

### 项目创建

| 命令 | 说明 | 示例 |
|------|------|------|
| `nx new <name>` | 创建新微服务 | `nx new user` |
| `nx new gateway <name>` | 创建 API 网关 | `nx new gateway apigw` |
| `nx new workspace <name>` | 创建多服务 monorepo | `nx new workspace myapp` |

### 代码生成

| 命令 | 说明 | 示例 |
|------|------|------|
| `nx gen api <file.api>` | 从 .api 生成服务 | `nx gen api user.api` |
| `nx gen gateway <file.api>` | 从 .api 生成网关 | `nx gen gateway gateway.api` |
| `nx gen client <name>` | 生成客户端 SDK | `nx gen client user` |
| `nx gen doc` | 生成 OpenAPI | `nx gen doc` |

### Vertical Slice

| 命令 | 说明 | 示例 |
|------|------|------|
| `nx module <name>` | 创建业务模块 | `nx module user` |
| `nx slice <module> <name>` | 创建 Command 切片 | `nx slice user create` |
| `nx query <module> <name>` | 创建 Query 切片 | `nx query user get` |

### 开发

| 命令 | 说明 |
|------|------|
| `nx run` | 启动开发服务器 |
| `nx build` | 构建生产二进制 |
| `nx test` | 运行测试 |
| `nx lint` | 代码检查 |
| `nx help` | 显示帮助 |

---

## 🔧 开发流程（最佳实践）

### 1. 设计 API

```bash
# 1. 创建服务
nx new user

# 2. 编写 DSL
vim user/user.api

# 3. 生成代码
nx gen api user/user.api
```

### 2. 实现业务

1. 在 `internal/config/config.go` 添加配置字段
2. 在 `internal/svc/service_context.go` 添加依赖（DB, Redis, etc.）
3. 在 `internal/logic/xxx.go` 实现业务逻辑

### 3. 添加自定义中间件

在 `internal/middleware/auth.go` 添加：

```go
func AuthMiddleware() core.Middleware {
    return func(next core.Handler) core.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            // 你的认证逻辑...
            return next(ctx, req)
        }
    }
}
```

在 `main.go` 使用：

```go
chain := middleware.DefaultChain()
chain.Use(middleware.AuthMiddleware())
srv.WithMiddleware(chain...)
```

### 4. 调用其他服务

使用自动生成的客户端 SDK：

```go
import (
    "github.com/your/project/user/client"
)

// 在 logic 中调用
client := client.NewClient("http://user:8080")
resp, err := client.CreateUser(ctx, &req)
if err != nil {
    return nil, err
}
```

---

## 🛡️ 服务治理特性

### 限流（Rate Limit）

- **算法**: 令牌桶
- **多级限流**: 全局 → 服务 → 方法
- 默认配置：`rate=1000/sec, burst=2000`

### 熔断（Circuit Breaker）

- **三态模型**: Closed → Open → HalfOpen → Closed
- **滑动窗口**: 统计错误率
- **自适应**: 根据错误率自动切换状态
- 默认：错误率 > 50% 触发熔断，30 秒后进入半开探测

### 降级（Overload Shedding）

- **双层保护**: CPU + 内存同时监控
- 默认：CPU > 90% 或 内存 > 85% 触发降级
- 过载时直接返回 503，保护后端服务

### 负载均衡（Load Balancer）

四种策略可选：

| 策略 | 说明 | 使用场景 |
|------|------|----------|
| Round Robin | 轮询 | 一般场景 |
| Weighted Round Robin | 加权轮询 | 实例配置不同 |
| Least Connection | 最少连接 | 请求耗时差异大 |
| Consistent Hash | 一致性哈希 | 有状态服务 |

### 过载保护

Nexus Micro 从入口到网关都有过载保护：

1. **网关层**: CPU+内存 双层检查，过载直接拒绝
2. **服务端**: 同样有 shedding 保护
3. **客户端**: 熔断 + 超时控制

---

## 📊 统一响应格式

Nexus Micro 使用统一的 JSON 响应格式：

```json
{
  "code": 0,
  "msg": "",
  "data": { ... },
  "request_id": "abc123xyz"
}
```

| 字段 | 说明 |
|------|------|
| `code` | 业务状态码，**0 表示成功** |
| `msg` | 错误提示信息 |
| `data` | 响应数据 |
| `request_id` | 请求追踪 ID |

### 错误码分段

| 段 | 范围 | 对应 HTTP 状态 |
|----|------|----------------|
| 成功 | `0` | 200 OK |
| 参数错误 | `1000-1999` | 400 Bad Request |
| 认证授权 | `2000-2999` | 401/403 |
| 业务错误 | `3000-4999` | 200 OK |
| 系统错误 | `5000-5999` | 500 Internal Server Error |
| 治理拒绝 | `8000-8999` | 503 Service Unavailable |

---

## 🐳 Docker 部署

### 微服务 Dockerfile

```dockerfile
# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o service main.go

# Runtime stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/service .
COPY --from=builder /app/etc ./etc

EXPOSE 8080
CMD ["./service"]
```

### 网关 Dockerfile

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o gateway main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/gateway .
COPY --from=builder /app/gateway/etc ./etc

EXPOSE 8888
CMD ["./gateway"]
```

### docker-compose.yml

```yaml
version: "3.8"

services:
  gateway:
    build: ./gateway
    ports:
      - "8888:8888"
    depends_on:
      - user

  user:
    build: ./services/user
    ports:
      - "8080:8080"
    depends_on:
      - postgres

  postgres:
    image: postgres:18-alpine
    environment:
      POSTGRES_USER: nexus
      POSTGRES_PASSWORD: nexus123
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  pgdata:
```

---

## ❓ 常见问题

### Q: 如何添加数据库连接？

A: 在 `internal/svc/service_context.go` 添加：

```go
import "database/sql"
_ "github.com/lib/pq"

type ServiceContext struct {
    Config *config.Config
    DB     *sql.DB
}

func NewServiceContext(cfg *config.Config) *ServiceContext {
    db, err := sql.Open("postgres", cfg.Database.DSN)
    if err != nil {
        panic(err)
    }
    return &ServiceContext{
        Config: cfg,
        DB:     db,
    }
}
```

### Q: 支持 gRPC 吗？

A: 当前版本生成 HTTP 服务，gRPC 支持正在开发中。你可以在 DSL 中使用 `@grpc` 注解，代码生成器会预留接口。

### Q: 如何集成 Gin/Echo？

A: Nexus Micro 自带 HTTP 服务器，你不需要集成 Gin。框架已经实现了中间件链、路由注册、统一响应格式。

### Q: 零依赖是什么意思？

A: Nexus Micro 核心框架只使用 Go 标准库，不依赖任何第三方库。这意味着：
- 🚀 编译快
- 📦 体积小
- 🔒 没有供应链安全问题
- 📈 不会因为依赖过期而无法构建

---

## 📝 完整示例

创建一个简单的待办事项 API：

```bash
# 1. 创建服务
nx new todo

# 2. 编写 DSL
cat > todo/todo.api << 'EOF'
info {
    title "Todo Service"
    desc "Todo 管理服务"
    version "1.0.0"
}

type (
    Todo {
        Id        int64
        Title     string
        Completed bool
        CreatedAt int64
    }

    CreateTodoReq {
        Title string
    }

    CreateTodoResp {
        Id int64
    }

    ListTodosReq {}

    ListTodosResp {
        Todos []Todo
    }
}

@server(
    prefix: /api/v1
    service: todo
)

service TodoService {
    @handler CreateTodo
    @doc "创建待办"
    post /todos

    @handler ListTodos
    @doc "获取待办列表"
    get /todos
}
EOF

# 3. 生成代码
nx gen api todo/todo.api

# 4. 实现 CreateTodo 逻辑
# 编辑 todo/internal/logic/create_todo.go

# 5. 启动服务
cd todo
go run main.go

# 6. 测试
curl -X POST http://localhost:8080/api/v1/todos \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello Nexus Micro"}'
```

---

## 👉 下一步

- 阅读 [README.md](../README.md) 查看完整框架说明
- 查看 [设计文档](../../nexus-micro-v1.html) 了解设计思想
- 查看 [性能对比](../../nexus-micro-vs-gozero-perf.html) 对比性能数据

---

## 📞 问题反馈

如果你遇到问题，请提交 Issue。

Happy Coding! 🎉
