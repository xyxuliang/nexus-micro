// Package response 提供统一的 HTTP 响应格式封装。
// 所有 HTTP 接口强制使用三段式响应结构：{code, msg, data, request_id}。
// 框架自动将 Handler 的返回值封装为统一格式，开发者无需手动构建响应体。
package response

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/xyxuliang/nexus-micro/internal/errors"
)

// Response 是统一响应结构体。
// 所有 HTTP 接口的响应都会封装为这个格式。
type Response struct {
	Code      int         `json:"code"`       // 业务状态码，0 表示成功
	Msg       string      `json:"msg"`        // 提示信息
	Data      interface{} `json:"data"`       // 响应数据，错误时为 nil
	RequestID string      `json:"request_id"` // 请求追踪 ID
}

// PageData 是分页响应的数据结构。
type PageData struct {
	Items    interface{} `json:"items"`     // 数据列表
	Total    int64       `json:"total"`     // 总数
	Page     int         `json:"page"`      // 当前页码
	PageSize int         `json:"page_size"` // 每页大小
}

// Success 创建成功响应（code=0）。
func Success(data interface{}) *Response {
	return &Response{
		Code: 0,
		Msg:  "success",
		Data: data,
	}
}

// Page 创建分页响应。
func Page(items interface{}, total int64, page, pageSize int) *Response {
	return &Response{
		Code: 0,
		Msg:  "success",
		Data: &PageData{
			Items:    items,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	}
}

// Error 创建错误响应。
func Error(code int, msg string) *Response {
	return &Response{
		Code: code,
		Msg:  msg,
	}
}

// FromError 从 Go error 创建响应。
// 如果 error 是 *errors.CodeError 类型，提取其 code 和 msg；
// 否则使用默认错误码 1000（通用错误）。
func FromError(err error) *Response {
	if ce, ok := err.(*errors.CodeError); ok {
		return &Response{
			Code: ce.Code,
			Msg:  ce.Message,
		}
	}
	return &Response{
		Code: 1000,
		Msg:  err.Error(),
	}
}

// WithRequestID 设置请求追踪 ID。
func (r *Response) WithRequestID(requestID string) *Response {
	r.RequestID = requestID
	return r
}

// WriteTo 将响应写入 http.ResponseWriter。
func (r *Response) WriteTo(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// 根据 code 决定 HTTP 状态码
	statusCode := http.StatusOK
	if r.Code != 0 {
		statusCode = codeToHTTPStatus(r.Code)
	}
	w.WriteHeader(statusCode)

	json.NewEncoder(w).Encode(r)
}

// codeToHTTPStatus 将业务错误码映射为 HTTP 状态码。
func codeToHTTPStatus(code int) int {
	switch {
	case code == 0:
		return http.StatusOK
	case code >= 1000 && code < 2000:
		// 通用错误：参数校验 400，资源不存在 404
		if code == 1001 {
			return http.StatusBadRequest
		}
		if code == 1003 {
			return http.StatusNotFound
		}
		return http.StatusBadRequest
	case code >= 2000 && code < 3000:
		// 认证授权
		if code == 2001 {
			return http.StatusUnauthorized
		}
		if code == 2003 {
			return http.StatusForbidden
		}
		return http.StatusUnauthorized
	case code >= 8000 && code < 9000:
		// 系统错误
		if code == 8003 {
			return http.StatusTooManyRequests
		}
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// WrapHandler 将业务 Handler 包装为统一响应格式的 http.Handler。
// 业务 Handler 返回 (data, error)，框架自动封装为统一响应。
func WrapHandler(handler func(ctx context.Context, req interface{}) (interface{}, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 提取 request_id
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = r.Header.Get("X-Request-Id")
		}

		var resp *Response

		// defer 必须在 handler 调用之前注册，否则 panic 无法被恢复
		defer func() {
			if rec := recover(); rec != nil {
				resp = &Response{
					Code:      8000,
					Msg:       "内部服务错误",
					RequestID: requestID,
				}
				resp.WriteTo(w)
			}
		}()

		// 调用业务 Handler
		data, err := handler(ctx, nil)

		if err != nil {
			resp = FromError(err)
		} else {
			resp = Success(data)
		}

		resp.WithRequestID(requestID)
		resp.WriteTo(w)
	}
}
