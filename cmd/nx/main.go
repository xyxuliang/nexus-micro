// Nexus Micro — Next Generation Golang Microservice Framework
// CLI 工具入口，提供项目创建、代码生成、开发运行等命令。
//
// 使用方式：
//
//	nx new <service-name>          创建新服务
//	nx new gateway <name>          创建 API 网关
//	nx new workspace <name>        创建多服务 workspace
//	nx gen api <file.api>          从 .api 生成完整服务
//	nx gen gateway <file.api>      从 .api 生成 API 网关
//	nx gen proto <file.proto>      从 .proto 生成完整服务
//	nx gen client <service-name>   生成客户端 SDK
//	nx gen doc                     生成 OpenAPI 文档
//	nx module <name>               创建业务模块 (Vertical Slice)
//	nx slice <module> <name>       创建 Command 切片
//	nx query <module> <name>       创建 Query 切片
//	nx run                         启动开发服务器
//	nx build                       构建生产二进制
//	nx test                        运行测试
//	nx lint                        代码检查
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xyxuliang/nexus-micro/generator"
)

// Version 是 CLI 版本号。
const Version = "1.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "new":
		cmdNew(os.Args[2:])
	case "gen":
		cmdGen(os.Args[2:])
	case "module":
		cmdModule(os.Args[2:])
	case "slice":
		cmdSlice(os.Args[2:])
	case "query":
		cmdQuery(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "build":
		cmdBuild(os.Args[2:])
	case "test":
		cmdTest(os.Args[2:])
	case "lint":
		cmdLint(os.Args[2:])
	case "rpc":
		cmdRPC(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("nx version %s\n", Version)
	case "help", "-h", "--help":
		if len(os.Args) > 2 {
			printCommandHelp(os.Args[2])
		} else {
			printUsage()
		}
	default:
		fmt.Fprintf(os.Stderr, "nx: unknown command %q\n", command)
		fmt.Fprintf(os.Stderr, "Run 'nx help' for usage.\n")
		os.Exit(1)
	}
}

// printUsage 打印 CLI 使用帮助。
func printUsage() {
	fmt.Println(`Nexus Micro CLI — Next Generation Golang Microservice Framework

Usage:
  nx <command> [arguments]

Project Commands:
  new <service-name>         创建新微服务
  new gateway <name>         创建 API 网关项目
  new workspace <name>       创建多服务 workspace（monorepo）
  new rpc <service-name>     创建 gRPC 微服务
  new rpc-gateway <name>     创建 HTTP+gRPC 双协议网关
  new rpc-workspace <name>   创建 RPC 多服务工作区

Generate Commands:
  gen api <file.api>         从 .api DSL 生成完整服务代码
  gen gateway <file.api>     从 .api DSL 生成 API 网关代码
  gen proto <file.proto>     从 .proto 生成 gRPC stub
  gen client <service-name>  生成客户端 SDK
  gen doc                    生成 OpenAPI 文档

RPC Commands:
  rpc proto <file.proto>     编译 .proto 生成 gRPC 代码
  rpc service <name>         生成 gRPC 服务实现骨架（从 .api）

Module Commands:
  module <name>              创建业务模块 (Vertical Slice)
  slice <module> <name>      创建 Command 切片
  query <module> <name>      创建 Query 切片

Dev Commands:
  run                        启动开发服务器
  build                      构建生产二进制
  test                       运行测试
  lint                       代码检查

Other:
  version                    显示版本
  help [command]             显示帮助（支持子命令帮助）

Examples:
  nx new mysvc                           # 创建 mysvc 微服务
  nx new gateway apigw                   # 创建 apigw 网关
  nx new workspace myapp                 # 创建多服务 monorepo
  nx new rpc user-service                # 创建 gRPC 用户服务
  nx new rpc-gateway apigw               # 创建 HTTP+gRPC 网关
  nx gen api user.api                    # 从 user.api 生成代码
  nx gen proto user.proto                # 从 .proto 生成 gRPC stub
  nx rpc proto user.proto                # 编译 .proto（等同于 gen proto）
  nx module user                         # 创建 user 模块
  nx run                                 # 启动开发服务器
  nx help new                            # 查看 new 命令帮助`)
}

// printCommandHelp 打印子命令帮助。
func printCommandHelp(cmd string) {
	switch cmd {
	case "new":
		fmt.Println(`Usage: nx new <service-name>
       nx new gateway <name>
       nx new workspace <name>
       nx new rpc <name>
       nx new rpc-gateway <name>
       nx new rpc-workspace <name>

Create a new project:
  new <name>             Create a new microservice
  new gateway <name>     Create an API gateway project
  new workspace <name>   Create a multi-service workspace (monorepo)
  new rpc <name>         Create a gRPC microservice
  new rpc-gateway <name> Create an HTTP+gRPC dual-protocol gateway
  new rpc-workspace <name> Create an RPC multi-service workspace

Examples:
  nx new user-service
  nx new gateway apigw
  nx new workspace myapp
  nx new rpc user-service
  nx new rpc-gateway apigw`)
	case "gen":
		fmt.Println(`Usage: nx gen <subcommand> [arguments]

Generate code from definition files:
  gen api <file.api>       Generate full service code from .api DSL
  gen gateway <file.api>   Generate API gateway code from .api DSL
  gen proto <file.proto>   Generate service code from .proto
  gen client <name>        Generate client SDK for a service
  gen doc                  Generate OpenAPI documentation

Examples:
  nx gen api user.api
  nx gen client user-service`)
	case "module":
		fmt.Println(`Usage: nx module <name>

Create a new business module (Vertical Slice architecture):
  module <name>    Create domain/ and internal/ directories

Example:
  nx module user`)
	case "slice":
		fmt.Println(`Usage: nx slice <module> <name>

Create a Command slice (write operation):
  slice <module> <name>    Create command, handler, validator, dto, mapper, event files

Example:
  nx slice user register`)
	case "query":
		fmt.Println(`Usage: nx query <module> <name>

Create a Query slice (read operation):
  query <module> <name>    Create query, handler, dto files

Example:
  nx query user profile`)
	case "run":
		fmt.Println(`Usage: nx run

Start the development server. Detects main.go in current directory
and runs it with go run. Supports passing build tags and ldflags.`)
	case "build":
		fmt.Println(`Usage: nx build [output]

Build a production binary. Default output is bin/<service-name>.
Supports cross-compilation via GOOS/GOARCH environment variables.`)
	case "test":
		fmt.Println(`Usage: nx test [packages]

Run tests. Defaults to ./... if no packages specified.
Passes through all standard go test flags.`)
	case "lint":
		fmt.Println(`Usage: nx lint [packages]

Run code checks. Defaults to ./... if no packages specified.
Runs go vet for static analysis.`)
	default:
		fmt.Fprintf(os.Stderr, "nx: no help available for %q\n", cmd)
		fmt.Fprintf(os.Stderr, "Run 'nx help' for available commands.\n")
	}
}

