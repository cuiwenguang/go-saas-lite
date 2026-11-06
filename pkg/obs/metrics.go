package obs

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 持有独立的 Prometheus 注册表，不使用全局默认注册表。
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	panics   prometheus.Counter
}

// NewMetrics 创建指标集。
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "saas",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "HTTP 请求总数。",
	}, []string{"method", "route", "status"})

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "saas",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP 请求耗时分布。",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status"})

	panics := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "saas",
		Subsystem: "http",
		Name:      "panics_total",
		Help:      "HTTP 处理中的 panic 次数。",
	})

	registry.MustRegister(requests, duration, panics)

	return &Metrics{
		registry: registry,
		requests: requests,
		duration: duration,
		panics:   panics,
	}
}

// Registry 返回本实例的注册表，后续业务指标注册到同一处。
func (m *Metrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.registry
}

// Handler 返回 /metrics 抓取入口。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Middleware 记录请求量、耗时与状态码；路由标签取路由模板，避免高基数。
func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		elapsed := time.Since(start).Seconds()

		m.requests.WithLabelValues(c.Request.Method, route, status).Inc()
		m.duration.WithLabelValues(c.Request.Method, route, status).Observe(elapsed)
	}
}

// IncPanic 记录一次 panic。
func (m *Metrics) IncPanic() {
	if m == nil {
		return
	}
	m.panics.Inc()
}
