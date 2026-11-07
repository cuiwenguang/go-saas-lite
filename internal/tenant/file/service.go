package file

import (
	"context"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/cuiwenguang/go-saas-lite/internal/quota"
	"github.com/cuiwenguang/go-saas-lite/internal/tasktype"
	auditlog "github.com/cuiwenguang/go-saas-lite/internal/tenant/audit"
	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/queue"
	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

var (
	bizPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	extPattern = regexp.MustCompile(`^\.[a-z0-9]{1,8}$`)
)

// Options 是文件服务参数。
type Options struct {
	// MaxUploadBytes 是单文件上限；0 表示不限制。
	MaxUploadBytes int64
}

// Service 负责文件元数据、对象存储与存储配额。
type Service struct {
	db       tenancy.DB
	storage  storage.Storage
	quota    *quota.Service
	enqueuer *queue.Client
	audit    *auditlog.Service
	opts     Options
}

// NewService 创建文件服务；enqueuer 为 nil 时删除对象走同步路径。
func NewService(db tenancy.DB, store storage.Storage, quotaService *quota.Service, enqueuer *queue.Client, auditService *auditlog.Service, opts Options) *Service {
	return &Service{db: db, storage: store, quota: quotaService, enqueuer: enqueuer, audit: auditService, opts: opts}
}

// Upload 写入对象并登记元数据；对象写入失败不会留下元数据。
func (s *Service) Upload(ctx context.Context, biz, originalName, contentType string, size int64, r io.Reader) (*Output, error) {
	if err := validateBiz(biz); err != nil {
		return nil, err
	}
	if s.opts.MaxUploadBytes > 0 && size > s.opts.MaxUploadBytes {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "file exceeds %d bytes", s.opts.MaxUploadBytes).
			WithMeta("max_bytes", s.opts.MaxUploadBytes)
	}

	scope, err := tenancy.ScopeFrom(ctx)
	if err != nil {
		return nil, err
	}
	if scope.TenantID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "tenant identity required")
	}

	name := sanitizeName(originalName)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id := uuid.Must(uuid.NewV7()).String()
	key := path.Join("tenant", scope.TenantID, biz, id+extension(name))

	if err := s.checkStorageQuota(ctx, scope.TenantID, size); err != nil {
		return nil, err
	}

	if err := s.storage.Put(ctx, key, r, size, contentType); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return nil, apperr.New(apperr.CodeInvalidArgument, "file too large")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "store file")
	}

	row := &File{
		ID:           id,
		Key:          key,
		Biz:          biz,
		OriginalName: name,
		ContentType:  contentType,
		SizeBytes:    size,
		CreatedAt:    time.Now(),
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Create(row)
	}); err != nil {
		_ = s.storage.Delete(ctx, key)
		return nil, apperr.Wrap(err, apperr.CodeInternal, "save file metadata")
	}

	out := toOutput(*row)

	s.recordAudit(ctx, auditlog.Entry{
		Category:   auditlog.CategoryFile,
		Action:     auditlog.ActionFileUpload,
		ObjectType: "file",
		ObjectID:   row.ID,
		Summary:    "上传文件 " + row.OriginalName,
		After:      map[string]any{"biz": row.Biz, "name": row.OriginalName, "size": row.SizeBytes},
	})

	return &out, nil
}

// List 返回当前租户的文件，可选按 biz 过滤。
func (s *Service) List(ctx context.Context, biz string) ([]Output, error) {
	if biz != "" {
		if err := validateBiz(biz); err != nil {
			return nil, err
		}
	}

	var rows []File
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		query := tx.Model(&File{})
		if biz != "" {
			query = query.Where("biz = ?", biz)
		}
		return query.Order("created_at DESC").Limit(200).Find(&rows)
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list files")
	}

	out := make([]Output, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOutput(row))
	}

	return out, nil
}

