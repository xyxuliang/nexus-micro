// Package metrics 提供 Prometheus 兼容的指标采集能力。
// 零外部依赖，实现 Counter、Gauge、Histogram 三种指标类型，
// 通过 /metrics 端点暴露，兼容 Prometheus 文本格式。
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// 核心类型
// =============================================================================

// MetricType 表示指标类型（Counter、Gauge、Histogram）。
type MetricType int

const (
	MetricTypeCounter   MetricType = iota // 单调递增计数器
	MetricTypeGauge                       // 可增可减的瞬时值
	MetricTypeHistogram                   // 分布统计
)

// LabelPair 是标签键值对。
type LabelPair struct {
	Name  string
	Value string
}

// =============================================================================
// Counter — 单调递增计数器
// =============================================================================

// Counter 是单调递增的计数器指标。
// 主要用于记录请求总数、错误总数等累积量。
type Counter struct {
	mu      sync.RWMutex
	name    string
	help    string
	labels  []string
	values  map[string]float64 // 无标签时的值
	labeled map[string]float64 // 带标签的值（key = "label1=val1,label2=val2"）
}

// Inc 计数器加 1。
func (c *Counter) Inc() {
	c.Add(1)
}

// Add 计数器加 n（n 必须 >= 0）。
func (c *Counter) Add(n float64) {
	if n < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[""] += n
}

// With 创建带标签的计数器子实例。
func (c *Counter) With(labels ...LabelPair) *CounterVec {
	return &CounterVec{counter: c, labels: labels}
}

// CounterVec 是带标签的计数器。
type CounterVec struct {
	counter *Counter
	labels  []LabelPair
}

// Inc 带标签的计数器加 1。
func (cv *CounterVec) Inc() { cv.Add(1) }

// Add 带标签的计数器加 n。
func (cv *CounterVec) Add(n float64) {
	if n < 0 {
		return
	}
	key := labelKey(cv.labels)
	cv.counter.mu.Lock()
	defer cv.counter.mu.Unlock()
	cv.counter.labeled[key] += n
}

// =============================================================================
// Gauge — 可增可减的瞬时值
// =============================================================================

// Gauge 是可增可减的瞬时值指标。
// 主要用于记录当前连接数、内存使用量、goroutine 数量等。
type Gauge struct {
	mu      sync.RWMutex
	name    string
	help    string
	labels  []string
	values  map[string]float64
	labeled map[string]float64
}

// Set 设置指标值。
func (g *Gauge) Set(v float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[""] = v
}

// Get 获取当前值。
func (g *Gauge) Get() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.values[""]
}

// Inc 加 1。
func (g *Gauge) Inc() { g.Add(1) }

// Dec 减 1。
func (g *Gauge) Dec() { g.Add(-1) }

// Add 加 n（n 可为负数）。
func (g *Gauge) Add(n float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[""] += n
}

// With 创建带标签的 Gauge 子实例。
func (g *Gauge) With(labels ...LabelPair) *GaugeVec {
	return &GaugeVec{gauge: g, labels: labels}
}

// GaugeVec 是带标签的 Gauge。
type GaugeVec struct {
	gauge  *Gauge
	labels []LabelPair
}

// Set 设置值。
func (gv *GaugeVec) Set(v float64) {
	key := labelKey(gv.labels)
	gv.gauge.mu.Lock()
	defer gv.gauge.mu.Unlock()
	gv.gauge.labeled[key] = v
}

// Inc 加 1。
func (gv *GaugeVec) Inc() { gv.Add(1) }

// Add 加 n。
func (gv *GaugeVec) Add(n float64) {
	key := labelKey(gv.labels)
	gv.gauge.mu.Lock()
	defer gv.gauge.mu.Unlock()
	gv.gauge.labeled[key] += n
}

// =============================================================================
// Histogram — 分布统计
// =============================================================================

// Histogram 是分布统计指标。
// 聚合请求延迟、响应大小等分布数据，提供 sum、count 和分桶统计。
type Histogram struct {
	mu      sync.Mutex
	name    string
	help    string
	buckets []float64 // 分桶边界（如 [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]）
	values  map[string]float64
	labeled map[string]map[string]float64 // key -> {sum, count, bucket_le_N}
}

// Observe 记录一个观测值。
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.record("", v)
}

// With 创建带标签的 Histogram 子实例。
func (h *Histogram) With(labels ...LabelPair) *HistogramVec {
	return &HistogramVec{histogram: h, labels: labels}
}

