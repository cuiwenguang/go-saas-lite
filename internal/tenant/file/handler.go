package file

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/internal/security/authz"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Handler 暴露租户文件接口与本地签名下载。
type Handler struct {
	service    *Service
	authorizer authz.Authorizer
}

// NewHandler 创建文件 handler。
func NewHandler(service *Service, authorizer authz.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// Register 挂载租户文件接口与无鉴权签名下载。
func (h *Handler) Register(group *gin.RouterGroup, site *gin.RouterGroup) {
	files := group.Group("/files")
	files.POST("", authz.Require(h.authorizer, authz.PermFileCreate), h.upload)
	files.GET("", authz.Require(h.authorizer, authz.PermFileRead), h.list)
	files.DELETE("/:id", authz.Require(h.authorizer, authz.PermFileDelete), h.remove)
	files.GET("/:id/url", authz.Require(h.authorizer, authz.PermFileRead), h.url)

	if site != nil {
		site.GET("/api/files/download", h.download)
	}
}

func (h *Handler) upload(c *gin.Context) {
	if max := h.service.MaxUploadBytes(); max > 0 {
		// 预留 1MB 给 multipart 边界与其他表单字段。
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max+1<<20)
	}

	biz := c.DefaultPostForm("biz", "default")
	header, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "file is required"))
		return
	}

	file, err := header.Open()
	if err != nil {
		response.Fail(c, apperr.Wrap(err, apperr.CodeInvalidArgument, "open upload"))
		return
	}
	defer func() { _ = file.Close() }()

	head := make([]byte, 512)
	n, readErr := io.ReadFull(file, head)
	if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
		response.Fail(c, apperr.Wrap(readErr, apperr.CodeInvalidArgument, "read upload"))
		return
	}
	head = head[:n]

	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(head)
	}

	out, err := h.service.Upload(
		c.Request.Context(),
		biz,
		header.Filename,
		contentType,
		header.Size,
		io.MultiReader(bytes.NewReader(head), file),
	)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, out)
}

func (h *Handler) list(c *gin.Context) {
	items, err := h.service.List(c.Request.Context(), c.Query("biz"))
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"items": items})
}

func (h *Handler) remove(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"deleted": true})
}

func (h *Handler) url(c *gin.Context) {
	ttl := 15 * time.Minute
	if raw := c.Query("ttl"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			response.Fail(c, apperr.New(apperr.CodeInvalidArgument, "ttl must be a positive integer"))
			return
		}
		if seconds > 86400 {
			seconds = 86400
		}
		ttl = time.Duration(seconds) * time.Second
	}

	signed, err := h.service.SignedURL(c.Request.Context(), c.Param("id"), ttl)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"url": signed, "expires_in": int(ttl.Seconds())})
}

// download 只校验签名，不要求登录；云端驱动直接返回预签名地址，不走此路由。
func (h *Handler) download(c *gin.Context) {
	key := c.Query("key")
	expires := c.Query("expires")
	sig := c.Query("sig")
	if key == "" || expires == "" || sig == "" {
		c.String(http.StatusBadRequest, "missing signature")
		return
	}

	if err := h.service.VerifySignedURL(key, expires, sig); err != nil {
		c.String(http.StatusForbidden, "invalid or expired signature")
		return
	}

	reader, err := h.service.Open(c.Request.Context(), key)
	if err != nil {
		if apperr.Is(err, apperr.CodeNotFound) {
			c.String(http.StatusNotFound, "file not found")
			return
		}
		c.String(http.StatusInternalServerError, "open file failed")
		return
	}
	defer func() { _ = reader.Close() }()

	contentType := mime.TypeByExtension(path.Ext(key))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	c.DataFromReader(http.StatusOK, -1, contentType, reader, nil)
}
