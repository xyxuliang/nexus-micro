// Package generator 提供 API 网关代码生成器。
// 从 .api DSL 的 AST 生成完整的网关项目代码，包含：
//   - 网关入口（含完整治理流水线：限流→降级→熔断→负载均衡→反向代理）
//   - 路由配置结构体
//   - 代理处理器
//   - 默认配置文件
package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xyxuliang/nexus-micro/dsl/ast"
	"github.com/xyxuliang/nexus-micro/dsl/parser"
	"github.com/xyxuliang/nexus-micro/internal/util"
)

// GatewayGenerator 是 API 网关代码生成器。
// 从 .api DSL 读取路由定义，生成包含完整治理流水线的网关项目。
type GatewayGenerator struct {
	outputDir string // 输出目录（网关项目名）
	module    string // Go module 路径
}

// NewGatewayGenerator 创建一个新的网关代码生成器。
// module 是 Go module 路径，如 "github.com/xyxuliang/nexus-micro"。
func NewGatewayGenerator(module string) *GatewayGenerator {
	return &GatewayGenerator{
		outputDir: "gateway",
		module:    module,
	}
}

// GenerateFromFile 从 .api 文件读取路由定义并生成网关代码。
func (g *GatewayGenerator) GenerateFromFile(apiFile string) error {
	content, err := os.ReadFile(apiFile)
	if err != nil {
		return fmt.Errorf("gateway generator: cannot read file %s: %w", apiFile, err)
	}
	return g.Generate(string(content))
}

// Generate 从 DSL 源码生成网关代码。
func (g *GatewayGenerator) Generate(source string) error {
	// 解析 DSL → AST
	p := parser.New(source)
	file, errs := p.Parse()
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		}
		return fmt.Errorf("gateway generator: %d parse errors", len(errs))
	}

	// 确保输出目录存在
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("gateway generator: cannot create output dir: %w", err)
	}

	// 收集所有路由信息
	routes := g.collectRoutes(file)

	// 生成网关入口
	g.generateGatewayMain(file, routes)

	// 生成配置结构体
	g.generateGatewayConfig(routes)

	// 生成代理处理器
	g.generateProxyHandler(routes)

	// 生成配置文件
	g.generateGatewayYAML(file, routes)

	return nil
}

// collectRoutes 从 AST 中收集所有路由定义。
func (g *GatewayGenerator) collectRoutes(file *ast.File) []routeInfo {
	var routes []routeInfo
	for _, svc := range file.Services {
		for _, handler := range svc.Handlers {
			routes = append(routes, routeInfo{
				Name:     handler.Name,
				Method:   handler.Method,
				Path:     handler.Path,
				Doc:      handler.Doc,
				Prefix:   svc.Prefix,
				Group:    svc.Group,
				Upstream: svc.Group + "-service", // 默认上游服务名
			})
		}
	}
	return routes
}

// routeInfo 是单条路由的元信息。
type routeInfo struct {
	Name     string // Handler 名称
	Method   string // HTTP 方法
	Path     string // 路由路径
	Doc      string // 文档描述
	Prefix   string // 路由前缀
	Group    string // 服务分组
	Upstream string // 上游服务名
}

