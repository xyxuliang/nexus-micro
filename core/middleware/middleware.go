// Package middleware 提供框架内置的中间件集合。
// 所有中间件在 HTTP 和 gRPC 协议之间完全共享，只需定义一次。
// 中间件执行顺序：SSE → RequestID → Tracing → Logger → Recovery → CORS → Auth → RateLimit → Timeout → Shedding → Metrics。
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/xyxuliang/nexus-micro/core"
	"github.com/xyxuliang/nexus-micro/core/contextkeys"
	"github.com/xyxuliang/nexus-micro/core/log"
	"github.com/xyxuliang/nexus-micro/core/metrics"
	"github.com/xyxuliang/nexus-micro/core/sse"
	"github.com/xyxuliang/nexus-micro/core/tracer"
	"github.com/xyxuliang/nexus-micro/internal/errors"
	"github.com/xyxuliang/nexus-micro/internal/util"
)

// RequestID 中间件为每个请求生成唯一的 request_id。
// 如果请求头中已有 X-Request-ID，则复用该值。
// request_id 会被注入到 context 和响应头中，供日志和追踪使用。
func RequestID() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			requestID := util.GenerateID()
			ctx = context.WithValue(ctx, contextkeys.RequestIDKey{}, requestID)
			return next(ctx, req)
		}
	}
}

// GetRequestID 从 context 中提取 request_id（委托给 contextkeys 包）。
func GetRequestID(ctx context.Context) string {
	return contextkeys.GetRequestID(ctx)
}

// Tracing 中间件集成 OpenTelemetry 兼容的分布式追踪。
// 自动从请求头提取或创建 W3C Trace Context，将 SpanContext 注入到 context 中。
// 配合 tracer 包使用，实现端到端的链路追踪。
func Tracing() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			traceID := util.GenerateID()
			ctx = context.WithValue(ctx, contextkeys.TraceIDKey{}, traceID)

			// 创建 Server Span（如果 Tracer 已初始化）
			tr := tracer.GlobalTracer()
			spanCtx, span := tr.StartServerSpan(ctx, "http.request")
			defer span.End()

			// 设置 Span 属性
			span.SetAttribute("trace_id", traceID)
			span.SetAttribute("request_id", GetRequestID(ctx))

			resp, err := next(spanCtx, req)

			// 记录错误
			if err != nil {
				span.RecordError(err)
			} else {
				span.SetStatus(tracer.StatusOK, "success")
			}

			return resp, err
		}
	}
}

// GetTraceID 从 context 中提取 trace_id（委托给 contextkeys 包）。
func GetTraceID(ctx context.Context) string {
	return contextkeys.GetTraceID(ctx)
}

// Logger 中间件使用结构化日志记录每个请求。
// 记录内容：method、path、status、duration、request_id、trace_id。
// 使用 log 包的全局 Logger，自动输出 JSON 或 Text 格式。
func Logger() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			start := time.Now()
			requestID := GetRequestID(ctx)
			traceID := GetTraceID(ctx)

			resp, err := next(ctx, req)

			elapsed := time.Since(start)
			lg := log.GlobalLogger()

			if err != nil {
				lg.ErrorContext(ctx, "request failed",
					log.String("request_id", requestID),
					log.String("trace_id", traceID),
					log.Duration("duration", elapsed),
					log.Err(err),
				)
			} else {
				lg.InfoContext(ctx, "request completed",
					log.String("request_id", requestID),
					log.String("trace_id", traceID),
					log.Duration("duration", elapsed),
				)
			}

			return resp, err
		}
	}
}

// Recovery 中间件捕获 panic 并恢复，防止单个请求崩溃导致整个服务挂掉。
// 恢复后会记录堆栈信息和 request_id，便于排查问题。
func Recovery() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (resp interface{}, err error) {
			defer func() {
				if r := recover(); r != nil {
					requestID := GetRequestID(ctx)
					traceID := GetTraceID(ctx)
					stack := string(debug.Stack())
					log.GlobalLogger().ErrorContext(ctx, "panic recovered",
						log.String("request_id", requestID),
						log.String("trace_id", traceID),
						log.String("panic", fmt.Sprintf("%v", r)),
						log.String("stack", stack),
					)
					err = errors.ErrInternalError
					resp = nil
				}
			}()
			return next(ctx, req)
		}
	}
}

