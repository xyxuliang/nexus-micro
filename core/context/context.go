// Package context 提供统一的请求上下文接口。
// 封装 request_id, trace_id, user_id, tenant_id, language 等常用信息。
// 框架自动将这些信息从 HTTP/gRPC 元数据注入到 context 中，
// 业务代码可以方便地通过 helpers 提取这些信息。
package context

import (
	"context"

	"github.com/nexus-micro/nexus-micro/internal/util"
)

// Context 是统一请求上下文接口。
// 每个请求都会获得一个实现了此接口的 context。
type Context interface {
	context.Context
	// RequestID 返回请求追踪 ID。
	RequestID() string
	// TraceID 返回链路追踪 ID。
	TraceID() string
	// UserID 返回当前用户 ID（未登录返回空）。
	UserID() string
	// TenantID 返回租户 ID（多租户场景）。
	TenantID() string
	// Language 返回语言偏好（国际化场景）。
	Language() string
}

type key int

const (
	requestIDKey key = iota
	traceIDKey
	userIDKey
	tenantIDKey
	languageKey
)

// WithRequestID 将 request_id 注入 context。
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = util.GenerateID()
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// WithTraceID 将 trace_id 注入 context。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		traceID = util.GenerateID()
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

// WithUserID 将 user_id 注入 context。
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// WithTenantID 将 tenant_id 注入 context。
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// WithLanguage 将 language 注入 context。
func WithLanguage(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, languageKey, lang)
}

// RequestID 从 context 提取 request_id。
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// TraceID 从 context 提取 trace_id。
func TraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}
	return ""
}

// UserID 从 context 提取 user_id。
func UserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// TenantID 从 context 提取 tenant_id。
func TenantID(ctx context.Context) string {
	if id, ok := ctx.Value(tenantIDKey).(string); ok {
		return id
	}
	return ""
}

// Language 从 context 提取 language。
func Language(ctx context.Context) string {
	if lang, ok := ctx.Value(languageKey).(string); ok {
		return lang
	}
	return "zh-CN"
}

// New 创建一个新的空 Context（生成 request_id 和 trace_id）。
func New() context.Context {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "")
	ctx = WithTraceID(ctx, "")
	return ctx
}

// From HTTP 提取所有标准头并注入 context。
// 供 HTTP 服务器在进入中间件管道前调用。
func FromHTTP(header httpHeader) context.Context {
	ctx := context.Background()
	if reqID := header.Get("X-Request-ID"); reqID != "" {
		ctx = WithRequestID(ctx, reqID)
	} else {
		ctx = WithRequestID(ctx, util.GenerateID())
	}
	if traceID := header.Get("X-Trace-ID"); traceID != "" {
		ctx = WithTraceID(ctx, traceID)
	}
	if userID := header.Get("X-User-ID"); userID != "" {
		ctx = WithUserID(ctx, userID)
	}
	if tenantID := header.Get("X-Tenant-ID"); tenantID != "" {
		ctx = WithTenantID(ctx, tenantID)
	}
	if lang := header.Get("Accept-Language"); lang != "" {
		// 取第一个语言
		if len(lang) > 0 && lang[0] != ',' {
			idx := 0
			for i, c := range lang {
				if c == ',' || c == ';' {
					break
				}
				idx = i
			}
			ctx = WithLanguage(ctx, lang[:idx+1])
		} else {
			ctx = WithLanguage(ctx, lang)
		}
	}
	return ctx
}

// httpHeader 是 http.Header 的简化接口。
type httpHeader interface {
	Get(key string) string
}