// generateGatewayMain 生成网关主入口（含完整治理流水线）。
func (g *GatewayGenerator) generateGatewayMain(file *ast.File, routes []routeInfo) {
	// 构建路由注册代码
	var routeReg strings.Builder
	for _, route := range routes {
		routeReg.WriteString(fmt.Sprintf(`	// %s: %s
	mux.HandleFunc("%s%s", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.Method%s {
			proxyHandler.ServeHTTP(w, r)
		} else {
			response.Error(1003, "method not allowed").WriteTo(w)
		}
	})
`, route.Doc, route.Path, route.Prefix, route.Path, strings.Title(strings.ToLower(route.Method))))
	}

	// 构建路由配置 YAML 条目
	var routeYAML strings.Builder
	for _, route := range routes {
		routeYAML.WriteString(fmt.Sprintf(`  - name: %s
    path: %s
    method: %s
    prefix: %s
    upstream: %s
    rate: 100
    burst: 200
    timeout: 5s
`, route.Name, route.Path, route.Method, route.Prefix, route.Upstream))
	}

	info := file.Info
	title := "Nexus API Gateway"
	version := "1.0.0"
	if info != nil {
		if info.Title != "" {
			title = info.Title
		}
		if info.Version != "" {
			version = info.Version
		}
	}

	content := fmt.Sprintf(`// %s — Nexus Micro API Gateway
// 由 nx gen gateway 自动生成，提供统一的 API 入口和服务治理。
//
// 治理流水线：
//   请求 → 限流(RateLimit) → 降级(Shedding) → 熔断(CircuitBreaker)
//        → 服务发现(Discovery) → 负载均衡(Balancer) → 反向代理 → 响应
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
	"strings"
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
	"%s/gateway/internal/config"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("etc/gateway.yaml")
	if err != nil {
		log.Fatalf("[gateway] failed to load config: %%v", err)
	}

	// 2. 初始化治理组件
	// 限流器 — 全局 + 每路由级别，先经过全局桶再经过服务桶
	limiter := ratelimit.NewMultiLevel(
		cfg.RateLimit.GlobalRate,
		cfg.RateLimit.GlobalBurst,
	)
	for _, route := range cfg.Routes {
		limiter.AddService(route.Name, route.Rate, route.Burst)
	}

	// 降级器 — CPU + 内存双层过载保护
	shedder := shedding.New(&shedding.Config{
		CPUThreshold: cfg.Shedding.CPUThreshold,
		MemThreshold: cfg.Shedding.MemThreshold,
		Window:       cfg.Shedding.Window,
	})

	// 负载均衡器 — 支持 RoundRobin / LeastConnection / ConsistentHash 四种策略
	lb := balancer.New(&balancer.Config{
		Strategy: cfg.Balancer.Strategy,
	})

	// 注册中心 — 静态服务发现（后续可扩展到 etcd/consul/k8s）
	reg := registry.NewStaticRegistry(cfg.Registry.StaticEndpoints)

	// 3. 构建中间件链
	chain := middleware.DefaultChain()
	chain.Use(middleware.Timeout(cfg.Timeout))
	chain.Use(middleware.Metrics())

	// 4. 创建 HTTP 路由
	mux := http.NewServeMux()

	// 健康检查 — 返回 JSON 格式状态信息
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		response.Success(map[string]interface{}{
			"status":  "ok",
			"service": cfg.Server.Name,
			"version": "%s",
			"time":    time.Now().Format(time.RFC3339),
			"cpu":     shedder.LastCPU(),
			"memory":  shedder.LastMem(),
		}).WithRequestID(middleware.GetRequestID(r.Context())).WriteTo(w)
	})

	// 就绪检查 — K8s readiness probe
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// 检查降级状态：如果过载则返回 503
		if shedder.IsOverloaded() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "overloaded")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "ready")
	})

	// 存活检查 — K8s liveness probe
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "alive")
	})

	// 5. 为每条路由注册代理处理器
	proxyHandler := NewProxyHandler(cfg, limiter, shedder, lb, reg)
%s
	// 6. 启动 HTTP 服务器
	addr := fmt.Sprintf(":%%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
		IdleTimeout:  120 * time.Second,
	}

	// 7. 优雅关闭 — 捕获 SIGINT/SIGTERM 信号
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

	log.Printf("[gateway] %%s v%%s starting on %%s", cfg.Server.Name, "%s", addr)
	log.Printf("[gateway] registered %%d routes", len(cfg.Routes))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[gateway] server error: %%v", err)
	}
	log.Println("[gateway] server stopped")
}

// =============================================================================
// ProxyHandler — API 网关代理处理器
// =============================================================================
// 将每条请求依次通过：限流 → 降级 → 熔断 → 服务发现 → 负载均衡 → 反向代理。
// 每一步失败都会返回对应的统一错误响应。

// ProxyHandler 是 API 网关的代理处理器。
// 集成了完整的服务治理流水线，对上游微服务进行透明代理。
type ProxyHandler struct {
	config   *config.Config               // 网关配置
	limiter  *ratelimit.MultiLevel        // 多级限流器（全局+服务+路由）
	shedder  shedding.Shedder             // 过载降级器（CPU + 内存）
	lb       balancer.LoadBalancer        // 负载均衡器
	registry registry.Registry            // 服务注册中心
	breakers map[string]*circuitbreaker.AdaptiveCB // 每上游服务的熔断器
}

// NewProxyHandler 创建代理处理器。
// 为每个不同的上游服务创建独立的熔断器，避免一个服务的故障影响其他服务。
func NewProxyHandler(
	cfg *config.Config,
	limiter *ratelimit.MultiLevel,
	shedder shedding.Shedder,
	lb balancer.LoadBalancer,
	reg registry.Registry,
) *ProxyHandler {
	// 为每个上游服务创建独立的熔断器
	breakers := make(map[string]*circuitbreaker.AdaptiveCB)
	seen := make(map[string]bool)
	for _, route := range cfg.Routes {
		if !seen[route.Upstream] {
			seen[route.Upstream] = true
			breakers[route.Upstream] = circuitbreaker.New(&circuitbreaker.Config{
				WindowSize:   cfg.CircuitBreaker.WindowSize,
				BucketCount:  cfg.CircuitBreaker.BucketCount,
				MinRequests:  cfg.CircuitBreaker.MinRequests,
				ErrorRate:    cfg.CircuitBreaker.ErrorRate,
				SleepWindow:  cfg.CircuitBreaker.SleepWindow,
			})
		}
	}

	return &ProxyHandler{
		config:   cfg,
		limiter:  limiter,
		shedder:  shedder,
		lb:       lb,
		registry: reg,
		breakers: breakers,
	}
}

// ServeHTTP 处理 HTTP 请求，执行完整的治理流水线。
//
// 治理流水线步骤：
//   1. 路由匹配 — 根据 method + path 查找对应路由配置
//   2. 限流     — 检查全局+路由级别令牌桶
//   3. 降级     — CPU/内存过载时直接返回 503
//   4. 熔断     — 上游错误率过高时快速失败
//   5. 服务发现 — 从注册中心获取上游实例列表
//   6. 负载均衡 — 选择一个健康实例
//   7. 反向代理 — 转发请求到选中的上游实例
func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestID(ctx)

	// 步骤 1：路由匹配
	route := p.config.FindRoute(r.Method, r.URL.Path)
	if route == nil {
		response.Error(1003, "route not found: "+r.Method+" "+r.URL.Path).
			WithRequestID(requestID).WriteTo(w)
		return
	}

	// 步骤 2：限流 — 先全局后服务级别
	if !p.limiter.Allow(route.Name, r.URL.Path) {
		response.Error(8003, "rate limit exceeded").
			WithRequestID(requestID).WriteTo(w)
		return
	}

	// 步骤 3：过载保护（降级）— CPU 或内存超过阈值时直接拒绝
	if !p.shedder.Allow() {
		response.Error(8001, "service overloaded, please retry later").
			WithRequestID(requestID).WriteTo(w)
		return
	}

	// 步骤 4：熔断 — 检查上游服务的熔断器状态
	breaker, ok := p.breakers[route.Upstream]
	if ok && !breaker.Allow() {
		response.Error(8002, fmt.Sprintf("circuit breaker open for %%s", route.Upstream)).
			WithRequestID(requestID).WriteTo(w)
		return
	}

	// 步骤 5：服务发现 — 从注册中心获取上游实例
	instances, err := p.registry.Discover(ctx, route.Upstream)
	if err != nil || len(instances) == 0 {
		response.Error(8000, fmt.Sprintf("no available instances for %%s", route.Upstream)).
			WithRequestID(requestID).WriteTo(w)
		return
	}

	// 步骤 6：负载均衡 — 选择一个健康实例
	instance, err := p.lb.Select(ctx, instances)
	if err != nil {
		response.Error(8000, "load balancer error: "+err.Error()).
			WithRequestID(requestID).WriteTo(w)
		return
	}

	// 步骤 7：反向代理 — 转发请求到选中的上游实例
	start := time.Now()
	targetURL := "http://" + instance.Endpoints[0]
	target, err := url.Parse(targetURL)
	if err != nil {
		response.Error(5000, "invalid upstream url: "+targetURL).
			WithRequestID(requestID).WriteTo(w)
		if ok {
			breaker.OnFailure(0)
		}
		return
	}

	// 创建反向代理，修改响应以支持路径重写
	proxy := httputil.NewSingleHostReverseProxy(target)
	
	// 路径重写：去掉网关前缀后转发给上游
	originalPath := r.URL.Path
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		// 去掉路由前缀
		req.URL.Path = strings.TrimPrefix(originalPath, route.Prefix)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		// 保留原始 Host 头
		req.Host = target.Host
	}

	// 代理错误处理：记录熔断失败
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if ok {
			breaker.OnFailure(time.Since(start))
		}
		log.Printf("[gateway] proxy error for %%s: %%v", route.Upstream, err)
		response.Error(8000, fmt.Sprintf("upstream error: %%v", err)).
			WithRequestID(requestID).WriteTo(w)
	}

	// 执行代理
	proxy.ServeHTTP(w, r)
	
	// 记录成功（用于熔断器统计）
	if ok {
		breaker.OnSuccess(time.Since(start))
	}
}
`, title, g.module, g.module, g.module, g.module, g.module, g.module, g.module, g.module,
		version, routeReg.String(), version)

	os.WriteFile(filepath.Join(g.outputDir, "main.go"), []byte(content), 0644)
}

