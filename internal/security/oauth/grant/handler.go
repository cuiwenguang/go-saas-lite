package grant

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露 OAuth 授权接口。
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
	grants := group.Group("/oauth/grants")
	grants.GET("", authz.Require(h.authorizer, authz.PermOAuthGrantRead), h.list)
	grants.DELETE("/:client_id", authz.Require(h.authorizer, authz.PermOAuthGrantRevoke), h.revoke)
}

func (h *Handler) list(c *gin.Context) {
	includeRevoked := c.Query("include_revoked") == "1"

	items, err := h.service.List(c.Request.Context(), includeRevoked)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": items})
}

func (h *Handler) revoke(c *gin.Context) {
	if err := h.service.Revoke(c.Request.Context(), c.Param("client_id")); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"revoked": true})
}