// cmdNew 创建新项目。
// 支持子命令：new service（默认）、new gateway、new workspace。
func cmdNew(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx new: missing argument")
		fmt.Fprintln(os.Stderr, "Usage: nx new <service-name>")
		fmt.Fprintln(os.Stderr, "       nx new gateway <name>")
		fmt.Fprintln(os.Stderr, "       nx new workspace <name>")
		os.Exit(1)
	}

	// 处理子命令
	if args[0] == "gateway" {
		cmdNewGateway(args[1:])
		return
	}
	if args[0] == "workspace" {
		cmdNewWorkspace(args[1:])
		return
	}
	if args[0] == "rpc" {
		cmdNewRpcService(args[1:])
		return
	}
	if args[0] == "rpc-gateway" {
		cmdNewRpcGateway(args[1:])
		return
	}
	if args[0] == "rpc-workspace" {
		cmdNewRpcWorkspace(args[1:])
		return
	}

	// 默认：创建微服务
	cmdNewService(args)
}

// cmdNewService 创建新微服务。
func cmdNewService(args []string) {
	serviceName := args[0]

	// 服务目录结构
	dirs := []string{
		serviceName,
		serviceName + "/etc",
		serviceName + "/internal/config",
		serviceName + "/internal/handler",
		serviceName + "/internal/logic",
		serviceName + "/internal/svc",
		serviceName + "/internal/middleware",
		serviceName + "/internal/server",
		serviceName + "/client",
		serviceName + "/docs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "nx new: failed to create %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// 生成默认配置文件
	configYAML := fmt.Sprintf(`# %s 服务配置
server:
  name: %s
  http:
    port: 8080
  grpc:
    port: 9090

# 服务治理配置
governance:
  ratelimit:
    rate: 1000       # 每秒请求数
    burst: 2000      # 突发请求数
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

# 中间件配置
middleware:
  cors: true
  request_id: true
  tracing: true
  logger: true
  recovery: true
  metrics: true

# 注册中心配置
registry:
  provider: static    # static | etcd | consul | k8s
  static_endpoints:
    - localhost:8080
  etcd_endpoints:
    - localhost:2379
  consul_addr: localhost:8500

# 日志配置
log:
  level: info        # debug | info | warn | error
  format: json       # json | text
`, serviceName, serviceName)

	configPath := serviceName + "/etc/" + serviceName + ".yaml"
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "nx new: failed to write config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service %s created\n", serviceName)
	fmt.Println("\nDirectory structure:")
	for _, dir := range dirs {
		fmt.Printf("  %s/\n", dir)
	}
	fmt.Printf("  %s\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Write your .api DSL in %s/%s.api\n", serviceName, serviceName)
	fmt.Printf("  2. Run: nx gen api %s/%s.api\n", serviceName, serviceName)
	fmt.Printf("  3. Run: cd %s && go run main.go\n", serviceName)
}