// generateGatewayConfig 生成网关配置结构体。
func (g *GatewayGenerator) generateGatewayConfig(routes []routeInfo) {
	configDir := filepath.Join(g.outputDir, "internal", "config")
	os.MkdirAll(configDir, 0755)

	content := fmt.Sprintf(`// Package config 提供 API 网关的配置结构体。
// 支持从 YAML 文件加载，所有字段都有合理的默认值。
//
// 配置示例（etc/gateway.yaml）：
//   server:
//     name: my-gateway
//     port: 8888
//   routes:
//     - name: GetUser
//       path: /users/:id
//       method: GET
//       prefix: /api/v1
//       upstream: user-service
//       rate: 100
//       burst: 200
//       timeout: 5s
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"%s/core/balancer"
	"gopkg.in/yaml.v3"
)

// =============================================================================
// 顶层配置结构
// =============================================================================

// Config 是 API 网关的完整配置。
// 包含网关服务器、路由表、治理组件和注册中心的所有配置项。
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

// =============================================================================
// 服务配置
// =============================================================================

// ServerConfig 是网关 HTTP 服务器的配置。
type ServerConfig struct {
	Name string `+"`yaml:\"name\"`"+` // 网关名称，用于日志和健康检查
	Port int    `+"`yaml:\"port\"`"+` // 监听端口，默认 8888
}

// =============================================================================
// 路由配置
// =============================================================================

// RouteConfig 是单条路由的配置。
// 每条路由定义了一个 API 端点及其对应的上游服务。
type RouteConfig struct {
	Name     string        `+"`yaml:\"name\"`"+`     // 路由名称（唯一标识）
	Path     string        `+"`yaml:\"path\"`"+`     // 匹配路径（如 /users/:id）
	Method   string        `+"`yaml:\"method\"`"+`   // HTTP 方法（GET/POST/PUT/DELETE）
	Prefix   string        `+"`yaml:\"prefix\"`"+`   // 路由前缀（代理前会去除）
	Upstream string        `+"`yaml:\"upstream\"`"+` // 上游服务名（对应注册中心的 key）
	Rate     int           `+"`yaml:\"rate\"`"+`     // 每秒请求限流数
	Burst    int           `+"`yaml:\"burst\"`"+`    // 突发请求数
	Timeout  time.Duration `+"`yaml:\"timeout\"`"+`  // 请求超时时间
}

// =============================================================================
// 治理组件配置
// =============================================================================

// RateLimitConfig 是限流器配置。
// 采用令牌桶算法，先经过全局桶，再经过服务级别的桶。
type RateLimitConfig struct {
	GlobalRate  int `+"`yaml:\"global_rate\"`"+`  // 全局每秒请求数，默认 10000
	GlobalBurst int `+"`yaml:\"global_burst\"`"+` // 全局突发请求数，默认 20000
}

// CircuitBreakerConfig 是熔断器配置。
// 采用三态自适应熔断（Closed → Open → HalfOpen），基于滑动窗口统计。
type CircuitBreakerConfig struct {
	WindowSize  time.Duration `+"`yaml:\"window_size\"`"+`  // 滑动窗口大小，默认 10s
	BucketCount int           `+"`yaml:\"bucket_count\"`"+` // 窗口桶数量，默认 10
	MinRequests int           `+"`yaml:\"min_requests\"`"+` // 最小请求数（低于此值不触发熔断），默认 20
	ErrorRate   float64       `+"`yaml:\"error_rate\"`"+`   // 错误率阈值，默认 0.5
	SlowRate    float64       `+"`yaml:\"slow_rate\"`"+`    // 慢请求率阈值，默认 0.5
	SleepWindow time.Duration `+"`yaml:\"sleep_window\"`"+` // 熔断休眠窗口，默认 30s
}

// SheddingConfig 是过载降级器配置。
// 同时监控 CPU 使用率和内存使用率，任一超过阈值即触发降级。
type SheddingConfig struct {
	CPUThreshold float64 `+"`yaml:\"cpu_threshold\"`"+` // CPU 使用率阈值（0.0-1.0），默认 0.9
	MemThreshold float64 `+"`yaml:\"mem_threshold\"`"+` // 内存使用率阈值（0.0-1.0），默认 0.85
	Window       int     `+"`yaml:\"window\"`"+`        // 滑动窗口大小（秒），默认 5
}

// BalancerConfig 是负载均衡器配置。
// 支持四种策略：RoundRobin、WeightedRoundRobin、LeastConnection、ConsistentHash。
type BalancerConfig struct {
	Strategy balancer.Strategy `+"`yaml:\"strategy\"`"+` // 负载均衡策略
}

// =============================================================================
// 注册中心配置
// =============================================================================

// RegistryConfig 是服务注册与发现配置。
// 支持四种 Provider：static（静态列表）、etcd、consul、k8s。
type RegistryConfig struct {
	Provider        string              `+"`yaml:\"provider\"`"+`         // 注册中心类型
	StaticEndpoints map[string][]string `+"`yaml:\"static_endpoints\"`"+` // 静态端点映射（服务名→地址列表）
	EtcdEndpoints   []string            `+"`yaml:\"etcd_endpoints\"`"+`   // etcd 集群地址
	ConsulAddr      string              `+"`yaml:\"consul_addr\"`"+`      // Consul 地址
}

// =============================================================================
// 公共方法
// =============================================================================

// Load 从 YAML 文件加载网关配置。
// 加载后自动填充所有默认值。
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("config: cannot read %%s: %%w", filePath, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: cannot parse %%s: %%w", filePath, err)
	}

	cfg.setDefaults()
	return cfg, nil
}

// setDefaults 为未设置的配置项填充默认值。
// 确保所有必需字段都有合理的初始值，实现"开箱即用"。
func (c *Config) setDefaults() {
	// 服务器默认值
	if c.Server.Port == 0 {
		c.Server.Port = 8888
	}
	if c.Server.Name == "" {
		c.Server.Name = "nexus-gateway"
	}
	
	// 超时默认值
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	
	// 限流默认值
	if c.RateLimit.GlobalRate == 0 {
		c.RateLimit.GlobalRate = 10000
	}
	if c.RateLimit.GlobalBurst == 0 {
		c.RateLimit.GlobalBurst = 20000
	}
	
	// 熔断默认值
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
	
	// 降级默认值
	if c.Shedding.CPUThreshold == 0 {
		c.Shedding.CPUThreshold = 0.9
	}
	if c.Shedding.MemThreshold == 0 {
		c.Shedding.MemThreshold = 0.85
	}
	if c.Shedding.Window == 0 {
		c.Shedding.Window = 5
	}
	
	// 负载均衡默认值 — 轮询
	if c.Balancer.Strategy == 0 {
		c.Balancer.Strategy = balancer.RoundRobin
	}
	
	// 注册中心默认值
	if c.Registry.StaticEndpoints == nil {
		c.Registry.StaticEndpoints = make(map[string][]string)
	}
	
	// 路由默认值
	for i := range c.Routes {
		if c.Routes[i].Rate == 0 {
			c.Routes[i].Rate = 100
		}
		if c.Routes[i].Burst == 0 {
			c.Routes[i].Burst = 200
		}
		if c.Routes[i].Timeout == 0 {
			c.Routes[i].Timeout = 5 * time.Second
		}
	}
}

// FindRoute 根据 HTTP 方法和路径查找匹配路由。
// 支持精确匹配和 :param 路径参数匹配。
// 返回 nil 表示未找到匹配路由。
func (c *Config) FindRoute(method, path string) *RouteConfig {
	for i := range c.Routes {
		route := &c.Routes[i]
		// 先匹配 HTTP 方法
		if !strings.EqualFold(route.Method, method) {
			continue
		}
		// 再匹配路径（支持 :param 占位符）
		if matchPath(route.Prefix+route.Path, path) {
			return route
		}
	}
	return nil
}

// matchPath 简化的路径匹配函数。
// 支持 :param 占位符匹配任意路径段。
// 例如：/users/:id 匹配 /users/123 但不匹配 /users/123/posts。
//
// TODO: 后续版本支持通配符 * 和正则表达式匹配。
func matchPath(pattern, actual string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	actualParts := strings.Split(strings.Trim(actual, "/"), "/")

	if len(patternParts) != len(actualParts) {
		return false
	}

	for i, part := range patternParts {
		// :param 占位符匹配任意非空值
		if strings.HasPrefix(part, ":") {
			if actualParts[i] == "" {
				return false
			}
			continue
		}
		// 精确匹配
		if part != actualParts[i] {
			return false
		}
	}

	return true
}
`, g.module)

	os.WriteFile(filepath.Join(configDir, "config.go"), []byte(content), 0644)
}

