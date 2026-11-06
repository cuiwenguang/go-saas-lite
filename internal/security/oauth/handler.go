package oauth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Handler 是 OAuth 端点的 HTTP 适配层。
type Handler struct {
	server *Server
}

// NewHandler 创建 handler。
func NewHandler(server *Server) *Handler {
	return &Handler{server: server}
}

// RegisterPublic 挂无需用户令牌的端点（注册 / 换令牌 / 撤销）。
func (h *Handler) RegisterPublic(group *gin.RouterGroup) {
	group.POST("/oauth/register", h.register)
	group.POST("/oauth/token", h.token)
	group.POST("/oauth/revoke", h.revoke)
}

// Register 挂需要用户会话的授权端点。
func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/oauth/authorize", h.authorize)
}

func (h *Handler) register(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-Id")
	if tenantID == "" {
		if scope, err := tenancy.ScopeFrom(c.Request.Context()); err == nil {
			tenantID = scope.TenantID
		}
	}

	var req RegisterInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	client, err := h.server.Register(c.Request.Context(), tenantID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	c.JSON(http.StatusCreated, response.Envelope{
		Code: 0, Msg: "created", Data: client,
		RequestID: response.RequestIDFrom(c.Request.Context()),
	})
}

func (h *Handler) authorize(c *gin.Context) {
	scope, err := tenancy.ScopeFrom(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	redirect, err := h.server.Authorize(c.Request.Context(), scope, AuthorizeInput{
		ClientID:            c.Query("client_id"),
		RedirectURI:         c.Query("redirect_uri"),
		State:               c.Query("state"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.Query("code_challenge_method"),
		Scope:               c.Query("scope"),
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	c.Redirect(http.StatusFound, redirect)
}

func (h *Handler) token(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-Id")
	if tenantID == "" && strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") {
		var payload struct {
			TenantID string `json:"tenant_id"`
		}
		_ = c.ShouldBindJSON(&payload)
		tenantID = payload.TenantID
	}

	req := TokenInput{
		GrantType:    c.PostForm("grant_type"),
		Code:         c.PostForm("code"),
		RedirectURI:  c.PostForm("redirect_uri"),
		ClientID:     c.PostForm("client_id"),
		CodeVerifier: c.PostForm("code_verifier"),
		RefreshToken: c.PostForm("refresh_token"),
	}

	var (
		result *TokenOutput
		err    error
	)
	switch req.GrantType {
	case "refresh_token":
		result, err = h.server.Refresh(c.Request.Context(), tenantID, req.RefreshToken)
	default:
		result, err = h.server.Exchange(c.Request.Context(), req)
	}
	if err != nil {
		response.Fail(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) revoke(c *gin.Context) {
	tenantID := c.GetHeader("X-Tenant-Id")
	if err := h.server.Revoke(c.Request.Context(), tenantID, c.PostForm("token")); err != nil {
		response.Fail(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"revoked": true})
}