// cmdNewGateway 创建 API 网关项目。
func cmdNewGateway(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx new gateway: missing gateway name")
		fmt.Fprintln(os.Stderr, "Usage: nx new gateway <name>")
		os.Exit(1)
	}

	name := args[0]
	modulePath := "github.com/xyxuliang/nexus-micro"

	dirs := []string{
		name,
		name + "/etc",
		name + "/internal/config",
		name + "/internal/handler",
		name + "/internal/middleware",
		name + "/internal/svc",
		name + "/docs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "nx new gateway: failed to create %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// 生成网关主入口（含完整治理流水线）
	mainGo := fmt.Sprintf(`// %s — Nexus Micro API Gateway
// 提供统一的 API 入口、路由转发、服务治理和可观测性。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"%s/core"
	"%s/core/balancer"
	"%s/core/circuitbreaker"
	"%s/core/middleware"
	"%s/core/ratelimit"
	"%s/core/registry"
	"%s/core/response"
	"%s/core/shedding"
	"%s/gateway/%s/internal/config"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("etc/%s.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %%v", err)
	}

	// 2. 初始化治理组件
	// 限流器 — 全局 + 每路由
	limiter := ratelimit.NewMultiLevel(
		cfg.RateLimit.GlobalRate,
		cfg.RateLimit.GlobalBurst,
	)
	for _, route := range cfg.Routes {
		limiter.AddService(route.Name, route.Rate, route.Burst)
	}

	// 降级器 — CPU + 内存双层保护
	shedder := shedding.New(&shedding.Config{
		CPUThreshold: cfg.Shedding.CPUThreshold,
		MemThreshold: cfg.Shedding.MemThreshold,
		Window:       cfg.Shedding.Window,
	})

	// 负载均衡器 — 多策略（round_robin | least_conn | consistent_hash）
	lb := balancer.New(&balancer.Config{
		Strategy: cfg.Balancer.Strategy,
	})

	// 注册中心 — 服务发现
	reg := registry.NewStaticRegistry(cfg.Registry.StaticEndpoints)

	// 3. 构建中间件链
	chain := middleware.DefaultChain()
	chain.Use(middleware.Timeout(cfg.Timeout))
	chain.Use(middleware.Metrics())

	// 4. 创建 HTTP 路由
	mux := http.NewServeMux()

	// 健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		response.Success(map[string]interface{}{
			"status":  "ok",
			"service": "%s-gateway",
			"time":    time.Now().Format(time.RFC3339),
		}).WithRequestID(middleware.GetRequestID(r.Context())).WriteTo(w)
	})

	// 就绪检查端点
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "ready")
	})

	// 5. 为每条路由注册代理处理器
	proxyHandler := NewProxyHandler(cfg, limiter, shedder, lb, reg, chain)
	for _, route := range cfg.Routes {
		pattern := route.Prefix + route.Path
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			proxyHandler.ServeHTTP(w, r)
		})
	}

	// 6. 启动 HTTP 服务器
	addr := fmt.Sprintf(":%%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
		IdleTimeout:  120 * time.Second,
	}

	// 7. 优雅关闭
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("[gateway] shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("[gateway] shutdown error: %%v", err)
		}
	}()

	log.Printf("[gateway] %%s gateway starting on %%s", cfg.Server.Name, addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[gateway] server error: %%v", err)
	}
	log.Println("[gateway] server stopped")
}

// ProxyHandler 是 API 网关的代理处理器。
// 集成限流 → 降级 → 负载均衡 → 熔断 → 反向代理的完整治理流水线。
type ProxyHandler struct {
	config    *config.Config
	limiter   *ratelimit.MultiLevel
	shedder   shedding.Shedder
	lb        balancer.LoadBalancer
	registry  registry.Registry
	chain     *core.MiddlewareChain
	breaker   *circuitbreaker.AdaptiveCB
}

// NewProxyHandler 创建代理处理器。
func NewProxyHandler(
	cfg *config.Config,
	limiter *ratelimit.MultiLevel,
	shedder shedding.Shedder,
	lb balancer.LoadBalancer,
	reg registry.Registry,
	chain *core.MiddlewareChain,
) *ProxyHandler {
	return &ProxyHandler{
		config:   cfg,
		limiter:  limiter,
		shedder:  shedder,
		lb:       lb,
		registry: reg,
		chain:    chain,
		breaker: circuitbreaker.New(&circuitbreaker.Config{
			WindowSize:   cfg.CircuitBreaker.WindowSize,
			BucketCount:  cfg.CircuitBreaker.BucketCount,
			MinRequests:  cfg.CircuitBreaker.MinRequests,
			ErrorRate:    cfg.CircuitBreaker.ErrorRate,
			SleepWindow:  cfg.CircuitBreaker.SleepWindow,
		}),
	}
}

// ServeHTTP 处理 HTTP 请求（治理流水线）。
func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := r.URL.Path

	// 查找匹配路由
	route := p.config.FindRoute(r.Method, path)
	if route == nil {
		response.Error(1003, "route not found").WriteTo(w)
		return
	}

	// 治理步骤 1：限流
	if !p.limiter.Allow(route.Name, path) {
		response.Error(8003, "rate limit exceeded").
			WithRequestID(middleware.GetRequestID(ctx)).WriteTo(w)
		return
	}

	// 治理步骤 2：过载保护（降级）
	if !p.shedder.Allow() {
		response.Error(8001, "service overloaded").
			WithRequestID(middleware.GetRequestID(ctx)).WriteTo(w)
		return
	}

	// 治理步骤 3：熔断
	if !p.breaker.Allow() {
		response.Error(8002, "circuit breaker open").
			WithRequestID(middleware.GetRequestID(ctx)).WriteTo(w)
		return
	}

	// 治理步骤 4：服务发现 + 负载均衡
	instances, err := p.registry.Discover(ctx, route.Upstream)
	if err != nil || len(instances) == 0 {
		response.Error(8000, fmt.Sprintf("no available instances for %%s", route.Upstream)).
			WithRequestID(middleware.GetRequestID(ctx)).WriteTo(w)
		return
	}

	instance, err := p.lb.Select(ctx, instances)
	if err != nil {
		response.Error(8000, "load balancer error").
			WithRequestID(middleware.GetRequestID(ctx)).WriteTo(w)
		return
	}

	// 治理步骤 5：反向代理
	start := time.Now()
	target, err := url.Parse("http://" + instance.Endpoints[0])
	if err != nil {
		response.Error(5000, "invalid upstream url").
			WithRequestID(middleware.GetRequestID(ctx)).WriteTo(w)
		p.breaker.OnFailure(0)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		p.breaker.OnFailure(time.Since(start))
		response.Error(8000, fmt.Sprintf("proxy error: %%v", err)).
			WithRequestID(middleware.GetRequestID(ctx)).WriteTo(w)
	}

	// 重写请求路径（去掉前缀）
	r.URL.Path = trimPrefix(path, route.Prefix)

	proxy.ServeHTTP(w, r)
	p.breaker.OnSuccess(time.Since(start))
}

// trimPrefix 去掉字符串前缀（避免与 strings 包冲突）。
func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
`, name, modulePath, modulePath, modulePath, modulePath, modulePath,
		modulePath, modulePath, modulePath, modulePath, name,
		name, name)

	os.WriteFile(name+"/main.go", []byte(mainGo), 0644)

	// 生成网关配置
	configGo := fmt.Sprintf(`// Package config 提供网关配置。
package config

import (
	"fmt"
	"os"
	"time"

	"%s/core/balancer"
	"gopkg.in/yaml.v3"
)

// Config 是网关的完整配置。
type Config struct {
	Server         ServerConfig         `+"`yaml:\"server\"`"+`
	Routes         []RouteConfig        `+"`yaml:\"routes\"`"+`
	RateLimit      RateLimitConfig      `+"`yaml:\"ratelimit\"`"+`
	CircuitBreaker CircuitBreakerConfig `+"`yaml:\"circuit_breaker\"`"+`
	Shedding       SheddingConfig       `+"`yaml:\"shedding\"`"+`
	Balancer       BalancerConfig       `+"`yaml:\"balancer\"`"+`
	Registry       RegistryConfig       `+"`yaml:\"registry\"`"+`
	Timeout        time.Duration        `+"`yaml:\"timeout\"`"+`
}

// ServerConfig 是网关服务配置。
type ServerConfig struct {
	Name string `+"`yaml:\"name\"`"+`
	Port int    `+"`yaml:\"port\"`"+`
}

// RouteConfig 是单条路由配置。
type RouteConfig struct {
	Name     string        `+"`yaml:\"name\"`"+`
	Path     string        `+"`yaml:\"path\"`"+`
	Method   string        `+"`yaml:\"method\"`"+`
	Prefix   string        `+"`yaml:\"prefix\"`"+`
	Upstream string        `+"`yaml:\"upstream\"`"+`
	Rate     int           `+"`yaml:\"rate\"`"+`
	Burst    int           `+"`yaml:\"burst\"`"+`
	Timeout  time.Duration `+"`yaml:\"timeout\"`"+`
}

// RateLimitConfig 是限流配置。
type RateLimitConfig struct {
	GlobalRate  int `+"`yaml:\"global_rate\"`"+`
	GlobalBurst int `+"`yaml:\"global_burst\"`"+`
}

// CircuitBreakerConfig 是熔断配置。
type CircuitBreakerConfig struct {
	WindowSize  time.Duration `+"`yaml:\"window_size\"`"+`
	BucketCount int           `+"`yaml:\"bucket_count\"`"+`
	MinRequests int           `+"`yaml:\"min_requests\"`"+`
	ErrorRate   float64       `+"`yaml:\"error_rate\"`"+`
	SleepWindow time.Duration `+"`yaml:\"sleep_window\"`"+`
}

// SheddingConfig 是降级配置。
type SheddingConfig struct {
	CPUThreshold float64 `+"`yaml:\"cpu_threshold\"`"+`
	MemThreshold float64 `+"`yaml:\"mem_threshold\"`"+`
	Window       int     `+"`yaml:\"window\"`"+`
}

// BalancerConfig 是负载均衡配置。
type BalancerConfig struct {
	Strategy balancer.Strategy `+"`yaml:\"strategy\"`"+`
}

// RegistryConfig 是注册中心配置。
type RegistryConfig struct {
	Provider        string              `+"`yaml:\"provider\"`"+`
	StaticEndpoints map[string][]string `+"`yaml:\"static_endpoints\"`"+`
}

// Load 从 YAML 文件加载配置。
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("config: cannot read %%s: %%w", filePath, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: cannot parse %%s: %%w", filePath, err)
	}

	// 设置默认值
	cfg.setDefaults()
	return cfg, nil
}

// setDefaults 设置默认配置值。
func (c *Config) setDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8888
	}
	if c.Server.Name == "" {
		c.Server.Name = "nexus-gateway"
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.RateLimit.GlobalRate == 0 {
		c.RateLimit.GlobalRate = 10000
	}
	if c.RateLimit.GlobalBurst == 0 {
		c.RateLimit.GlobalBurst = 20000
	}
	if c.CircuitBreaker.WindowSize == 0 {
		c.CircuitBreaker.WindowSize = 10 * time.Second
	}
	if c.CircuitBreaker.BucketCount == 0 {
		c.CircuitBreaker.BucketCount = 10
	}
	if c.CircuitBreaker.MinRequests == 0 {
		c.CircuitBreaker.MinRequests = 20
	}
	if c.CircuitBreaker.ErrorRate == 0 {
		c.CircuitBreaker.ErrorRate = 0.5
	}
	if c.CircuitBreaker.SleepWindow == 0 {
		c.CircuitBreaker.SleepWindow = 30 * time.Second
	}
	if c.Shedding.CPUThreshold == 0 {
		c.Shedding.CPUThreshold = 0.9
	}
	if c.Shedding.MemThreshold == 0 {
		c.Shedding.MemThreshold = 0.85
	}
	if c.Shedding.Window == 0 {
		c.Shedding.Window = 5
	}
	if c.Balancer.Strategy == 0 {
		c.Balancer.Strategy = balancer.RoundRobin
	}
	if c.Registry.StaticEndpoints == nil {
		c.Registry.StaticEndpoints = make(map[string][]string)
	}
}

// FindRoute 根据 HTTP 方法和路径查找匹配路由。
func (c *Config) FindRoute(method, path string) *RouteConfig {
	for i := range c.Routes {
		route := &c.Routes[i]
		if route.Method == method && matchPath(route.Path, path) {
			return route
		}
	}
	return nil
}

// matchPath 简化的路径匹配（支持 :param 占位符）。
func matchPath(pattern, actual string) bool {
	// TODO: 支持路径参数匹配
	return pattern == actual
}
`, modulePath)

	os.WriteFile(name+"/internal/config/config.go", []byte(configGo), 0644)

	// 生成网关 YAML 配置
	gatewayYAML := fmt.Sprintf(`# %s API Gateway 配置
server:
  name: %s-gateway
  port: 8888

# 路由配置（由 nx gen gateway 自动生成）
routes:
  # - name: GetUser
  #   path: /users/:id
  #   method: GET
  #   prefix: /api/v1
  #   upstream: user-service
  #   rate: 100
  #   burst: 200
  #   timeout: 5s

# 服务治理
ratelimit:
  global_rate: 10000
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
  window: 5

balancer:
  strategy: round_robin  # round_robin | least_conn | consistent_hash

# 服务注册与发现
registry:
  provider: static
  static_endpoints:
    user-service:
      - localhost:8081
      - localhost:8082
    order-service:
      - localhost:8083
      - localhost:8084

# 超时配置
timeout: 30s
`, name, name)

	os.WriteFile(name+"/etc/"+name+".yaml", []byte(gatewayYAML), 0644)

	// 生成 .api 模板文件
	apiTemplate := fmt.Sprintf(`info {
    title "%s API Gateway"
    desc "Nexus Micro API Gateway"
    version "1.0.0"
}

@server(
    prefix: "/api/v1"
    service: "%s"
)

service %sGateway {
    @handler Health
    @doc "健康检查"
    get /health

    // 在此定义网关路由...
    // @handler CreateUser
    // @doc "创建用户"
    // post /users
}
`, name, name, name)

	os.WriteFile(name+"/"+name+".api", []byte(apiTemplate), 0644)

	fmt.Printf("✓ Gateway %s created\n", name)
	fmt.Println("\nDirectory structure:")
	fmt.Printf("  %s/\n", name)
	fmt.Printf("  %s/main.go               — 网关入口（含治理流水线）\n", name)
	fmt.Printf("  %s/                          — 默认配置文件\n", name+"/etc/"+name+".yaml")
	fmt.Printf("  %s/                      — 路由 DSL 模板\n", name+"/"+name+".api")
	fmt.Printf("  %s/internal/config/      — 配置结构体\n", name)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. 编辑 %s 配置上游服务\n", name+"/etc/"+name+".yaml")
	fmt.Printf("  2. 编辑 %s 定义网关路由\n", name+"/"+name+".api")
	fmt.Printf("  3. Run: nx gen gateway %s\n", name+"/"+name+".api")
	fmt.Printf("  4. Run: cd %s && go run main.go\n", name)
}