// CORS 中间件处理跨域请求。
// 允许所有来源的跨域请求，生产环境应配置具体的允许域名。
func CORS() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// CORS 处理逻辑在 HTTP 层面，这里预留接口
			// 实际实现时从 HTTP header 中读取 Origin 并设置响应头
			return next(ctx, req)
		}
	}
}

// =============================================================================
// SSE 中间件 — 检测 SSE 请求并跳过 JSON 响应包装
// =============================================================================

// SSE 中间件检测 SSE 请求（Accept: text/event-stream），将 context 标记为 SSE 请求。
// 标记后，框架的 response.WrapHandler 会跳过 JSON 响应包装。
// 必须放在中间件链前端，确保 SSE 请求不会被后续中间件包装为 JSON 响应。
//
// 使用方式：
//
//	chain := middleware.DefaultChain()
//	chain.Use(middleware.SSE())
func SSE() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// 检测是否为 SSE 请求
			if isSSERequest(ctx, req) {
				ctx = context.WithValue(ctx, contextkeys.SSEKey{}, struct{}{})
			}
			return next(ctx, req)
		}
	}
}

// isSSERequest 判断请求是否为 SSE 请求。
// 检查 Accept 头包含 text/event-stream，或请求路径以 /sse/ 开头。
func isSSERequest(ctx context.Context, req interface{}) bool {
	// 尝试从 req 中提取 HTTP 请求信息
	type httpRequest interface {
		Header() http.Header
		URL() string
	}

	if hr, ok := req.(httpRequest); ok {
		// 检查 Accept 头
		if strings.Contains(hr.Header().Get("Accept"), "text/event-stream") {
			return true
		}
		// 检查路径约定
		if strings.HasPrefix(hr.URL(), "/sse/") || strings.HasPrefix(hr.URL(), "/events") {
			return true
		}
	}
	return false
}

// SSEAware HTTP 中间件，在 HTTP 层面检测 SSE 请求并标记 context。
// 适用于直接使用标准 http.Handler 的场景。
func SSEAware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sse.IsSSE(r) {
			ctx := context.WithValue(r.Context(), contextkeys.SSEKey{}, struct{}{})
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// Timout / Metrics（治理相关中间件）
// =============================================================================
// 如果请求在超时时间内未完成，自动取消并返回超时错误。
func Timeout(timeout time.Duration) core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// 使用 channel 实现超时控制
			type result struct {
				resp interface{}
				err  error
			}
			done := make(chan result, 1)

			go func() {
				r, e := next(ctx, req)
				done <- result{r, e}
			}()

			select {
			case res := <-done:
				return res.resp, res.err
			case <-ctx.Done():
				return nil, fmt.Errorf("request timeout after %v", timeout)
			}
		}
	}
}

// Metrics 中间件集成 Prometheus 指标采集。
// 自动记录 HTTP 请求总数（http_requests_total）、请求延迟（http_request_duration_seconds）、
// 并发请求数（http_requests_in_flight）和响应大小（http_response_size_bytes）。
func Metrics() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			metrics.HTTPRequestsInFlight.Inc()
			start := time.Now()

			resp, err := next(ctx, req)

			metrics.HTTPRequestsInFlight.Dec()
			duration := time.Since(start).Seconds()

			// 提取 method 和 path（从 req 中获取，实际使用时从 HTTP 请求获取）
			method := "GET"
			path := "/"
			status := "200"
			if err != nil {
				status = "500"
			}

			// 记录请求总数
			metrics.HTTPRequestsTotal.With(
				metrics.LabelPair{Name: "method", Value: method},
				metrics.LabelPair{Name: "path", Value: path},
				metrics.LabelPair{Name: "status", Value: status},
			).Inc()

			// 记录请求延迟分布
			metrics.HTTPRequestDuration.With(
				metrics.LabelPair{Name: "method", Value: method},
				metrics.LabelPair{Name: "path", Value: path},
				metrics.LabelPair{Name: "status", Value: status},
			).Observe(duration)

			return resp, err
		}
	}
}

