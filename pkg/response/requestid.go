package response

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderRequestID 是请求链路 ID 的传输头。
const HeaderRequestID = "X-Request-Id"

type requestIDKey struct{}

// WithRequestID 把 request_id 写入 ctx。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom 读取 ctx 中的 request_id，缺失返回空串。
func RequestIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// RequestID 是链路 ID 中间件：优先复用上游传入的头，缺失则生成 UUIDv7。
// 放在 recover 之后、trace 之前，保证 panic 响应也带 request_id。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			v7, err := uuid.NewV7()
			if err != nil {
				id = uuid.NewString()
			} else {
				id = v7.String()
			}
		}

		c.Header(HeaderRequestID, id)
		c.Request = c.Request.WithContext(WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}