// cmdNewWorkspace 创建多服务 workspace（monorepo）。
func cmdNewWorkspace(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx new workspace: missing workspace name")
		fmt.Fprintln(os.Stderr, "Usage: nx new workspace <name>")
		os.Exit(1)
	}

	name := args[0]

	dirs := []string{
		name,
		name + "/services",
		name + "/gateway",
		name + "/shared",
		name + "/deploy",
		name + "/scripts",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "nx new workspace: failed to create %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// 生成 go.work
	goWork := fmt.Sprintf(`go 1.24

use (
	./services/*
	./gateway
	./shared
)
`)
	os.WriteFile(name+"/go.work", []byte(goWork), 0644)

	// 生成 Makefile
	makefile := fmt.Sprintf(`# %s Workspace Makefile
.PHONY: help build test clean run-gateway

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%%-15s\033[0m %%s\n", $$1, $$2}'

build: ## 构建所有服务
	@echo "Building all services..."
	@for dir in services/*/; do \
		echo "Building $$dir..."; \
		(cd $$dir && go build -o ../../bin/$$(basename $$dir) .) || exit 1; \
	done
	@echo "Building gateway..."
	(cd gateway && go build -o ../bin/gateway .)
	@echo "✓ Build complete"

test: ## 运行所有测试
	go test ./...

clean: ## 清理构建产物
	rm -rf bin/

run-gateway: ## 启动网关
	cd gateway && go run main.go

run-%%: ## 启动指定服务 (e.g. make run-user)
	cd services/$* && go run main.go
`, name)

	os.WriteFile(name+"/Makefile", []byte(makefile), 0644)

	// 生成 docker-compose.yml
	dockerCompose := fmt.Sprintf(`# %s Workspace Docker Compose
version: "3.8"

services:
  # 基础设施
  postgres:
    image: postgres:18-alpine
    environment:
      POSTGRES_USER: nexus
      POSTGRES_PASSWORD: nexus123
      POSTGRES_DB: nexus
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nexus"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  # API 网关
  gateway:
    build:
      context: ./gateway
      dockerfile: Dockerfile
    ports:
      - "8888:8888"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - CONFIG_PATH=/app/etc/%s.yaml
    volumes:
      - ./gateway/etc:/app/etc

volumes:
  pgdata:
`, name, name)

	os.WriteFile(name+"/docker-compose.yml", []byte(dockerCompose), 0644)

	// 生成 .gitignore
	gitignore := `# Binaries
bin/
*.exe
*.dll
*.so
*.dylib

# Test binary
*.test

# Output of the go coverage tool
*.out

# Dependency directories
vendor/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Env
.env
.env.local
`
	os.WriteFile(name+"/.gitignore", []byte(gitignore), 0644)

	fmt.Printf("✓ Workspace %s created\n", name)
	fmt.Println("\nDirectory structure:")
	fmt.Printf("  %s/\n", name)
	fmt.Printf("  %s/services/      — 微服务目录\n", name)
	fmt.Printf("  %s/gateway/       — API 网关\n", name)
	fmt.Printf("  %s/shared/        — 共享库（类型、工具）\n", name)
	fmt.Printf("  %s/deploy/        — 部署配置\n", name)
	fmt.Printf("  %s/scripts/       — 构建脚本\n", name)
	fmt.Printf("  %s/go.work        — Go workspace\n", name)
	fmt.Printf("  %s/Makefile       — 构建命令\n", name)
	fmt.Printf("  %s/docker-compose.yml\n", name)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. cd %s\n", name)
	fmt.Printf("  2. 创建服务: nx new services/user\n")
	fmt.Printf("  3. 创建网关: nx new gateway gateway\n")
	fmt.Printf("  4. 开发服务: make build && make run-gateway\n")
}

