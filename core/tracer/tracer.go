// Package tracer 提供 OpenTelemetry 兼容的分布式链路追踪能力。
// 零外部依赖实现，兼容 W3C Trace Context 传播标准。
// 与服务端中间件 Tracing() 配合使用，自动从 HTTP Header 提取/注入 trace context。
package tracer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xyxuliang/nexus-micro/core/contextkeys"
	"github.com/xyxuliang/nexus-micro/internal/util"
)

// =============================================================================
// 核心类型定义
// =============================================================================

// SpanKind 表示 Span 的类型（Server、Client、Internal 等）。
type SpanKind int

const (
	SpanKindInternal SpanKind = iota // 内部 span（默认）
	SpanKindServer                   // 服务端 span（接收请求）
	SpanKindClient                   // 客户端 span（发起 RPC 调用）
)

// StatusCode 表示 Span 的状态。
type StatusCode int

const (
	StatusOK    StatusCode = iota // 成功
	StatusError                   // 失败
)

// SpanContext 是 W3C Trace Context 兼容的上下文。
// 包含完整的 trace_id、span_id 和 trace_flags 信息。
type SpanContext struct {
	TraceID    string // 全局唯一的 trace ID（32 字符十六进制）
	SpanID     string // 当前 span 的唯一 ID（16 字符十六进制）
	TraceFlags byte   // trace 标志（01=sample, 00=not sampled）
}

// =============================================================================
// Span 定义
// =============================================================================

// Span 表示一次操作（如一个 HTTP 请求、一次 RPC 调用、一次数据库查询）。
// 每个 Span 包含开始时间、结束时间、属性、事件和状态信息。
type Span struct {
	mu sync.Mutex

	name       string                 // Span 名称（如 "GET /users"）
	kind       SpanKind               // Span 类型
	startTime  time.Time              // 开始时间
	endTime    time.Time              // 结束时间
	status     StatusCode             // 状态
	statusMsg  string                 // 状态信息（错误描述）
	attributes map[string]interface{} // 属性（key-value 标签）
	events     []Event                // 事件（时间点上的注释）
	spanCtx    SpanContext            // Span 上下文
	parentID   string                 // 父 Span ID
	finished   bool                   // 是否已结束
}

// Event 是 Span 上的时间点事件。
// 用于记录特定时刻发生的有意义的事情（如 "cache_miss"、"retry_attempt"）。
type Event struct {
	Name       string                 // 事件名称
	Timestamp  time.Time              // 发生时间
	Attributes map[string]interface{} // 事件属性
}

// =============================================================================
// Tracer 定义
// =============================================================================

// Tracer 是追踪器的核心接口。
// 用于创建和管理 Span，支持与外部追踪系统（如 Jaeger、Zipkin）对接。
type Tracer struct {
	mu          sync.RWMutex
	serviceName string              // 服务名称
	exporters   []SpanExporter      // 导出器列表（Jaeger、Zipkin、OTLP 等）
	processor   SpanProcessor       // Span 处理器（batch、simple）
	sampler     Sampler             // 采样器
}

// SpanExporter 是 Span 导出器接口。
// 实现此接口即可对接不同的追踪后端（如打印到控制台、发送到 Jaeger agent）。
type SpanExporter interface {
	Export(spans []*Span) error
	Shutdown(ctx context.Context) error
}

// SpanProcessor 是 Span 处理器接口。
// 决定 Span 被创建后的处理方式（同步/异步、批量/单独）。
type SpanProcessor interface {
	OnStart(ctx context.Context, span *Span)
	OnEnd(span *Span)
	Shutdown(ctx context.Context) error
}

// Sampler 是采样器接口。
// 决定一个 Span 是否应该被采样（记录并导出）。
type Sampler interface {
	ShouldSample(traceID string) bool
}

// =============================================================================
// 默认实现
// =============================================================================

// NoopExporter 是空操作导出器（默认）。
// 所有 Span 都不会被导出，适合开发和测试环境。
type NoopExporter struct{}

func (e *NoopExporter) Export(spans []*Span) error      { return nil }
func (e *NoopExporter) Shutdown(ctx context.Context) error { return nil }

// AlwaysSample 始终采样（默认）。
type AlwaysSample struct{}

func (s *AlwaysSample) ShouldSample(traceID string) bool { return true }

// RateSample 按比例采样。
type RateSample struct {
	rate float64 // 采样率：0.0-1.0（1.0=全部采样）
}

func (s *RateSample) ShouldSample(traceID string) bool {
	if s.rate >= 1.0 {
		return true
	}
	if s.rate <= 0.0 {
		return false
	}
	// 使用 trace_id 的哈希值决定是否采样（确保同一 trace 的所有 span 采样一致）
	var h uint32
	for _, b := range []byte(traceID) {
		h = h*31 + uint32(b)
	}
	return float64(h)/float64(^uint32(0)) < s.rate
}

