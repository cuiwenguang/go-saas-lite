package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsEndpoints(t *testing.T) {
	engine := newTestRouter(t)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "openapi: 3.1.0") {
		t.Fatalf("openapi.yaml: %d", rec.Code)
	}

	ui := httptest.NewRecorder()
	engine.ServeHTTP(ui, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if ui.Code != http.StatusOK || !strings.Contains(ui.Body.String(), "swagger-ui") {
		t.Fatalf("docs: %d", ui.Code)
	}
}
