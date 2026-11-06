// Package response 定义统一响应外壳 {code, msg, data, request_id}。
//
// MCP 端点不使用本外壳，按 JSON-RPC 2.0 返回（见 readme 1.11）。
package response

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
)

// Envelope 是普通 HTTP 接口的统一响应体。
type Envelope struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	Write(c, apperr.CodeOK, "ok", data)
}

// Write 按指定错误码写响应；code 为 0 时为成功。
func Write(c *gin.Context, code apperr.Code, msg string, data any) {
	c.JSON(apperr.HTTPStatus(code), Envelope{
		Code:      int(code),
		Msg:       msg,
		Data:      data,
		RequestID: RequestIDFrom(c.Request.Context()),
	})
}

// Fail 把任意 error 映射为 HTTP 状态码与统一响应体。
// 领域错误的 Meta 放进 data；内部错误不外泄底层原因。
func Fail(c *gin.Context, err error) {
	appErr := apperr.From(err)
	if appErr == nil {
		c.Status(apperr.HTTPStatus(apperr.CodeOK))
		return
	}

	var data any
	if len(appErr.Meta) > 0 {
		data = appErr.Meta
	}

	c.AbortWithStatusJSON(apperr.HTTPStatus(appErr.Code), Envelope{
		Code:      int(appErr.Code),
		Msg:       appErr.Msg,
		Data:      data,
		RequestID: RequestIDFrom(c.Request.Context()),
	})
}