// Delete 删除对象与元数据；对象不存在时继续删元数据。
func (s *Service) Delete(ctx context.Context, id string) error {
	row, err := s.load(ctx, id)
	if err != nil {
		return err
	}

	if err := s.db.Tx(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).Delete(&File{})
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "delete file metadata")
	}

	if s.enqueuer != nil {
		err := s.enqueuer.Enqueue(ctx, queue.Task{
			Type:     tasktype.FileDelete,
			Payload:  tasktype.FileDeletePayload{Key: row.Key},
			ID:       "file-delete:" + row.ID,
			MaxRetry: 5,
		})
		if err == nil {
			s.recordAudit(ctx, auditlog.Entry{
				Category:   auditlog.CategoryFile,
				Action:     auditlog.ActionFileDelete,
				Level:      auditlog.LevelWarn,
				ObjectType: "file",
				ObjectID:   row.ID,
				Summary:    "删除文件 " + row.OriginalName,
			})

			return nil
		}
	}

	if err := s.storage.Delete(ctx, row.Key); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return apperr.Wrap(err, apperr.CodeInternal, "delete stored file")
	}

	s.recordAudit(ctx, auditlog.Entry{
		Category:   auditlog.CategoryFile,
		Action:     auditlog.ActionFileDelete,
		Level:      auditlog.LevelWarn,
		ObjectType: "file",
		ObjectID:   row.ID,
		Summary:    "删除文件 " + row.OriginalName,
	})

	return nil
}

// SignedURL 返回文件的临时下载地址。
func (s *Service) SignedURL(ctx context.Context, id string, ttl time.Duration) (string, error) {
	row, err := s.load(ctx, id)
	if err != nil {
		return "", err
	}

	url, err := s.storage.SignedURL(ctx, row.Key, ttl)
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "sign file url")
	}

	return url, nil
}

// Open 按对象 key 打开文件，供签名下载路由使用。
func (s *Service) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	reader, err := s.storage.Get(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "file not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "open stored file")
	}

	return reader, nil
}

// MaxUploadBytes 返回单文件上限。
func (s *Service) MaxUploadBytes() int64 {
	return s.opts.MaxUploadBytes
}

func (s *Service) recordAudit(ctx context.Context, entry auditlog.Entry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, entry)
}

func (s *Service) load(ctx context.Context, id string) (*File, error) {
	var row File
	err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Where("id = ?", id).First(&row)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.New(apperr.CodeNotFound, "file not found")
	}
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load file")
	}

	return &row, nil
}

func (s *Service) checkStorageQuota(ctx context.Context, tenantID string, incoming int64) error {
	if s.quota == nil {
		return nil
	}

	limits, err := s.quota.Limits(ctx, tenantID)
	if err != nil {
		return err
	}
	if limits.StorageGB <= 0 {
		return nil
	}

	var used int64
	if err := s.db.Read(ctx, func(tx tenancy.Tx) error {
		return tx.Raw(storageUsageSQL, tenantID).Scan(&used)
	}); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "read storage usage")
	}

	capacity := int64(limits.StorageGB) * 1024 * 1024 * 1024
	if used+incoming > capacity {
		return apperr.New(apperr.CodeQuotaExceeded, "storage quota exceeded").
			WithMeta("metric", "storage_gb").
			WithMeta("limit", capacity).
			WithMeta("used", used)
	}

	return nil
}

func validateBiz(biz string) error {
	if !bizPattern.MatchString(biz) {
		return apperr.New(apperr.CodeInvalidArgument, "biz must match ^[a-z0-9][a-z0-9_-]{0,63}$")
	}

	return nil
}

// sanitizeName 只保留展示用的文件名，去掉路径与控制字符。
func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	if len(name) > 200 {
		name = name[len(name)-200:]
	}

	return name
}

func extension(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if !extPattern.MatchString(ext) {
		return ""
	}

	return ext
}

func toOutput(row File) Output {
	return Output{
		ID:           row.ID,
		Key:          row.Key,
		Biz:          row.Biz,
		OriginalName: row.OriginalName,
		ContentType:  row.ContentType,
		SizeBytes:    row.SizeBytes,
		CreatedAt:    row.CreatedAt,
	}
}
