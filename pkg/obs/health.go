package obs

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Checker 是 /readyz 的依赖探活项；DB、Redis 在后续里程碑注册进来。
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type checkResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type readyResponse struct {
	Status string                 `json:"status"`
	Checks map[string]checkResult `json:"checks"`
}

// Health 维护就绪探活项集合。
type Health struct {
	mu       sync.RWMutex
	checkers []Checker
}

// NewHealth 创建空的健康检查器。
func NewHealth() *Health {
	return &Health{}
}

// Register 追加探活项，通常在装配阶段调用。
func (h *Health) Register(checkers ...Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers = append(h.checkers, checkers...)
}

// Live 是 liveness：进程存活即 200，不检查外部依赖。
func (h *Health) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready 是 readiness：任一依赖不可用返回 503。
func (h *Health) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	h.mu.RLock()
	checkers := make([]Checker, len(h.checkers))
	copy(checkers, h.checkers)
	h.mu.RUnlock()

	results := make(map[string]checkResult, len(checkers))
	failed := false

	for _, checker := range checkers {
		if err := checker.Check(ctx); err != nil {
			results[checker.Name()] = checkResult{Status: "down", Error: err.Error()}
			failed = true
			continue
		}
		results[checker.Name()] = checkResult{Status: "up"}
	}

	body := readyResponse{Status: "ok", Checks: results}
	if failed {
		body.Status = "unavailable"
		c.JSON(http.StatusServiceUnavailable, body)
		return
	}

	c.JSON(http.StatusOK, body)
}
