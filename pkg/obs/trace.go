package obs

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type traceIDKey struct{}

// WithTraceID 把 trace_id 写入 ctx。
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceIDFrom 读取 ctx 中的 trace_id，缺失返回空串。
func TraceIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

// NewTracerProvider 创建并注册全局 TracerProvider。
// exporter 支持 none（不导出）与 stdout（本地调试）。
func NewTracerProvider(serviceName, env, exporter string) (*sdktrace.TracerProvider, error) {
	res := resource.NewSchemaless(
		attribute.String("service.name", serviceName),
		attribute.String("deployment.environment", env),
	)

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}

	switch strings.ToLower(strings.TrimSpace(exporter)) {
	case "", "none":
		// 只生成本地 Span，不导出。
	case "stdout":
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout trace exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	default:
		return nil, fmt.Errorf("unsupported trace exporter %q", exporter)
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp, nil
}

// TraceContext 把当前 Span 的 trace_id 放进 ctx，供日志自动补全。
// 必须放在 otelgin 之后。
func TraceContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		spanContext := trace.SpanContextFromContext(c.Request.Context())
		if spanContext.IsValid() {
			ctx := WithTraceID(c.Request.Context(), spanContext.TraceID().String())
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
