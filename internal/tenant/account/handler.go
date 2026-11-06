package account

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Handler 暴露登录与 API Key 接口。
type Handler struct {
	service    *Service
	authorizer authz.Authorizer
}

// NewHandler 创建 handler。
func NewHandler(service *Service, authorizer authz.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterPublic 挂无需令牌的登录接口。
func (h *Handler) RegisterPublic(group *gin.RouterGroup) {
	group.POST("/auth/login", h.login)
}

// Register 挂需要租户令牌的 API Key 接口。
func (h *Handler) Register(group *gin.RouterGroup) {
	keys := group.Group("/api-keys")
	keys.GET("", authz.Require(h.authorizer, authz.PermAPIKeyRead), h.listKeys)
	keys.POST("", authz.Require(h.authorizer, authz.PermAPIKeyCreate), h.createKey)
	keys.DELETE("/:id", authz.Require(h.authorizer, authz.PermAPIKeyRevoke), h.revokeKey)
}

func (h *Handler) login(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-Id")
	if tenantID == "" {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "X-Tenant-Id header is required"))
		return
	}

	var req LoginInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	result, err := h.service.Login(c.Request.Context(), tenantID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, result)
}

func (h *Handler) createKey(c *gin.Context) {
	var req CreateKeyInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	scope, err := tenancy.ScopeFrom(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	plain, key, err := h.service.CreateKey(c.Request.Context(), scope.TenantID, scope.UserID, req.Name)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"key": key, "plain": plain})
}

func (h *Handler) listKeys(c *gin.Context) {
	scope, err := tenancy.ScopeFrom(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	keys, err := h.service.ListKeys(c.Request.Context(), scope.TenantID, scope.UserID)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": keys})
}

func (h *Handler) revokeKey(c *gin.Context) {
	scope, err := tenancy.ScopeFrom(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.service.RevokeKey(c.Request.Context(), scope.TenantID, scope.UserID, c.Param("id")); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"revoked": true})
}
