package tenants

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露平台侧租户接口。
type Handler struct {
	service *Service
}

// NewHandler 创建 handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register 挂到平台路由链上；每个接口显式声明所需权限点。
func (h *Handler) Register(group *gin.RouterGroup) {
	tenants := group.Group("/tenants")
	tenants.POST("", authz.PlatformRequire(authz.TenantCreate), h.create)
	tenants.GET("", authz.PlatformRequire(authz.TenantRead), h.list)
	/* /summary 必须排在 /:id 之前，否则会被参数路由吃掉 */
	tenants.GET("/summary", authz.PlatformRequire(authz.TenantRead), h.summary)
	tenants.GET("/:id", authz.PlatformRequire(authz.TenantRead), h.get)
	tenants.PATCH("/:id", authz.PlatformRequire(authz.TenantUpdate), h.update)
	tenants.PATCH("/:id/status", authz.PlatformRequire(authz.TenantUpdate), h.updateStatus)
	tenants.POST("/:id/retry", authz.PlatformRequire(authz.TenantUpdate), h.retry)
	tenants.POST("/:id/admin-password", authz.PlatformRequire(authz.TenantUpdate), h.resetAdminPassword)
	tenants.GET("/:id/users", authz.PlatformRequire(authz.TenantRead), h.users)
	tenants.GET("/:id/usage", authz.PlatformRequire(authz.TenantRead), h.usage)
	tenants.DELETE("/:id", authz.PlatformRequire(authz.TenantDelete), h.archive)
}

func (h *Handler) create(c *gin.Context) {
	var req CreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	tenant, password, err := h.service.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	c.JSON(http.StatusCreated, response.Envelope{
		Code:      0,
		Msg:       "created",
		Data:      CredentialOutput{Tenant: toOutput(*tenant), InitialPassword: password},
		RequestID: response.RequestIDFrom(c.Request.Context()),
	})
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("size"))

	filter := ListFilter{
		Status:   c.Query("status"),
		Keyword:  c.Query("keyword"),
		PlanCode: c.Query("plan_code"),
		Page:     page,
		Size:     size,
	}
	if raw := c.Query("expire_before"); raw != "" {
		when, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "expire_before must be RFC3339"))
			return
		}
		filter.ExpireBefore = &when
	}

	items, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.Fail(c, err)
		return
	}

	page, size = normalizePage(page, size)
	response.OK(c, gin.H{"items": toOutputs(items), "total": total, "page": page, "size": size})
}

func (h *Handler) summary(c *gin.Context) {
	out, err := h.service.Summary(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) get(c *gin.Context) {
	tenant, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, toOutput(*tenant))
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	tenant, err := h.service.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, toOutput(*tenant))
}

func (h *Handler) retry(c *gin.Context) {
	tenant, password, err := h.service.RetryProvision(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, CredentialOutput{Tenant: toOutput(*tenant), InitialPassword: password})
}

func (h *Handler) resetAdminPassword(c *gin.Context) {
	tenant, password, err := h.service.ResetAdminPassword(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, CredentialOutput{Tenant: toOutput(*tenant), InitialPassword: password})
}

func (h *Handler) users(c *gin.Context) {
	items, err := h.service.Users(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": items})
}

func (h *Handler) usage(c *gin.Context) {
	out, err := h.service.Usage(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) updateStatus(c *gin.Context) {
	var req UpdateStatusInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}
	if req.Status == "" {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "status is required"))
		return
	}

	tenant, err := h.service.UpdateStatus(c.Request.Context(), c.Param("id"), req.Status)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, toOutput(*tenant))
}

func (h *Handler) archive(c *gin.Context) {
	if err := h.service.Archive(c.Request.Context(), c.Param("id")); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"status": StatusArchived})
}