// generateProxyHandler 生成代理处理器（预留，实际上 proxy 逻辑已嵌入 main.go）。
// 此方法为将来将 ProxyHandler 提取为独立文件做准备。
func (g *GatewayGenerator) generateProxyHandler(routes []routeInfo) {
	handlerDir := filepath.Join(g.outputDir, "internal", "handler")
	os.MkdirAll(handlerDir, 0755)

	content := `// Package handler 提供网关代理处理器。
// ProxyHandler 集成完整的治理流水线，对上游微服务进行透明代理。
//
// 预留此包供用户自定义处理逻辑。
// 默认的代理处理器已嵌入 main.go，如需自定义可在此扩展。
package handler
`
	os.WriteFile(filepath.Join(handlerDir, "proxy.go"), []byte(content), 0644)
}

// generateGatewayYAML 生成网关的默认 YAML 配置文件。
func (g *GatewayGenerator) generateGatewayYAML(file *ast.File, routes []routeInfo) {
	etcDir := filepath.Join(g.outputDir, "etc")
	os.MkdirAll(etcDir, 0755)

	// 构建路由条目
	var routeYAML strings.Builder
	for _, route := range routes {
		routeYAML.WriteString(fmt.Sprintf(`  - name: %s
    path: %s
    method: %s
    prefix: %s
    upstream: %s
    rate: 100
    burst: 200
    timeout: 5s
`, route.Name, route.Path, route.Method, route.Prefix, route.Upstream))
	}

	info := file.Info
	title := "Nexus Gateway"
	if info != nil && info.Title != "" {
		title = info.Title
	}

	content := fmt.Sprintf(`# %s 配置
# 由 nx gen gateway 自动生成，可根据需要修改。
#
# 治理流水线说明：
#   请求 → 限流(RateLimit) → 降级(Shedding) → 熔断(CircuitBreaker)
#        → 服务发现(Discovery) → 负载均衡(Balancer) → 反向代理 → 响应

# ---------------------------------------------------------------------------
# 网关服务配置
# ---------------------------------------------------------------------------
server:
  name: nexus-gateway  # 网关名称
  port: 8888           # 监听端口

# ---------------------------------------------------------------------------
# 路由配置（自动从 .api DSL 生成）
# 每条路由定义了一个 API 端点及其对应的上游服务
# ---------------------------------------------------------------------------
routes:
%s
# ---------------------------------------------------------------------------
# 限流配置 — 令牌桶算法
# 请求先经过全局桶，再经过每个路由的独立桶
# ---------------------------------------------------------------------------
ratelimit:
  global_rate: 10000   # 全局每秒请求数
  global_burst: 20000  # 全局突发请求数

# ---------------------------------------------------------------------------
# 熔断配置 — 三态自适应熔断器
# Closed(正常) → Open(熔断) → HalfOpen(半开探测) → Closed
# ---------------------------------------------------------------------------
circuit_breaker:
  window_size: 10s     # 统计窗口大小
  bucket_count: 10     # 窗口中的桶数量
  min_requests: 20     # 最小请求数（低于此值不触发）
  error_rate: 0.5      # 错误率阈值（0.0-1.0）
  slow_rate: 0.5       # 慢请求率阈值（0.0-1.0）
  sleep_window: 30s    # 熔断后休眠时间

# ---------------------------------------------------------------------------
# 降级配置 — CPU + 内存双层过载保护
# 任一资源超过阈值即触发降级，返回 503
# ---------------------------------------------------------------------------
shedding:
  cpu_threshold: 0.9   # CPU 使用率阈值
  mem_threshold: 0.85  # 内存使用率阈值
  window: 5            # 滑动窗口（秒）

# ---------------------------------------------------------------------------
# 负载均衡配置
# round_robin:     轮询（默认）
# least_connection: 最少连接数
# consistent_hash:  一致性哈希（适合有状态服务）
# weighted_round_robin: 加权轮询
# ---------------------------------------------------------------------------
balancer:
  strategy: round_robin

# ---------------------------------------------------------------------------
# 服务注册与发现配置
# provider: static | etcd | consul | k8s
# ---------------------------------------------------------------------------
registry:
  provider: static
  static_endpoints:
    # 在此配置上游服务的地址列表
    # 格式: <service-name>:
    #   - host:port
    #   - host:port
    user-service:
      - localhost:8081
      - localhost:8082
    order-service:
      - localhost:8083
      - localhost:8084

# ---------------------------------------------------------------------------
# 超时配置
# ---------------------------------------------------------------------------
timeout: 30s
`, title, routeYAML.String())

	os.WriteFile(filepath.Join(etcDir, "gateway.yaml"), []byte(content), 0644)
}

// toSnakeCase 将 PascalCase 转换为 snake_case（委托给 util 包）。
func toSnakeCase(s string) string {
	return util.ToSnakeCase(s)
}

// goTypeToProto 将 Go 类型转换为 Proto 类型（委托给 util 包）。
func goTypeToProto(goType string) string {
	return util.GoTypeToProto(goType)
}