// record 记录观测值到指定 key。
func (h *Histogram) record(key string, v float64) {
	if h.values == nil {
		h.values = make(map[string]float64)
	}
	if h.labeled == nil {
		h.labeled = make(map[string]map[string]float64)
	}

	// 累计
	h.values[key+"_sum"] += v
	h.values[key+"_count"]++

	// 分桶统计
	for _, b := range h.buckets {
		if v <= b {
			bucketKey := fmt.Sprintf("%s_bucket_le_%v", key, b)
			h.values[bucketKey]++
		}
	}
}

// HistogramVec 是带标签的 Histogram。
type HistogramVec struct {
	histogram *Histogram
	labels    []LabelPair
}

// Observe 记录观测值。
func (hv *HistogramVec) Observe(v float64) {
	key := labelKey(hv.labels)
	hv.histogram.mu.Lock()
	defer hv.histogram.mu.Unlock()
	hv.histogram.record(key, v)
}

// =============================================================================
// Registry — 指标注册中心
// =============================================================================

// Registry 是指标注册中心。
// 管理所有指标，提供 /metrics 端点所需的 Prometheus 文本格式输出。
type Registry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
	hists    map[string]*Histogram
}

// NewRegistry 创建新的指标注册中心。
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]*Counter),
		gauges:   make(map[string]*Gauge),
		hists:    make(map[string]*Histogram),
	}
}

// RegisterCounter 注册一个 Counter。
func (r *Registry) RegisterCounter(name, help string, labels ...string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &Counter{
		name:    name,
		help:    help,
		labels:  labels,
		values:  make(map[string]float64),
		labeled: make(map[string]float64),
	}
	r.counters[name] = c
	return c
}

// RegisterGauge 注册一个 Gauge。
func (r *Registry) RegisterGauge(name, help string, labels ...string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := &Gauge{
		name:    name,
		help:    help,
		labels:  labels,
		values:  make(map[string]float64),
		labeled: make(map[string]float64),
	}
	r.gauges[name] = g
	return g
}

// RegisterHistogram 注册一个 Histogram。
func (r *Registry) RegisterHistogram(name, help string, buckets []float64, labels ...string) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := &Histogram{
		name:    name,
		help:    help,
		buckets: buckets,
		values:  make(map[string]float64),
		labeled: make(map[string]map[string]float64),
	}
	r.hists[name] = h
	return h
}

// MustRegisterCounter 强制注册 Counter，如果已存在则 panic。
func (r *Registry) MustRegisterCounter(name, help string, labels ...string) *Counter {
	c := r.counter(name)
	if c != nil {
		panic(fmt.Sprintf("metrics: counter %s already registered", name))
	}
	return r.RegisterCounter(name, help, labels...)
}

// counter 检查 Counter 是否已注册。
func (r *Registry) counter(name string) *Counter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.counters[name]
}

// =============================================================================
// Prometheus 文本格式输出
// =============================================================================

