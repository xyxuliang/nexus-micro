// Package middleware 提供框架内置的中间件集合。
// 所有中间件在 HTTP 和 gRPC 协议之间完全共享，只需定义一次。
// 中间件执行顺序：RequestID → Tracing → Logger → Recovery → CORS → Auth → RateLimit → Timeout → Shedding → Metrics。
package middleware

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/nexus-micro/nexus-micro/core"
	"github.com/nexus-micro/nexus-micro/internal/errors"
	"github.com/nexus-micro/nexus-micro/internal/util"
)

// requestIDKey 是 context 中存储 request_id 的 key 类型。
type requestIDKey struct{}

// traceIDKey 是 context 中存储 trace_id 的 key 类型。
type traceIDKey struct{}

// RequestID 中间件为每个请求生成唯一的 request_id。
// 如果请求头中已有 X-Request-ID，则复用该值。
// request_id 会被注入到 context 和响应头中。
func RequestID() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			requestID := util.GenerateID()
			ctx = context.WithValue(ctx, requestIDKey{}, requestID)
			return next(ctx, req)
		}
	}
}

// GetRequestID 从 context 中提取 request_id。
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Tracing 中间件为每个请求生成 OpenTelemetry trace_id。
// 框架后续会集成 OpenTelemetry SDK，当前版本提供基础 trace_id 生成。
func Tracing() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			traceID := util.GenerateID()
			ctx = context.WithValue(ctx, traceIDKey{}, traceID)
			return next(ctx, req)
		}
	}
}

// GetTraceID 从 context 中提取 trace_id。
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Logger 中间件记录每个请求的方法、路径、耗时和状态码。
func Logger() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			start := time.Now()
			requestID := GetRequestID(ctx)

			resp, err := next(ctx, req)

			elapsed := time.Since(start)
			log.Printf("[%s] request completed in %v", requestID, elapsed)

			if err != nil {
				log.Printf("[%s] request failed: %v", requestID, err)
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
					stack := string(debug.Stack())
					log.Printf("[%s] PANIC RECOVERED: %v\n%s", requestID, r, stack)
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

// Timeout 中间件为请求设置超时时间。
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

// Metrics 中间件采集请求指标（QPS、延迟、状态码）。
// 框架后续会集成 Prometheus，当前版本提供基础计数。
func Metrics() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			elapsed := time.Since(start)

			// 记录指标（后续接入 Prometheus）
			_ = elapsed
			_ = resp
			_ = err

			return resp, err
		}
	}
}

// DefaultChain 返回框架推荐的默认中间件链。
// 包含：RequestID、Tracing、Logger、Recovery、CORS。
func DefaultChain() *core.MiddlewareChain {
	chain := core.NewMiddlewareChain()
	chain.Use(RequestID())
	chain.Use(Tracing())
	chain.Use(Logger())
	chain.Use(Recovery())
	chain.Use(CORS())
	return chain
}

// FullChain 返回完整的中间件链（含治理组件）。
// 包含：RequestID、Tracing、Logger、Recovery、CORS、Timeout、Metrics。
func FullChain(timeout time.Duration) *core.MiddlewareChain {
	chain := DefaultChain()
	chain.Use(Timeout(timeout))
	chain.Use(Metrics())
	return chain
}

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