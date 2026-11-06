package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/app"
	"github.com/cuiwenguang/go-saas-lite/internal/security/auth"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
)

type stubChecker struct {
	name string
	err  error
}

func (s stubChecker) Name() string { return s.name }

func (s stubChecker) Check(context.Context) error { return s.err }

func newTestRouter(t *testing.T, checkers ...obs.Checker) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := app.DefaultConfig()
	logger := obs.NewLogger("error", "text")
	metrics := obs.NewMetrics()
	health := obs.NewHealth()
	health.Register(checkers...)
	issuer := auth.NewIssuer(cfg.Auth.Secret, cfg.Auth.IssuerName)

	engine, _ := app.NewRouter(cfg, logger, metrics, health, issuer, nil, nil, nil)

	return engine
}

func TestHealthIsAlwaysOK(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	newTestRouter(t).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", recorder.Code)
	}
}

func TestReadyzWithoutCheckerIsOK(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	newTestRouter(t).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", recorder.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("want status ok, got %q", body.Status)
	}
}

func TestReadyzFailsWhenCheckerFails(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	newTestRouter(t, stubChecker{name: "db", err: errors.New("connection refused")}).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", recorder.Code)
	}
}

func TestRequestIDHeaderIsEchoed(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", "trace-from-gateway")

	newTestRouter(t).ServeHTTP(recorder, req)

	if got := recorder.Header().Get("X-Request-Id"); got != "trace-from-gateway" {
		t.Fatalf("want upstream request id echoed, got %q", got)
	}
}

func TestRequestIDIsGeneratedWhenMissing(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	newTestRouter(t).ServeHTTP(recorder, req)

	if got := recorder.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("want generated X-Request-Id, got empty")
	}
}

func TestMetricsEndpointServesPrometheusFormat(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)

	// 先打一次业务请求，确认指标已采集。
	warm := httptest.NewRecorder()
	router.ServeHTTP(warm, httptest.NewRequest(http.MethodGet, "/health", nil))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "saas_http_requests_total") {
		t.Fatalf("metrics body missing request counter:\n%s", body)
	}
}

func TestOptionsPreflightSkipsHandler(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)

	newTestRouter(t).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", recorder.Code)
	}
}