// cmdGen 代码生成。
func cmdGen(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "nx gen: missing subcommand")
		fmt.Fprintln(os.Stderr, "Usage: nx gen [api|gateway|proto|client|doc] ...")
		os.Exit(1)
	}

	subcommand := args[0]
	switch subcommand {
	case "api":
		cmdGenAPI(args[1:])
	case "gateway":
		cmdGenGateway(args[1:])
	case "proto":
		cmdGenProto(args[1:])
	case "client":
		cmdGenClient(args[1:])
	case "doc":
		cmdGenDoc(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "nx gen: unknown subcommand %q\n", subcommand)
		os.Exit(1)
	}
}

// cmdGenAPI 从 .api 文件生成服务代码。
func cmdGenAPI(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx gen api: missing .api file")
		fmt.Fprintln(os.Stderr, "Usage: nx gen api <file.api>")
		os.Exit(1)
	}

	apiFile := args[0]
	fmt.Printf("Generating service code from %s...\n", apiFile)

	gen := generator.New("service", "github.com/xyxuliang/nexus-micro")
	if err := gen.GenerateFromFile(apiFile); err != nil {
		fmt.Fprintf(os.Stderr, "nx gen api: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Service code generation completed")
}

// cmdGenGateway 从 .api 文件生成网关代码。
func cmdGenGateway(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx gen gateway: missing .api file")
		fmt.Fprintln(os.Stderr, "Usage: nx gen gateway <file.api>")
		os.Exit(1)
	}

	apiFile := args[0]
	fmt.Printf("Generating gateway code from %s...\n", apiFile)

	gen := generator.NewGatewayGenerator("github.com/xyxuliang/nexus-micro")
	if err := gen.GenerateFromFile(apiFile); err != nil {
		fmt.Fprintf(os.Stderr, "nx gen gateway: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Gateway code generation completed")
}

// cmdGenProto 从 .proto 文件生成 gRPC stub。
func cmdGenProto(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx gen proto: missing .proto file")
		fmt.Fprintln(os.Stderr, "Usage: nx gen proto <file.proto>")
		os.Exit(1)
	}

	protoFile := args[0]
	fmt.Printf("Generating gRPC stubs from %s...\n", protoFile)

	// 确定输出目录（proto 文件所在目录的父目录）
	protoDir := filepath.Dir(protoFile)
	outDir := filepath.Dir(protoDir)
	if outDir == "." {
		outDir = "."
	}

	// 获取模块路径
	modulePath := detectModule()

	cmd := exec.Command("protoc",
		"--proto_path="+protoDir,
		"--go_out="+outDir,
		"--go_opt=module="+modulePath,
		"--go-grpc_out="+outDir,
		"--go-grpc_opt=module="+modulePath,
		protoFile,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nx gen proto: protoc failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Make sure protoc, protoc-gen-go and protoc-gen-go-grpc are installed.")
		os.Exit(1)
	}

	fmt.Println("✓ gRPC stub generation completed")
}

// detectModule 从 go.mod 读取模块路径，失败则返回默认值。
func detectModule() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "github.com/xyxuliang/nexus-micro"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return "github.com/xyxuliang/nexus-micro"
}

