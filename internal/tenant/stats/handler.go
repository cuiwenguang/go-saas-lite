package stats

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露租户总览接口。
type Handler struct {
	service    *Service
	authorizer authz.Authorizer
}

// NewHandler 创建 handler。
func NewHandler(service *Service, authorizer authz.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// Register 挂到租户链上。
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/stats/overview", authz.Require(h.authorizer, authz.PermStatsRead), h.overview)
}

func (h *Handler) overview(c *gin.Context) {
	out, err := h.service.Overview(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}