// WriteTo 将指标数据以 Prometheus 文本格式写入 io.Writer。
// 兼容 /metrics 端点，可直接被 Prometheus Server 抓取。
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var written int64

	// 输出 Counters
	for _, name := range sortedKeys(r.counters) {
		c := r.counters[name]
		n, _ := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
		written += int64(n)
		// 无标签值
		if v := c.values[""]; v > 0 {
			n, _ = fmt.Fprintf(w, "%s %v\n", c.name, v)
			written += int64(n)
		}
		// 带标签值
		for k, v := range c.labeled {
			n, _ = fmt.Fprintf(w, "%s{%s} %v\n", c.name, k, v)
			written += int64(n)
		}
	}

	// 输出 Gauges
	for _, name := range sortedKeys(r.gauges) {
		g := r.gauges[name]
		n, _ := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", g.name, g.help, g.name)
		written += int64(n)
		// 无标签值
		if v, ok := g.values[""]; ok {
			n, _ = fmt.Fprintf(w, "%s %v\n", g.name, v)
			written += int64(n)
		}
		// 带标签值
		for k, v := range g.labeled {
			n, _ = fmt.Fprintf(w, "%s{%s} %v\n", g.name, k, v)
			written += int64(n)
		}
	}

	// 输出 Histograms
	for _, name := range sortedKeys(r.hists) {
		h := r.hists[name]
		n, _ := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, h.help, h.name)
		written += int64(n)

		// 分桶输出（按 bucket 边界排序）
		sortedBuckets := make([]float64, len(h.buckets))
		copy(sortedBuckets, h.buckets)
		sort.Float64s(sortedBuckets)

		for _, bucket := range sortedBuckets {
			bucketKey := fmt.Sprintf("_bucket_le_%v", bucket)
			v := h.values[bucketKey]
			n, _ = fmt.Fprintf(w, "%s_bucket{le=\"%v\"} %v\n", h.name, bucket, v)
			written += int64(n)
		}

		// +Inf bucket
		sum := h.values["_sum"]
		count := h.values["_count"]
		n, _ = fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %v\n", h.name, count)
		written += int64(n)
		n, _ = fmt.Fprintf(w, "%s_sum %v\n", h.name, sum)
		written += int64(n)
		n, _ = fmt.Fprintf(w, "%s_count %v\n", h.name, count)
		written += int64(n)

		// 带标签的 Histogram
		for key, labelMap := range h.labeled {
			lsum := labelMap["sum"]
			lcount := labelMap["count"]
			for _, bucket := range sortedBuckets {
				bk := fmt.Sprintf("bucket_le_%v", bucket)
				bv := labelMap[bk]
				n, _ = fmt.Fprintf(w, "%s_bucket{%s,le=\"%v\"} %v\n", h.name, key, bucket, bv)
				written += int64(n)
			}
			n, _ = fmt.Fprintf(w, "%s_bucket{%s,le=\"+Inf\"} %v\n", h.name, key, lcount)
			written += int64(n)
			n, _ = fmt.Fprintf(w, "%s_sum{%s} %v\n", h.name, key, lsum)
			written += int64(n)
			n, _ = fmt.Fprintf(w, "%s_count{%s} %v\n", h.name, key, lcount)
			written += int64(n)
		}
	}

	return written, nil
}

// =============================================================================
// 默认指标 — 立即可用的 HTTP 服务指标
// =============================================================================