// cmdGenClient 生成客户端 SDK。
func cmdGenClient(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx gen client: missing service name")
		fmt.Fprintln(os.Stderr, "Usage: nx gen client <service-name>")
		os.Exit(1)
	}

	serviceName := args[0]
	fmt.Printf("Generating client SDK for %s...\n", serviceName)

	gen := generator.New("client", "github.com/xyxuliang/nexus-micro")
	if err := gen.GenerateClient(serviceName); err != nil {
		fmt.Fprintf(os.Stderr, "nx gen client: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Client SDK generation completed")
}

// cmdGenDoc 生成 OpenAPI 文档。
func cmdGenDoc(args []string) {
	fmt.Println("nx gen doc: generating OpenAPI documentation...")
	fmt.Println("(OpenAPI doc generation coming soon)")
}

// cmdModule 创建业务模块。
func cmdModule(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx module: missing module name")
		fmt.Fprintln(os.Stderr, "Usage: nx module <name>")
		os.Exit(1)
	}

	moduleName := args[0]
	dirs := []string{
		"app/" + moduleName + "/domain",
		"app/" + moduleName + "/internal",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "nx module: failed to create %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	fmt.Printf("✓ Module %s created\n", moduleName)
	fmt.Printf("  app/%s/domain/    — entity, repository, event, errors\n", moduleName)
	fmt.Printf("  app/%s/internal/  — repository implementation\n", moduleName)
}

// cmdSlice 创建 Vertical Slice 切片。
func cmdSlice(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "nx slice: missing module or slice name")
		fmt.Fprintln(os.Stderr, "Usage: nx slice <module> <name>")
		os.Exit(1)
	}

	moduleName := args[0]
	sliceName := args[1]
	sliceDir := "app/" + moduleName + "/" + sliceName

	if err := os.MkdirAll(sliceDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "nx slice: failed to create %s: %v\n", sliceDir, err)
		os.Exit(1)
	}

	files := []string{"command.go", "handler.go", "validator.go", "dto.go", "mapper.go", "event.go"}
	for _, f := range files {
		path := sliceDir + "/" + f
		os.WriteFile(path, []byte("// "+sliceName+" "+f+"\n"), 0644)
	}

	fmt.Printf("✓ Slice %s/%s created\n", moduleName, sliceName)
	fmt.Printf("  app/%s/%s/ — command, handler, validator, dto, mapper, event\n", moduleName, sliceName)
}

// cmdQuery 创建 Query 切片。
func cmdQuery(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "nx query: missing module or query name")
		fmt.Fprintln(os.Stderr, "Usage: nx query <module> <name>")
		os.Exit(1)
	}

	moduleName := args[0]
	queryName := args[1]
	queryDir := "app/" + moduleName + "/" + queryName

	if err := os.MkdirAll(queryDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "nx query: failed to create %s: %v\n", queryDir, err)
		os.Exit(1)
	}

	files := []string{"query.go", "handler.go", "dto.go"}
	for _, f := range files {
		path := queryDir + "/" + f
		os.WriteFile(path, []byte("// "+queryName+" "+f+"\n"), 0644)
	}

	fmt.Printf("✓ Query %s/%s created\n", moduleName, queryName)
}

// cmdRun 启动开发服务器。
// 在当前目录查找 main.go 并执行 go run。
func cmdRun(args []string) {
	mainFile := "main.go"
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		entries, err := os.ReadDir(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "nx run: cannot read current directory: %v\n", err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.IsDir() {
				p := filepath.Join(e.Name(), "main.go")
				if _, err := os.Stat(p); err == nil {
					mainFile = p
					break
				}
			}
		}
		if mainFile == "main.go" {
			fmt.Fprintln(os.Stderr, "nx run: no main.go found in current directory")
			fmt.Fprintln(os.Stderr, "Run 'nx new <name>' to create a project first.")
			os.Exit(1)
		}
	}

	fmt.Printf("Starting development server (%s)...\n", mainFile)
	cmd := exec.Command("go", append([]string{"run", mainFile}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nx run: %v\n", err)
		os.Exit(1)
	}
}

// cmdBuild 构建生产二进制。
// 默认输出到 bin/<服务名>，支持 GOOS/GOARCH 交叉编译。
func cmdBuild(args []string) {
	output := "bin/service"
	if len(args) > 0 {
		output = args[0]
	}

	cwd, _ := os.Getwd()
	svcName := filepath.Base(cwd)
	if output == "bin/service" {
		output = "bin/" + svcName
	}

	fmt.Printf("Building %s...\n", output)
	cmd := exec.Command("go", "build", "-o", output, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nx build: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Built %s\n", output)
}

// cmdTest 运行测试。
// 默认运行 ./... 所有包，支持传递 go test 参数。
func cmdTest(args []string) {
	testArgs := []string{"test"}
	if len(args) == 0 {
		testArgs = append(testArgs, "./...")
	} else {
		testArgs = append(testArgs, args...)
	}
	testArgs = append(testArgs, "-v", "-count=1")

	fmt.Println("Running tests...")
	cmd := exec.Command("go", testArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

// cmdLint 代码检查。
// 运行 go vet 进行静态分析，默认检查所有包。
func cmdLint(args []string) {
	packages := "./..."
	if len(args) > 0 {
		packages = args[0]
	}

	fmt.Printf("Running go vet on %s...\n", packages)
	cmd := exec.Command("go", "vet", packages)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
	fmt.Println("✓ Lint complete")
}

// =============================================================================
// RPC Commands
// =============================================================================

// cmdRPC RPC 相关命令分发：nx rpc proto <file> | nx rpc service <name>
func cmdRPC(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx rpc: missing subcommand")
		fmt.Fprintln(os.Stderr, "Usage: nx rpc [proto|service] ...")
		os.Exit(1)
	}
	switch args[0] {
	case "proto":
		cmdGenProto(args[1:])
	case "service":
		cmdRPCGenService(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "nx rpc: unknown subcommand %q\n", args[0])
		os.Exit(1)
	}
}

// cmdRPCGenService 从 .api 生成 gRPC 服务骨架（nx rpc service 入口）。
func cmdRPCGenService(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx rpc service: missing service name")
		fmt.Fprintln(os.Stderr, "Usage: nx rpc service <name>")
		os.Exit(1)
	}
	createRPCService(args[0])
}

// cmdNewRpcService 创建 gRPC 微服务项目 (nx new rpc 入口)。
func cmdNewRpcService(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx new rpc: missing service name")
		fmt.Fprintln(os.Stderr, "Usage: nx new rpc <name>")
		os.Exit(1)
	}
	createRPCService(args[0])
}

