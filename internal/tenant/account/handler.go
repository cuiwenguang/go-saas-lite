package account

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const (
	refreshCookieName = "saas_tenant_refresh"
	refreshCookiePath = "/api/tenant/v1/auth"
)

// Options 是租户会话 HTTP 侧选项。
type Options struct {
	// SecureCookies 决定 refresh cookie 是否带 Secure（生产 HTTPS）。
	SecureCookies bool
}

// Handler 暴露会话、资料与 API Key 接口。
type Handler struct {
	service    *Service
	authorizer authz.Authorizer
	opts       Options
}

// NewHandler 创建 handler。
func NewHandler(service *Service, authorizer authz.Authorizer, opts Options) *Handler {
	return &Handler{service: service, authorizer: authorizer, opts: opts}
}

// RegisterPublic 挂无需令牌的会话接口（登录 / 刷新 / 登出）。
func (h *Handler) RegisterPublic(group *gin.RouterGroup) {
	session := group.Group("/auth")
	session.POST("/login", h.login)
	session.POST("/refresh", h.refresh)
	session.POST("/logout", h.logout)
	session.POST("/otp/send", h.sendCode)
	session.POST("/password/reset", h.resetPassword)
}

// Register 挂需要租户令牌的接口。
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/me", h.me)
	group.POST("/auth/password", h.changePassword)

	keys := group.Group("/api-keys")
	keys.GET("", authz.Require(h.authorizer, authz.PermAPIKeyRead), h.listKeys)
	keys.POST("", authz.Require(h.authorizer, authz.PermAPIKeyCreate), h.createKey)
	keys.DELETE("/:id", authz.Require(h.authorizer, authz.PermAPIKeyRevoke), h.revokeKey)
}

func (h *Handler) login(c *gin.Context) {
	var req LoginInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	result, err := h.service.Login(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	h.setRefreshCookie(c, result.RefreshToken)
	response.OK(c, result)
}

func (h *Handler) sendCode(c *gin.Context) {
	var req SendCodeInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	if err := h.service.SendCode(c.Request.Context(), req); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"sent": true})
}

func (h *Handler) resetPassword(c *gin.Context) {
	var req ResetPasswordInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	if err := h.service.ResetPassword(c.Request.Context(), req); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"ok": true})
}

func (h *Handler) refresh(c *gin.Context) {
	raw, err := c.Cookie(refreshCookieName)
	if err != nil || raw == "" {
		response.Fail(c, apperr.New(apperr.CodeUnauthenticated, "missing refresh token"))
		return
	}

	result, err := h.service.Refresh(c.Request.Context(), raw)
	if err != nil {
		h.clearRefreshCookie(c)
		response.Fail(c, err)
		return
	}

	h.setRefreshCookie(c, result.RefreshToken)
	response.OK(c, result)
}

// logout 必须幂等：令牌已失效也返回成功。
func (h *Handler) logout(c *gin.Context) {
	raw, _ := c.Cookie(refreshCookieName)
	if err := h.service.Logout(c.Request.Context(), raw); err != nil {
		h.clearRefreshCookie(c)
		response.Fail(c, err)
		return
	}

	h.clearRefreshCookie(c)
	response.OK(c, gin.H{"ok": true})
}

func (h *Handler) me(c *gin.Context) {
	out, err := h.service.Me(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) changePassword(c *gin.Context) {
	var req ChangePasswordInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	if err := h.service.ChangePassword(c.Request.Context(), req); err != nil {
		response.Fail(c, err)
		return
	}

	h.clearRefreshCookie(c)
	response.OK(c, gin.H{"ok": true})
}

func (h *Handler) setRefreshCookie(c *gin.Context, token string) {
	if token == "" {
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, token, int(h.service.RefreshTTL().Seconds()), refreshCookiePath, "", h.opts.SecureCookies, true)
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", h.opts.SecureCookies, true)
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
