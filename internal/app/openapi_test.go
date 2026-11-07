package app_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"gopkg.in/yaml.v3"

	apidocs "github.com/cuiwenguang/go-saas-lite/docs/api"
	"github.com/cuiwenguang/go-saas-lite/internal/app"
)

type openAPIDoc struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

// TestOpenAPICoversRoutes 防止新增路由后忘记补文档。
func TestOpenAPICoversRoutes(t *testing.T) {
	if os.Getenv("TEST_DATABASE_DSN") == "" {
		t.Skip("TEST_DATABASE_DSN is required for full route graph")
	}

	var doc openAPIDoc
	if err := yaml.Unmarshal(apidocs.OpenAPI, &doc); err != nil {
		t.Fatalf("parse openapi: %v", err)
	}

	cfg := testConfig(t)
	cfg.Redis.Addr = os.Getenv("TEST_REDIS_ADDR")
	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = "127.0.0.1:6379"
	}
	var engine *gin.Engine
	instance := fx.New(app.New(cfg), fx.NopLogger, fx.Populate(&engine))
	if err := instance.Err(); err != nil {
		t.Fatalf("build engine: %v", err)
	}
	if engine == nil {
		t.Fatal("engine was not constructed")
	}

	documented := make(map[string]struct{})
	methods := []string{"get", "post", "put", "patch", "delete"}
	for path, operations := range doc.Paths {
		for _, method := range methods {
			if _, ok := operations[method]; ok {
				documented[strings.ToUpper(method)+" "+path] = struct{}{}
			}
		}
	}

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+normalizeGinPath(route.Path)] = struct{}{}
	}

	for key := range registered {
		if _, ok := documented[key]; !ok {
			t.Errorf("route %s is registered but missing from OpenAPI", key)
		}
	}
	for key := range documented {
		if _, ok := registered[key]; !ok {
			t.Errorf("OpenAPI documents %s but no route is registered", key)
		}
	}
}

func normalizeGinPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = fmt.Sprintf("{%s}", part[1:])
		}
	}

	return strings.Join(parts, "/")
}
