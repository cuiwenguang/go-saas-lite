package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露 MCP 配置与调用日志接口。
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
	mcpGroup := group.Group("/mcp")

	mcpGroup.GET("/servers", authz.Require(h.authorizer, authz.PermMCPRead), h.listServers)
	mcpGroup.POST("/servers", authz.Require(h.authorizer, authz.PermMCPUpdate), h.createServer)
	mcpGroup.PATCH("/servers/:id", authz.Require(h.authorizer, authz.PermMCPUpdate), h.updateServer)
	mcpGroup.DELETE("/servers/:id", authz.Require(h.authorizer, authz.PermMCPUpdate), h.deleteServer)

	mcpGroup.GET("/tools", authz.Require(h.authorizer, authz.PermMCPRead), h.listTools)
	mcpGroup.PATCH("/tools/:id", authz.Require(h.authorizer, authz.PermMCPUpdate), h.updateTool)

	mcpGroup.GET("/calls", authz.Require(h.authorizer, authz.PermMCPRead), h.listCalls)
}

func (h *Handler) listServers(c *gin.Context) {
	items, err := h.service.ListServers(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": items})
}

func (h *Handler) createServer(c *gin.Context) {
	var req CreateServerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	out, err := h.service.CreateServer(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) updateServer(c *gin.Context) {
	var req UpdateServerInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	out, err := h.service.UpdateServer(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) deleteServer(c *gin.Context) {
	if err := h.service.DeleteServer(c.Request.Context(), c.Param("id")); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"deleted": true})
}

func (h *Handler) listTools(c *gin.Context) {
	items, err := h.service.ListTools(c.Request.Context(), c.Query("server_id"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": items})
}

func (h *Handler) updateTool(c *gin.Context) {
	var req UpdateToolInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "invalid request body"))
		return
	}

	out, err := h.service.UpdateTool(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) listCalls(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("size"))

	items, total, err := h.service.ListCalls(c.Request.Context(), CallFilter{
		Status: c.Query("status"),
		Tool:   c.Query("tool"),
		Page:   page,
		Size:   size,
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	page, size = normalizePage(page, size)
	response.OK(c, PageOutput{Items: items, Total: total, Page: page, Size: size})
}
