package members

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露租户成员与角色接口。
type Handler struct {
	service    *Service
	authorizer authz.Authorizer
}

// NewHandler 创建 handler。
func NewHandler(service *Service, authorizer authz.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// Register 挂到租户链上，每个接口显式声明权限点。
func (h *Handler) Register(group *gin.RouterGroup) {
	users := group.Group("/users")
	users.GET("", authz.Require(h.authorizer, authz.PermUserRead), h.list)
	users.POST("", authz.Require(h.authorizer, authz.PermUserCreate), h.create)
	users.PATCH("/:id", authz.Require(h.authorizer, authz.PermUserUpdate), h.update)
	users.POST("/:id/password", authz.Require(h.authorizer, authz.PermUserUpdate), h.resetPassword)
	users.PATCH("/:id/roles", authz.Require(h.authorizer, authz.PermUserUpdate), h.setRoles)

	roles := group.Group("/roles")
	roles.GET("", authz.Require(h.authorizer, authz.PermRoleRead), h.listRoles)
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("size"))

	items, total, err := h.service.List(c.Request.Context(), ListFilter{
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
		Page:    page,
		Size:    size,
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	page, size = normalizePage(page, size)
	response.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	out, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	out, err := h.service.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) resetPassword(c *gin.Context) {
	password, err := h.service.ResetPassword(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"initial_password": password})
}

func (h *Handler) setRoles(c *gin.Context) {
	var req SetRolesInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	out, err := h.service.SetRoles(c.Request.Context(), c.Param("id"), req.Codes)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) listRoles(c *gin.Context) {
	items, err := h.service.Roles(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": items})
}
