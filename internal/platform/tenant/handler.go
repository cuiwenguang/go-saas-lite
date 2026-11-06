package tenant

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

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

// Register 挂到平台路由链上。
func (h *Handler) Register(group *gin.RouterGroup) {
	tenants := group.Group("/tenants")
	tenants.POST("", h.create)
	tenants.GET("", h.list)
	tenants.GET("/:id", h.get)
	tenants.PATCH("/:id/status", h.updateStatus)
	tenants.DELETE("/:id", h.archive)
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
		Data:      gin.H{"tenant": tenant, "initial_password": password},
		RequestID: response.RequestIDFrom(c.Request.Context()),
	})
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("size"))

	items, total, err := h.service.List(c.Request.Context(), ListFilter{
		Status: c.Query("status"),
		Page:   page,
		Size:   size,
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": items, "total": total})
}

func (h *Handler) get(c *gin.Context) {
	tenant, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, tenant)
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

	response.OK(c, tenant)
}

func (h *Handler) archive(c *gin.Context) {
	if err := h.service.Archive(c.Request.Context(), c.Param("id")); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"status": StatusArchived})
}