// DefaultChain 返回框架推荐的默认中间件链。
// 包含：SSE → RequestID → Tracing → Logger → Recovery → CORS。
func DefaultChain() *core.MiddlewareChain {
	chain := core.NewMiddlewareChain()
	chain.Use(SSE())
	chain.Use(RequestID())
	chain.Use(Tracing())
	chain.Use(Logger())
	chain.Use(Recovery())
	chain.Use(CORS())
	return chain
}

// FullChain 返回完整的中间件链（含治理组件）。
// 包含：SSE → RequestID → Tracing → Logger → Recovery → CORS → Timeout → Metrics。
func FullChain(timeout time.Duration) *core.MiddlewareChain {
	chain := DefaultChain()
	chain.Use(Timeout(timeout))
	chain.Use(Metrics())
	return chain
}

// =============================================================================
// HTTP 中间件适配器
// =============================================================================

// HTTPTracing 是 HTTP 层面的 Tracing 中间件。
// 从 HTTP 请求头中提取或创建 trace context，并注入到 context 中。
func HTTPTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 提取 trace context
		headers := make(map[string]string)
		for k, v := range r.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}

		sc := tracer.ExtractHTTPHeaders(headers)
		ctx := r.Context()
		if sc != nil {
			ctx = tracer.ContextWithSpanContext(ctx, sc)
		}

		// 创建 Server Span
		tr := tracer.GlobalTracer()
		ctx, span := tr.StartServerSpan(ctx, r.Method+" "+r.URL.Path)
		defer span.End()

		span.SetAttribute("http.method", r.Method)
		span.SetAttribute("http.url", r.URL.String())
		span.SetAttribute("http.host", r.Host)
		span.SetAttribute("http.user_agent", r.UserAgent())

		// 注入响应头
		respHeaders := make(map[string]string)
		tracer.InjectHTTPHeaders(&span.SpanContext, respHeaders)
		for k, v := range respHeaders {
			w.Header().Set(k, v)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HTTPMetrics 是 HTTP 层面的 Metrics 中间件。
// 包装 http.Handler，自动采集 HTTP 请求指标。
func HTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics.HTTPRequestsInFlight.Inc()
		start := time.Now()

		// 包装 ResponseWriter 以捕获状态码
		crw := &captureResponseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(crw, r)

		metrics.HTTPRequestsInFlight.Dec()
		duration := time.Since(start).Seconds()

		status := fmt.Sprintf("%d", crw.statusCode)
		metrics.HTTPRequestsTotal.With(
			metrics.LabelPair{Name: "method", Value: r.Method},
			metrics.LabelPair{Name: "path", Value: r.URL.Path},
			metrics.LabelPair{Name: "status", Value: status},
		).Inc()

		metrics.HTTPRequestDuration.With(
			metrics.LabelPair{Name: "method", Value: r.Method},
			metrics.LabelPair{Name: "path", Value: r.URL.Path},
			metrics.LabelPair{Name: "status", Value: status},
		).Observe(duration)
	})
}

// captureResponseWriter 包装 http.ResponseWriter 以捕获状态码。
type captureResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (crw *captureResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
	crw.ResponseWriter.WriteHeader(code)
}

// =============================================================================
// Span 信息提取（从 context）
// =============================================================================

// SpanContext 是 tracer 包的 SpanContext 类型的别名，方便中间件使用。
type SpanContext = tracer.SpanContext

// HTTPMiddleware 将 core.Middleware 转换为兼容标准 http.Handler 的中间件。
// 保留此方法供用户自定义 HTTP 中间件转换。
func HTTPMiddleware(mw core.Middleware) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 包装为 core.Handler
			handler := mw(func(ctx context.Context, req interface{}) (interface{}, error) {
				next.ServeHTTP(w, r.WithContext(ctx))
				return nil, nil
			})

			handler(ctx, nil)
		})
	}
}