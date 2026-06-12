// Package errors 提供框架统一的错误类型和错误码体系。
// 通过 NewCode() 创建带错误码的错误，框架自动将其转换为统一响应格式。
// 错误码分段：0=成功, 1000-1999=通用, 2000-2999=认证, 3000-3999=用户,
// 4000-4999=订单, 5000-5999=支付, 6000-6999=存储, 7000-7999=通知, 8000-8999=系统。
package errors

import "fmt"

// CodeError 是带错误码的错误类型。
// 框架的中间件和响应层会自动识别此类型并提取错误码。
type CodeError struct {
	Code    int    // 业务错误码
	Message string // 错误描述
	Err     error  // 原始错误（可选）
}

// Error 实现 error 接口。
func (e *CodeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 实现 errors.Unwrap 接口，支持 errors.Is 和 errors.As。
func (e *CodeError) Unwrap() error {
	return e.Err
}

// NewCode 创建带错误码的错误。
func NewCode(code int, msg string) *CodeError {
	return &CodeError{Code: code, Message: msg}
}

// WrapCode 包装已有错误为带错误码的错误。
func WrapCode(code int, msg string, err error) *CodeError {
	return &CodeError{Code: code, Message: msg, Err: err}
}

// 预定义的通用错误码
const (
	// CodeSuccess 成功
	CodeSuccess = 0

	// CodeInvalidParam 参数校验失败
	CodeInvalidParam = 1001

	// CodeNotFound 资源不存在
	CodeNotFound = 1003

	// CodeOperationNotAllowed 操作不允许
	CodeOperationNotAllowed = 1005

	// CodeUnauthorized 未登录
	CodeUnauthorized = 2001

	// CodeTokenExpired Token 过期
	CodeTokenExpired = 2002

	// CodeForbidden 权限不足
	CodeForbidden = 2003

	// CodeInternalError 内部错误
	CodeInternalError = 8000

	// CodeServiceUnavailable 服务不可用
	CodeServiceUnavailable = 8001

	// CodeCircuitBreakerOpen 熔断拒绝
	CodeCircuitBreakerOpen = 8002

	// CodeRateLimited 限流拒绝
	CodeRateLimited = 8003
)

// 预定义的错误实例
var (
	ErrNotFound            = NewCode(CodeNotFound, "资源不存在")
	ErrInvalidParam        = NewCode(CodeInvalidParam, "参数校验失败")
	ErrUnauthorized        = NewCode(CodeUnauthorized, "未登录")
	ErrForbidden           = NewCode(CodeForbidden, "权限不足")
	ErrInternalError       = NewCode(CodeInternalError, "内部服务错误")
	ErrServiceUnavailable  = NewCode(CodeServiceUnavailable, "服务不可用")
	ErrCircuitBreakerOpen  = NewCode(CodeCircuitBreakerOpen, "服务熔断中")
	ErrRateLimited         = NewCode(CodeRateLimited, "请求过于频繁")
)