// createRPCService 生成 gRPC 服务项目。
func createRPCService(serviceName string) {
	fmt.Printf("Creating gRPC service %s...\n", serviceName)

	dirs := []string{
		serviceName, serviceName + "/proto",
		serviceName + "/internal/server", serviceName + "/etc",
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}

	// .proto 模板
	sN := capitalize(serviceName)
	protoContent := fmt.Sprintf(`syntax = "proto3";

package %s;

option go_package = "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/%s;%spb";

service %sService {
  rpc SayHello(HelloReq) returns (HelloResp);
}

message HelloReq { string name = 1; }
message HelloResp { string message = 1; }
`, serviceName, serviceName, serviceName, sN)
	os.WriteFile(serviceName+"/proto/"+serviceName+".proto", []byte(protoContent), 0644)

	// etc 配置
	cfgYAML := fmt.Sprintf("server:\n  name: %s\n  grpc_port: 50051\netcd:\n  endpoints:\n    - localhost:2379\n  ttl: 30\n", serviceName)
	os.WriteFile(serviceName+"/etc/"+serviceName+".yaml", []byte(cfgYAML), 0644)

	// server 实现
	svrGo := fmt.Sprintf(`package server

import (
	"context"
	"google.golang.org/grpc"
	%[1]spb "github.com/xyxuliang/nexus-micro/examples/rpc-demo/pkg/pb/%[1]s"
)

type %[2]sServer struct {
	%[1]spb.Unimplemented%[2]sServiceServer
}

func Register(srv *grpc.Server) {
	%[1]spb.Register%[2]sServiceServer(srv, &%[2]sServer{})
}

func (s *%[2]sServer) SayHello(ctx context.Context, req *%[1]spb.HelloReq) (*%[1]spb.HelloResp, error) {
	return &%[1]spb.HelloResp{Message: "Hello, " + req.Name + "!"}, nil
}
`, serviceName, sN)
	os.WriteFile(serviceName+"/internal/server/server.go", []byte(svrGo), 0644)

	// main.go
	mainGo := fmt.Sprintf(`package main

import (
	"context"; "fmt"; "log"; "net"; "os"; "os/signal"; "strconv"; "sync"; "syscall"; "time"
	"google.golang.org/grpc"; "google.golang.org/grpc/reflection"
	"github.com/xyxuliang/nexus-micro/core/registry"
	"github.com/xyxuliang/nexus-micro/examples/rpc-demo/%[1]s/internal/server"
)

func main() {
	cfg := struct{Server struct{Name string;GrpcPort int};Etcd struct{Endpoints []string;TTL int}}{}
	cfg.Server.Name = "%[1]s"; cfg.Server.GrpcPort = 50051
	cfg.Etcd.Endpoints = []string{"localhost:2379"}; cfg.Etcd.TTL = 30
	if ps := os.Getenv("GRPC_PORT"); ps != "" {
		if p, _ := strconv.Atoi(ps); p > 0 { cfg.Server.GrpcPort = p }
	}
	inst := &registry.ServiceInstance{
		ID: fmt.Sprintf("%[1]s-%%d", time.Now().UnixNano()), Name: cfg.Server.Name, Version: "1.0.0",
		Metadata: map[string]string{"weight": "100", "protocol": "grpc"},
		Endpoints: []string{fmt.Sprintf("localhost:%%d", cfg.Server.GrpcPort)},
	}
	reg, err := registry.NewEtcdRegistry(cfg.Etcd.Endpoints, 5*time.Second)
	if err != nil { log.Fatalf("etcd: %%v", err) }
	defer reg.Close()
	reg.Register(context.Background(), inst)
	log.Printf("[%[1]s] registered (grpc %%s)", inst.Endpoints[0])
	addr := fmt.Sprintf(":%%d", cfg.Server.GrpcPort)
	lis, _ := net.Listen("tcp", addr)
	srv := grpc.NewServer(); reflection.Register(srv); server.Register(srv)
	var wg sync.WaitGroup; wg.Add(1)
	go func() {
		defer wg.Done()
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		reg.Deregister(context.Background(), inst)
		srv.GracefulStop()
	}()
	log.Printf("[%[1]s] gRPC listening on %%s", addr)
	srv.Serve(lis); wg.Wait()
}
`, serviceName)
	os.WriteFile(serviceName+"/main.go", []byte(mainGo), 0644)

	fmt.Printf("✓ RPC service %s created\n", serviceName)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. nx gen proto %s/proto/%s.proto\n", serviceName, serviceName)
	fmt.Printf("  2. cd %s && go run main.go\n", serviceName)
}