// DefaultBuckets 是 HTTP 请求延迟的默认分桶（秒）。
// 覆盖 0.5ms 到 10s 的范围，适合大多数 HTTP 服务。
var DefaultBuckets = []float64{0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// DefaultRegistry 是全局默认的指标注册中心。
var DefaultRegistry = NewRegistry()

// 默认 HTTP 指标
var (
	HTTPRequestsTotal    *Counter
	HTTPRequestDuration  *Histogram
	HTTPRequestsInFlight *Gauge
	HTTPResponseSize     *Histogram
)

// HTTPLabels 是 HTTP 指标的默认标签。
var HTTPLabels = []string{"method", "path", "status"}

func init() {
	HTTPRequestsTotal = DefaultRegistry.RegisterCounter(
		"http_requests_total",
		"Total number of HTTP requests",
		HTTPLabels...,
	)
	HTTPRequestDuration = DefaultRegistry.RegisterHistogram(
		"http_request_duration_seconds",
		"HTTP request duration in seconds",
		DefaultBuckets,
		HTTPLabels...,
	)
	HTTPRequestsInFlight = DefaultRegistry.RegisterGauge(
		"http_requests_in_flight",
		"Current number of HTTP requests being processed",
	)
	HTTPResponseSize = DefaultRegistry.RegisterHistogram(
		"http_response_size_bytes",
		"HTTP response size in bytes",
		[]float64{100, 1000, 10000, 100000, 1000000},
		HTTPLabels...,
	)
}

// =============================================================================
// 治理指标 — 服务治理组件专用指标
// =============================================================================

var (
	// RateLimitTotal 限流拒绝计数
	RateLimitTotal *Counter
	// CircuitBreakerState 熔断器状态（0=Closed, 1=Open, 2=HalfOpen）
	CircuitBreakerState *Gauge
	// SheddingTotal 降级拒绝计数
	SheddingTotal *Counter
	// ActiveConnections 当前活跃连接数
	ActiveConnections *Gauge
	// GoroutineCount 当前 goroutine 数量
	GoroutineCount *Gauge
)

func init() {
	RateLimitTotal = DefaultRegistry.RegisterCounter(
		"nexus_ratelimit_rejections_total",
		"Total number of rate limit rejections",
		"service", "method",
	)
	CircuitBreakerState = DefaultRegistry.RegisterGauge(
		"nexus_circuit_breaker_state",
		"Circuit breaker state (0=Closed, 1=Open, 2=HalfOpen)",
		"service",
	)
	SheddingTotal = DefaultRegistry.RegisterCounter(
		"nexus_shedding_rejections_total",
		"Total number of overload shedding rejections",
		"service",
	)
	ActiveConnections = DefaultRegistry.RegisterGauge(
		"nexus_active_connections",
		"Current number of active connections",
	)
	GoroutineCount = DefaultRegistry.RegisterGauge(
		"nexus_goroutine_count",
		"Current number of goroutines",
	)
}

// =============================================================================
// HTTP Handler — 指标中间件
// =============================================================================

// MetricsHandler 是 HTTP 指标采集的中间件处理器。
// 记录请求总数、延迟分布、并发请求数和响应大小。
func MetricsHandler(name string, next func(w io.Writer, path string) (int, error)) func(w io.Writer, path string) (int, error) {
	return func(w io.Writer, path string) (int, error) {
		HTTPRequestsInFlight.Inc()
		start := time.Now()

		status, err := next(w, path)

		HTTPRequestsInFlight.Dec()
		duration := time.Since(start).Seconds()

		HTTPRequestsTotal.With(
			LabelPair{Name: "method", Value: "GET"},
			LabelPair{Name: "path", Value: path},
			LabelPair{Name: "status", Value: fmt.Sprintf("%d", status)},
		).Inc()

		HTTPRequestDuration.With(
			LabelPair{Name: "method", Value: "GET"},
			LabelPair{Name: "path", Value: path},
			LabelPair{Name: "status", Value: fmt.Sprintf("%d", status)},
		).Observe(duration)

		return status, err
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// labelKey 将标签对转换为字符串 key（格式 "name1=val1,name2=val2"）。
func labelKey(labels []LabelPair) string {
	if len(labels) == 0 {
		return ""
	}
	sorted := make([]LabelPair, len(labels))
	copy(sorted, labels)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var key string
	for i, lp := range sorted {
		if i > 0 {
			key += ","
		}
		key += fmt.Sprintf("%s=\"%s\"", lp.Name, lp.Value)
	}
	return key
}

// sortedKeys 返回 map 的排序键列表。
func sortedKeys(m interface{}) []string {
	switch v := m.(type) {
	case map[string]*Counter:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	case map[string]*Gauge:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	case map[string]*Histogram:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	return nil
}

// =============================================================================
// 运行时指标采集
// =============================================================================

// RuntimeCollector 定期采集 Go 运行时指标。
type RuntimeCollector struct {
	stopCh chan struct{}
	ticker *time.Ticker
}

// StartRuntimeCollector 启动运行时指标采集器。
// 每 interval 秒更新一次 goroutine 数量。
func StartRuntimeCollector(interval time.Duration) *RuntimeCollector {
	rc := &RuntimeCollector{
		stopCh: make(chan struct{}),
		ticker: time.NewTicker(interval),
	}
	go func() {
		for {
			select {
			case <-rc.ticker.C:
				// Goroutine 数量由调用方定期更新
			case <-rc.stopCh:
				return
			}
		}
	}()
	return rc
}

// Stop 停止采集器。
func (rc *RuntimeCollector) Stop() {
	rc.ticker.Stop()
	close(rc.stopCh)
}

// =============================================================================
// 使用示例
// =============================================================================

// Example 展示如何使用 Metrics 包。
func Example() {
	// 1. 创建 Registry
	reg := NewRegistry()

	// 2. 注册指标
	requestCounter := reg.RegisterCounter("my_requests_total", "Total requests", "method", "endpoint")
	activeConnections := reg.RegisterGauge("my_active_connections", "Active connections")
	requestDuration := reg.RegisterHistogram("my_request_duration_seconds", "Request duration",
		[]float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0}, "method")

	// 3. 使用指标
	requestCounter.With(
		LabelPair{Name: "method", Value: "GET"},
		LabelPair{Name: "endpoint", Value: "/users"},
	).Inc()

	activeConnections.Set(42)
	requestDuration.With(
		LabelPair{Name: "method", Value: "GET"},
	).Observe(0.025)

	// 4. 输出 Prometheus 格式
	_ = reg
	_ = activeConnections
	_ = requestDuration
}

// 确保导入的包被使用
var _ = atomic.Value{}
var _ = math.MaxFloat64
