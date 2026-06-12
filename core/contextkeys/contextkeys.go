// Package contextkeys 提供跨包共享的 context key 定义。
// 避免因 context key 类型定义在 package A 而 package B 需要导入 A 导致的循环依赖。
// log、middleware、tracer 等包都通过此包获取统一的 context key。
package contextkeys

import "context"

// RequestIDKey 是 context 中存储 request_id 的 key 类型。
type RequestIDKey struct{}

// TraceIDKey 是 context 中存储 trace_id 的 key 类型。
type TraceIDKey struct{}

// SpanContextKey 是 context 中存储 SpanContext 的 key 类型。
type SpanContextKey struct{}

// SSEKey 是 context 中标记 SSE 请求的 key 类型。
// 当 context 包含此 key 时，框架跳过 JSON 响应包装，交由 SSE 处理器直接写入 HTTP 响应。
type SSEKey struct{}

// =============================================================================
// context 值提取辅助函数
// =============================================================================

// GetRequestID 从 context 中提取 request_id。
// 返回空字符串表示 context 中没有 request_id。
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// GetTraceID 从 context 中提取 trace_id。
// 返回空字符串表示 context 中没有 trace_id。
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(TraceIDKey{}).(string); ok {
		return id
	}
	return ""
}

// IsSSE 检查 context 是否为 SSE 请求。
func IsSSE(ctx context.Context) bool {
	_, ok := ctx.Value(SSEKey{}).(struct{})
	return ok
}