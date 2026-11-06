package conf

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露租户侧配置接口。
type Handler struct {
	service    *Service
	authorizer authz.Authorizer
}

// NewHandler 创建 handler。
func NewHandler(service *Service, authorizer authz.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// Register 挂到租户路由链上。
func (h *Handler) Register(group *gin.RouterGroup) {
	configs := group.Group("/configs")
	configs.GET("", authz.Require(h.authorizer, PermRead), h.list)
	configs.GET("/:namespace/:key", authz.Require(h.authorizer, PermRead), h.get)
	configs.PUT("/:namespace/:key", authz.Require(h.authorizer, PermUpdate), h.set)
	configs.DELETE("/:namespace/:key", authz.Require(h.authorizer, PermUpdate), h.remove)
}

func (h *Handler) list(c *gin.Context) {
	namespace := c.Query("namespace")
	if namespace == "" {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "namespace is required"))
		return
	}

	entries, err := h.service.List(c.Request.Context(), namespace)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": entries})
}

func (h *Handler) get(c *gin.Context) {
	value, err := h.service.Get(c.Request.Context(), c.Param("namespace"), c.Param("key"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"namespace": c.Param("namespace"), "key": c.Param("key"), "value": value})
}

func (h *Handler) set(c *gin.Context) {
	var req SetConfigInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	if err := h.service.Set(c.Request.Context(), c.Param("namespace"), c.Param("key"), req.Value); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"updated": true})
}

func (h *Handler) remove(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("namespace"), c.Param("key")); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"deleted": true})
}
