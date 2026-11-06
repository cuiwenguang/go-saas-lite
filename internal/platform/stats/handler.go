package stats

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露平台总览统计接口。
type Handler struct {
	service *Service
}

// NewHandler 创建 handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register 挂到平台路由链上。
func (h *Handler) Register(group *gin.RouterGroup) {
	stats := group.Group("/stats")
	stats.GET("/overview", authz.PlatformRequire(authz.StatsRead), h.overview)
	stats.GET("/usage-top", authz.PlatformRequire(authz.StatsRead), h.usageTop)
}

func (h *Handler) overview(c *gin.Context) {
	out, err := h.service.Overview(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) usageTop(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))

	items, err := h.service.UsageTop(c.Request.Context(), c.Query("metric"), limit)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, items)
}