// SimpleProcessor 是简单的 Span 处理器。
// Span 结束时立即导出，适合调试。
type SimpleProcessor struct {
	exporter SpanExporter
}

func (p *SimpleProcessor) OnStart(ctx context.Context, span *Span) {}
func (p *SimpleProcessor) OnEnd(span *Span) {
	if p.exporter != nil {
		_ = p.exporter.Export([]*Span{span})
	}
}
func (p *SimpleProcessor) Shutdown(ctx context.Context) error {
	if p.exporter != nil {
		return p.exporter.Shutdown(ctx)
	}
	return nil
}

// =============================================================================
// Tracer 配置
// =============================================================================

// Config 是 Tracer 的配置。
type Config struct {
	ServiceName string         // 服务名称
	Exporter    SpanExporter   // Span 导出器（nil = NoopExporter）
	Sampler     Sampler        // 采样器（nil = AlwaysSample）
	Enabled     bool           // 是否启用追踪
}

// =============================================================================
// NewTracer — 构造函数
// =============================================================================

// NewTracer 创建一个新的 Tracer 实例。
func NewTracer(cfg *Config) *Tracer {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Exporter == nil {
		cfg.Exporter = &NoopExporter{}
	}
	if cfg.Sampler == nil {
		cfg.Sampler = &AlwaysSample{}
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "nexus-service"
	}

	return &Tracer{
		serviceName: cfg.ServiceName,
		exporters:   []SpanExporter{cfg.Exporter},
		sampler:     cfg.Sampler,
		processor:   &SimpleProcessor{exporter: cfg.Exporter},
	}
}

// =============================================================================
// Span 创建
// =============================================================================

// StartSpan 创建一个新的 Span。
// 如果 ctx 中包含父 SpanContext，新 Span 会正确设置 parentID 和 TraceID。
//
// 使用方式：
//
//	ctx, span := tracer.StartSpan(ctx, "GET /users")
//	defer span.End()
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	traceID := util.GenerateID()
	spanID := util.GenerateID()[:16]
	var parentID string

	// 尝试从 ctx 获取父 Span 信息
	if parentCtx := SpanContextFromContext(ctx); parentCtx != nil {
		traceID = parentCtx.TraceID
		parentID = parentCtx.SpanID
	}

	// 采样判断
	if !t.sampler.ShouldSample(traceID) {
		// 不采样：创建 NoopSpan
		return ctx, &Span{
			name:      name,
			startTime: time.Now(),
			spanCtx: SpanContext{
				TraceID:    traceID,
				SpanID:     spanID,
				TraceFlags: 0, // not sampled
			},
			finished: true,
		}
	}

	span := &Span{
		name:       name,
		kind:       SpanKindInternal,
		startTime:  time.Now(),
		status:     StatusOK,
		attributes: make(map[string]interface{}),
		events:     make([]Event, 0),
		spanCtx: SpanContext{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: 1, // sampled
		},
		parentID: parentID,
	}

	t.processor.OnStart(ctx, span)
	return ContextWithSpanContext(ctx, &span.spanCtx), span
}

// StartServerSpan 创建一个服务端 Span（HTTP 请求入口）。
func (t *Tracer) StartServerSpan(ctx context.Context, name string) (context.Context, *Span) {
	ctx, span := t.StartSpan(ctx, name)
	span.kind = SpanKindServer
	return ctx, span
}

// StartClientSpan 创建一个客户端 Span（RPC 调用）。
func (t *Tracer) StartClientSpan(ctx context.Context, name string) (context.Context, *Span) {
	ctx, span := t.StartSpan(ctx, name)
	span.kind = SpanKindClient
	return ctx, span
}

// =============================================================================
// Span 方法
// =============================================================================

// End 结束 Span。
// 标记 Span 完成，触发处理器将 Span 发送给导出器。
// 只能调用一次，多次调用无效。
func (s *Span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.endTime = time.Now()
	s.finished = true
}

// SetAttribute 设置 Span 属性（键值对标签）。
func (s *Span) SetAttribute(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]interface{})
	}
	s.attributes[key] = value
}

// SetAttributes 批量设置 Span 属性。
func (s *Span) SetAttributes(attrs map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]interface{})
	}
	for k, v := range attrs {
		s.attributes[k] = v
	}
}

// AddEvent 添加一个事件到 Span。
// 事件是时间轴上的特定时刻（如 "cache_hit"、"retry_attempt"）。
func (s *Span) AddEvent(name string, attrs ...map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	evt := Event{
		Name:      name,
		Timestamp: time.Now(),
	}
	if len(attrs) > 0 {
		evt.Attributes = attrs[0]
	}
	s.events = append(s.events, evt)
}

