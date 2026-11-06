// Package apperr 定义全项目统一的错误码与领域错误类型。
//
// 错误码分段（见 readme D6）：
//
//	1xxxx 租户
//	2xxxx 认证鉴权
//	3xxxx 配额
//	4xxxx 存储
//	9xxxx 业务
//
// 框架自身只占 1~4 段。通用错误（参数不合法、内部错误等）用 4 位码，
// 避免与 5 位业务段冲突。
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Code 是 5 位分段错误码；4 位码留给框架通用错误。
type Code int

const (
	// CodeOK 表示成功，不进入错误响应体。
	CodeOK Code = 0

	// 通用错误（4 位，框架内部使用）
	CodeInvalidArgument Code = 1000
	CodeNotFound        Code = 1001
	CodeConflict        Code = 1002
	CodeInternal        Code = 1003
	CodeUnavailable     Code = 1004

	// 1xxxx 租户
	CodeTenantNotFound  Code = 10001
	CodeTenantNotReady  Code = 10002
	CodeTenantDisabled  Code = 10003
	CodeTenantExpired   Code = 10004
	CodeTenantProvision Code = 10005

	// 2xxxx 认证鉴权
	CodeUnauthenticated  Code = 20001
	CodeInvalidToken     Code = 20002
	CodeTokenExpired     Code = 20003
	CodeForbidden        Code = 20004
	CodePermissionDenied Code = 20005
	CodeActorMismatch    Code = 20006

	// 3xxxx 配额
	CodeQuotaExceeded Code = 30001
	CodePlanNotFound  Code = 30002

	// 4xxxx 存储
	CodeStorageUnavailable Code = 40001
	CodeFileNotFound       Code = 40002
)

// HTTPStatus 把错误码映射为 HTTP 状态码。
func HTTPStatus(code Code) int {
	switch code {
	case CodeOK:
		return http.StatusOK

	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeInternal:
		return http.StatusInternalServerError

	case CodeTenantNotFound, CodePlanNotFound, CodeFileNotFound:
		return http.StatusNotFound
	case CodeTenantNotReady, CodeTenantDisabled, CodeTenantExpired:
		return http.StatusForbidden
	case CodeTenantProvision:
		return http.StatusConflict

	case CodeUnauthenticated, CodeInvalidToken, CodeTokenExpired:
		return http.StatusUnauthorized
	case CodeForbidden, CodePermissionDenied, CodeActorMismatch:
		return http.StatusForbidden

	case CodeQuotaExceeded:
		return http.StatusTooManyRequests

	case CodeStorageUnavailable:
		return http.StatusServiceUnavailable
	}

	return http.StatusInternalServerError
}

// Error 是全项目统一的领域错误。
// Meta 用于承载结构化上下文（字段名、上限值等），会放进响应体的 data。
type Error struct {
	Code Code
	Msg  string
	Meta map[string]any
	Err  error
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("%d: %s", e.Code, e.Msg)
	}
	return fmt.Sprintf("%d: %s: %v", e.Code, e.Msg, e.Err)
}

// Unwrap 支持 errors.Is / errors.As 继续向内匹配。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WithMeta 返回带上下文的新错误，不修改原错误。
func (e *Error) WithMeta(key string, value any) *Error {
	clone := *e
	clone.Meta = make(map[string]any, len(e.Meta)+1)
	for k, v := range e.Meta {
		clone.Meta[k] = v
	}
	clone.Meta[key] = value
	return &clone
}

// New 创建一个领域错误。
func New(code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

// Newf 创建一个带格式化消息的领域错误。
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap 把底层错误包成领域错误，保留原始错误链。
func Wrap(err error, code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg, Err: err}
}

// From 把任意 error 归一化为 *Error；未知错误按内部错误处理。
func From(err error) *Error {
	if err == nil {
		return nil
	}

	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}

	return &Error{Code: CodeInternal, Msg: "internal error", Err: err}
}

// Is 判断错误是否属于指定错误码。
func Is(err error, code Code) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}
