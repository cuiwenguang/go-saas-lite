// Package obs 提供日志、链路追踪、指标与健康检查等横切能力。
//
// 约定：
//   - 业务代码统一用 slog.InfoContext(ctx, ...)，字段由本包 Handler 自动补全；
//   - 租户 / 用户字段由上层通过 WithLogAttrs 注入，obs 不依赖业务包。
package obs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

type logAttrsKey struct{}

// WithLogAttrs 把结构化字段挂到 ctx，日志 Handler 自动补全。
// tenantctx 后续用它注入 tenant_id / user_id，避免 obs 反向依赖业务包。
func WithLogAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}

	prev, _ := ctx.Value(logAttrsKey{}).([]slog.Attr)
	next := make([]slog.Attr, 0, len(prev)+len(attrs))
	next = append(next, prev...)
	next = append(next, attrs...)

	return context.WithValue(ctx, logAttrsKey{}, next)
}

// LogAttrsFrom 读取 ctx 中挂载的结构化字段。
func LogAttrsFrom(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	attrs, _ := ctx.Value(logAttrsKey{}).([]slog.Attr)
	return attrs
}

// NewLogger 按级别与格式创建带上下文补全的日志器。
// format 支持 text / json，未知值按 text 处理。
func NewLogger(level, format string) *slog.Logger {
	return NewLoggerWithWriter(os.Stdout, level, format)
}

// NewLoggerWithWriter 便于测试注入输出目标。
func NewLoggerWithWriter(w io.Writer, level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var base slog.Handler
	if strings.EqualFold(format, "json") {
		base = slog.NewJSONHandler(w, opts)
	} else {
		base = slog.NewTextHandler(w, opts)
	}

	return slog.New(&ctxHandler{inner: base})
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ctxHandler 在每条日志落盘前补全 request_id、trace_id 与 ctx 挂载字段。
type ctxHandler struct {
	inner slog.Handler
}

func (h *ctxHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *ctxHandler) Handle(ctx context.Context, record slog.Record) error {
	if id := response.RequestIDFrom(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	if id := TraceIDFrom(ctx); id != "" {
		record.AddAttrs(slog.String("trace_id", id))
	}
	for _, attr := range LogAttrsFrom(ctx) {
		record.AddAttrs(attr)
	}

	return h.inner.Handle(ctx, record)
}

func (h *ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ctxHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ctxHandler) WithGroup(name string) slog.Handler {
	return &ctxHandler{inner: h.inner.WithGroup(name)}
}