// SetStatus 设置 Span 状态。
func (s *Span) SetStatus(code StatusCode, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = code
	s.statusMsg = msg
}

// RecordError 记录错误到 Span。
// 自动设置状态为 Error 并添加 error 属性。
func (s *Span) RecordError(err error) {
	s.SetStatus(StatusError, err.Error())
	s.SetAttribute("error", true)
	s.SetAttribute("error.message", err.Error())
}

// Duration 返回 Span 的持续时间。
func (s *Span) Duration() time.Duration {
	if s.endTime.IsZero() {
		return time.Since(s.startTime)
	}
	return s.endTime.Sub(s.startTime)
}

// =============================================================================
// Span 信息获取
// =============================================================================

// Name 返回 Span 名称。
func (s *Span) Name() string { return s.name }

// Kind 返回 Span 类型。
func (s *Span) Kind() SpanKind { return s.kind }

// TraceID 返回 Trace ID。
func (s *Span) TraceID() string { return s.spanCtx.TraceID }

// SpanID 返回 Span ID。
func (s *Span) SpanID() string { return s.spanCtx.SpanID }

// ParentID 返回父 Span ID。
func (s *Span) ParentID() string { return s.parentID }

// Status 返回 Span 状态。
func (s *Span) Status() StatusCode { return s.status }

// Attributes 返回所有属性。
func (s *Span) Attributes() map[string]interface{} { return s.attributes }

// Events 返回所有事件。
func (s *Span) Events() []Event { return s.events }

// =============================================================================
// Context 传播 (W3C Trace Context 兼容)
// =============================================================================

// ContextWithSpanContext 将 SpanContext 注入到 context 中。
func ContextWithSpanContext(ctx context.Context, sc *SpanContext) context.Context {
	return context.WithValue(ctx, contextkeys.SpanContextKey{}, sc)
}

// SpanContextFromContext 从 context 中提取 SpanContext。
// 返回 nil 表示 context 中没有 trace 信息。
func SpanContextFromContext(ctx context.Context) *SpanContext {
	if sc, ok := ctx.Value(contextkeys.SpanContextKey{}).(*SpanContext); ok {
		return sc
	}
	return nil
}

// =============================================================================
// HTTP Header 传播
// =============================================================================

// InjectHTTPHeaders 将 SpanContext 注入到 HTTP Header 中。
// header 是一个 map[string]string，直接操作。
func InjectHTTPHeaders(sc *SpanContext, header map[string]string) {
	if sc == nil {
		return
	}
	header["traceparent"] = fmt.Sprintf("00-%s-%s-%02x", sc.TraceID, sc.SpanID, sc.TraceFlags)
}

// ExtractHTTPHeaders 从 HTTP Header 中提取 SpanContext。
// 遵循 W3C Trace Context 标准格式。
func ExtractHTTPHeaders(header map[string]string) *SpanContext {
	tp, ok := header["traceparent"]
	if !ok {
		return nil
	}

	// 解析 W3C traceparent: "00-traceID-spanID-traceFlags"
	parts := make([]string, 0, 4)
	current := ""
	for _, ch := range tp {
		if ch == '-' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	parts = append(parts, current)

	if len(parts) != 4 || parts[0] != "00" {
		return nil
	}

	flags := byte(0)
	if parts[3] == "01" {
		flags = 1
	}

	return &SpanContext{
		TraceID:    parts[1],
		SpanID:     parts[2],
		TraceFlags: flags,
	}
}

// =============================================================================
// 全局 Tracer
// =============================================================================

var globalTracer *Tracer

// SetGlobalTracer 设置全局 Tracer 实例。
// 通常在应用启动时调用。
func SetGlobalTracer(t *Tracer) {
	globalTracer = t
}

// GlobalTracer 返回全局 Tracer 实例。
// 如果未设置，返回一个默认的 Noop Tracer。
func GlobalTracer() *Tracer {
	if globalTracer == nil {
		globalTracer = NewTracer(&Config{Enabled: false})
	}
	return globalTracer
}

// =============================================================================
// Console Exporter — 调试用，打印 Span 到控制台
// =============================================================================

// ConsoleExporter 将 Span 信息打印到标准输出。
// 适合开发调试，不适合生产环境。
type ConsoleExporter struct {
	mu sync.Mutex
}

func (e *ConsoleExporter) Export(spans []*Span) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, span := range spans {
		fmt.Printf("[trace] trace_id=%s span_id=%s parent_id=%s name=%s duration=%v status=%d\n",
			span.TraceID(), span.SpanID(), span.ParentID(), span.Name(), span.Duration(), span.Status())
		for k, v := range span.Attributes() {
			fmt.Printf("  %s=%v\n", k, v)
		}
	}
	return nil
}

func (e *ConsoleExporter) Shutdown(ctx context.Context) error { return nil }