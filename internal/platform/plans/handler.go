package plans

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露平台侧套餐接口。
type Handler struct {
	service *Service
}

// NewHandler 创建 handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register 挂到平台路由链上；每个接口显式声明所需权限点。
func (h *Handler) Register(group *gin.RouterGroup) {
	plans := group.Group("/plans")
	plans.GET("", authz.PlatformRequire(authz.PlanRead), h.list)
	plans.POST("", authz.PlatformRequire(authz.PlanUpdate), h.create)
	plans.PATCH("/:id", authz.PlatformRequire(authz.PlanUpdate), h.update)
}

func (h *Handler) list(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, items)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	plan, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, plan)
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	plan, err := h.service.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, plan)
}
