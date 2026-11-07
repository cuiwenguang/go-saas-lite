package account

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

const (
	refreshCookieName = "saas_platform_refresh"
	refreshCookiePath = "/api/platform/v1/auth"
)

// Options 是平台账号 HTTP 侧选项。
type Options struct {
	// SecureCookies 决定 refresh cookie 是否带 Secure（生产走 HTTPS 时为 true）。
	SecureCookies bool
}

// Handler 处理平台会话与账号自助接口。
type Handler struct {
	service *Service
	opts    Options
}

// NewHandler 创建 handler。
func NewHandler(service *Service, opts Options) *Handler {
	return &Handler{service: service, opts: opts}
}

// RegisterPublic 挂免鉴权的会话接口（登录 / 刷新 / 登出）。
func (h *Handler) RegisterPublic(group *gin.RouterGroup) {
	session := group.Group("/auth")
	session.POST("/login", h.login)
	session.POST("/refresh", h.refresh)
	session.POST("/logout", h.logout)
}

// Register 挂需要平台令牌的账号接口。
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/me", h.me)
	group.POST("/auth/password", h.changePassword)
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

func (h *Handler) refresh(c *gin.Context) {
	raw, err := c.Cookie(refreshCookieName)
	if err != nil || raw == "" {
		response.Fail(c, apperr.New(apperr.CodeUnauthenticated, "missing refresh token"))
		return
	}

	out, err := h.service.Refresh(c.Request.Context(), raw)
	if err != nil {
		h.clearRefreshCookie(c)
		response.Fail(c, err)
		return
	}

	h.setRefreshCookie(c, out.RefreshToken)
	response.OK(c, out)
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
	scope, err := tenancy.ScopeFrom(c.Request.Context())
	if err != nil || scope.UserID == "" {
		response.Fail(c, apperr.New(apperr.CodeUnauthenticated, "platform identity required"))
		return
	}

	out, err := h.service.Me(c.Request.Context(), scope.UserID)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) changePassword(c *gin.Context) {
	scope, err := tenancy.ScopeFrom(c.Request.Context())
	if err != nil || scope.UserID == "" {
		response.Fail(c, apperr.New(apperr.CodeUnauthenticated, "platform identity required"))
		return
	}

	var req ChangePasswordInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	if err := h.service.ChangePassword(c.Request.Context(), scope.UserID, req); err != nil {
		response.Fail(c, err)
		return
	}

	// 改密后 refresh 令牌已作废，清掉 cookie，前端需重新登录。
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
