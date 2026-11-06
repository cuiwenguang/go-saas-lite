package response

import (
	"context"

	"github.com/gin-gonic/gin"
)

type clientIPKey struct{}

// WithClientIP 把客户端 IP 写入 ctx，供审计等场景读取。
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// ClientIPFrom 读取 ctx 中的客户端 IP，缺失返回空串。
func ClientIPFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	ip, _ := ctx.Value(clientIPKey{}).(string)

	return ip
}

// ClientIP 是客户端 IP 中间件：审计日志记录操作来源用。
func ClientIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(WithClientIP(c.Request.Context(), c.ClientIP()))
		c.Next()
	}
}
