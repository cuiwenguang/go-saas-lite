package obs

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Recover 捕获 handler panic：记录日志与指标，返回统一错误响应，不拖垮进程。
func Recover(log *slog.Logger, metrics *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			metrics.IncPanic()
			log.ErrorContext(c.Request.Context(), "http handler panic",
				"panic", recovered,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"stack", string(debug.Stack()),
			)

			response.Fail(c, apperr.New(apperr.CodeInternal, "internal error"))
		}()

		c.Next()
	}
}
