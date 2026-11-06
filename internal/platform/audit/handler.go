package audit

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露平台侧审计查询接口。
type Handler struct {
	service *Service
}

// NewHandler 创建 handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register 挂到平台路由链上。
func (h *Handler) Register(group *gin.RouterGroup) {
	logs := group.Group("/audit-logs")
	logs.GET("", authz.PlatformRequire(authz.AuditRead), h.list)
}

func (h *Handler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("size"))

	filter := Filter{
		Category: c.Query("category"),
		Action:   c.Query("action"),
		Level:    c.Query("level"),
		Actor:    c.Query("actor"),
		ObjectID: c.Query("object_id"),
		Page:     page,
		Size:     size,
	}

	if raw := c.Query("from"); raw != "" {
		when, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "from must be RFC3339"))
			return
		}
		filter.From = &when
	}
	if raw := c.Query("to"); raw != "" {
		when, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "to must be RFC3339"))
			return
		}
		filter.To = &when
	}

	items, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.Fail(c, err)
		return
	}

	page, size = normalizePage(page, size)
	response.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}