// cmdNewRpcGateway 创建 HTTP+gRPC 双协议网关。
func cmdNewRpcGateway(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx new rpc-gateway: missing gateway name")
		os.Exit(1)
	}
	name := args[0]
	fmt.Printf("Creating RPC gateway %s...\n", name)

	dirs := []string{
		name, name + "/etc",
		name + "/internal/pool", name + "/internal/proxy",
		name + "/internal/handler", name + "/internal/router", name + "/internal/config",
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}

	// etc
	os.WriteFile(name+"/etc/"+name+".yaml", []byte(fmt.Sprintf(
		"server:\n  name: %s\n  port: 8889\n  grpc_port: 8890\netcd:\n  endpoints:\n    - localhost:2379\n", name)), 0644)

	// config
	os.WriteFile(name+"/internal/config/config.go", []byte(fmt.Sprintf(`package config
import ("fmt"; "os"; "gopkg.in/yaml.v3")
type Config struct {
	Server struct{Name string`+"`yaml:\"name\"`"+`;Port int`+"`yaml:\"port\"`"+`;GrpcPort int`+"`yaml:\"grpc_port\"`"+`}`+"`yaml:\"server\"`"+`
	Etcd struct{Endpoints []string`+"`yaml:\"endpoints\"`"+`}`+"`yaml:\"etcd\"`"+`
}
func Load(path string) (*Config, error) {
	d,_:=os.ReadFile(path);c:=&Config{}
	c.Server.Name="%[1]s";c.Server.Port=8889;c.Server.GrpcPort=8890
	c.Etcd.Endpoints=[]string{"localhost:2379"}
	if err:=yaml.Unmarshal(d,c);err!=nil{return nil,err}
	return c,nil
}
`, name)), 0644)

	// pool skeleton
	os.WriteFile(name+"/internal/pool/pool.go", []byte(`package pool
// TODO: Implement gRPC connection pool with etcd discovery + round-robin LB.
// See examples/rpc-demo/gateway/internal/pool/pool.go for reference.
type Services struct{}
`), 0644)

	// proxy skeleton
	os.WriteFile(name+"/internal/proxy/proxy.go", []byte(`package proxy
import ("google.golang.org/grpc"; "github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool")
func Register(srv *grpc.Server, pools *pool.Services) {
	// Register your gRPC proxy handlers here.
	_ = srv; _ = pools
}
`), 0644)

	// handler skeleton
	os.WriteFile(name+"/internal/handler/handler.go", []byte(`package handler
import ("net/http"; "github.com/gin-gonic/gin"; "github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool")
type HTTPHandler struct{Pools *pool.Services}
func New(pools *pool.Services) *HTTPHandler { return &HTTPHandler{Pools: pools} }
func (h *HTTPHandler) Health(c *gin.Context) { c.JSON(200, gin.H{"code":0,"msg":"ok"}) }
`), 0644)

	// router skeleton
	os.WriteFile(name+"/internal/router/router.go", []byte(`package router
import ("net/http"; "time"; "github.com/gin-gonic/gin"; . "github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/handler"; "github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool")
func Register(r *gin.Engine, pools *pool.Services) {
	h:=New(pools)
	r.GET("/health",func(c *gin.Context){c.JSON(http.StatusOK,gin.H{"code":0,"msg":"ok","data":gin.H{"time":time.Now().Format(time.RFC3339)}})})
	api:=r.Group("/api/v1"); _=api; _=h
}
`), 0644)

	// main.go
	os.WriteFile(name+"/main.go", []byte(fmt.Sprintf(`package main
import ("context";"fmt";"log";"net";"net/http";"os";"os/signal";"sync";"syscall";"time"
"github.com/gin-gonic/gin";"google.golang.org/grpc";"google.golang.org/grpc/reflection"
"github.com/xyxuliang/nexus-micro/core/registry"
"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/config"
"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/pool"
"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/proxy"
"github.com/xyxuliang/nexus-micro/examples/rpc-demo/gateway/internal/router")

func main() {
	cfg,_:=config.Load("etc/%[1]s.yaml")
	reg,_:=registry.NewEtcdRegistry(cfg.Etcd.Endpoints,5*time.Second)
	defer reg.Close()
	pools:=&pool.Services{}
	var wg sync.WaitGroup

	// HTTP
	r:=gin.New();r.Use(gin.Logger(),gin.Recovery())
	router.Register(r,pools)
	httpSrv:=&http.Server{Addr:fmt.Sprintf(":%%d",cfg.Server.Port),Handler:r,ReadTimeout:30*time.Second,WriteTimeout:30*time.Second}

	wg.Add(1);go func(){defer wg.Done();httpSrv.ListenAndServe()}()

	// gRPC
	lis,_:=net.Listen("tcp",fmt.Sprintf(":%%d",cfg.Server.GrpcPort))
	srv:=grpc.NewServer();reflection.Register(srv);proxy.Register(srv,pools)
	wg.Add(1);go func(){defer wg.Done();srv.Serve(lis)}()

	go func(){quit:=make(chan os.Signal,1);signal.Notify(quit,syscall.SIGINT,syscall.SIGTERM);<-quit;srv.GracefulStop();httpSrv.Shutdown(context.Background())}()

	log.Println("[gateway] ready: HTTP :8889 | gRPC :8890")
	wg.Wait()
}
`, name)), 0644)

	fmt.Printf("✓ RPC Gateway %s created\n", name)
	fmt.Println("  Edit internal/pool/, proxy/, handler/, router/ to add your services.")
}

// cmdNewRpcWorkspace 创建 RPC 多服务工作区。
func cmdNewRpcWorkspace(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "nx new rpc-workspace: missing workspace name")
		os.Exit(1)
	}
	name := args[0]
	dirs := []string{name, name + "/proto", name + "/pkg/pb", name + "/services", name + "/gateway", name + "/scripts"}
	for _, d := range dirs {
		os.MkdirAll(d, 0755)
	}
	os.WriteFile(name+"/go.work", []byte("go 1.24\n\nuse (\n\t./services/*\n\t./gateway\n\t./pkg/pb\n)\n"), 0644)
	os.WriteFile(name+"/scripts/gen_proto.sh", []byte(fmt.Sprintf(`#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"; WS_DIR="$SCRIPT_DIR/.."
MODULE="github.com/xyxuliang/nexus-micro/examples/rpc-demo"
cd "$WS_DIR"
protoc --proto_path=proto --go_out=. --go_opt=module="$MODULE" --go-grpc_out=. --go-grpc_opt=module="$MODULE" proto/*.proto
echo "proto generation done."
`)), 0755)
	os.WriteFile(name+"/scripts/build.sh", []byte(`#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"; WS_DIR="$SCRIPT_DIR/.."
BIN_DIR="$WS_DIR/bin"; mkdir -p "$BIN_DIR"; cd "$WS_DIR"
for svc in services/*/; do
    [[ -f "$svc/main.go" ]] || continue
    n=$(basename "$svc"); echo "Building $n..."; go build -o "$BIN_DIR/$n-rpc" "./$svc" || exit 1
done
echo "Building gateway..."; go build -o "$BIN_DIR/gateway-rpc" ./gateway/ || exit 1
echo "Done."; ls -lh "$BIN_DIR/"
`), 0755)
	os.WriteFile(name+"/Makefile", []byte(".PHONY: proto build clean\n\nproto:\n\tcd scripts && ./gen_proto.sh\n\nbuild:\n\tcd scripts && ./build.sh\n\nclean:\n\trm -rf bin/ logs/\n"), 0644)
	os.WriteFile(name+"/.gitignore", []byte("bin/\nlogs/\n"), 0644)

	fmt.Printf("✓ RPC Workspace %s created\n", name)
	fmt.Println("  proto/     — protobuf definitions")
	fmt.Println("  services/  — gRPC services")
	fmt.Println("  gateway/   — HTTP+gRPC gateway")
	fmt.Println("  scripts/   — gen_proto.sh + build.sh")
}

// capitalize 首字母大写。
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
