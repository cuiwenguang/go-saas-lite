// Package storage 的阿里云 OSS 驱动。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSOptions 是阿里云 OSS 驱动配置。
type OSSOptions struct {
	Bucket          string
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	// MaxBytes 是单对象上限；0 表示不限制（由上层校验）。
	MaxBytes int64
}

// OSS 把对象保存在阿里云 OSS。
type OSS struct {
	bucket   *oss.Bucket
	maxBytes int64
}

// NewOSS 创建 OSS 驱动；缺少必需参数时返回错误。
func NewOSS(opts OSSOptions) (*OSS, error) {
	bucketName := strings.TrimSpace(opts.Bucket)
	endpoint := strings.TrimSpace(opts.Endpoint)
	keyID := strings.TrimSpace(opts.AccessKeyID)
	keySecret := strings.TrimSpace(opts.AccessKeySecret)
	if bucketName == "" || endpoint == "" || keyID == "" || keySecret == "" {
		return nil, errors.New("storage: oss bucket, endpoint, access_key_id and access_key_secret are required")
	}

	client, err := oss.New(endpoint, keyID, keySecret)
	if err != nil {
		return nil, fmt.Errorf("storage: create oss client: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("storage: open oss bucket %s: %w", bucketName, err)
	}

	return &OSS{bucket: bucket, maxBytes: opts.MaxBytes}, nil
}

// Put 上传对象。
func (o *OSS) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if key == "" {
		return ErrInvalidKey
	}
	if o.maxBytes > 0 && size > o.maxBytes {
		return ErrTooLarge
	}

	options := make([]oss.Option, 0, 2)
	if contentType != "" {
		options = append(options, oss.ContentType(contentType))
	}
	if size > 0 {
		options = append(options, oss.ContentLength(size))
	}

	if err := o.bucket.PutObject(key, r, options...); err != nil {
		return fmt.Errorf("storage: oss put %s: %w", key, err)
	}

	return nil
}

// Get 下载对象；404 转为 ErrNotFound。
func (o *OSS) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	body, err := o.bucket.GetObject(key)
	if err != nil {
		if isOSSNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: oss get %s: %w", key, err)
	}

	return body, nil
}

// Delete 删除对象；对象不存在时视为成功。
func (o *OSS) Delete(ctx context.Context, key string) error {
	if err := o.bucket.DeleteObject(key); err != nil {
		if isOSSNotFound(err) {
			return nil
		}
		return fmt.Errorf("storage: oss delete %s: %w", key, err)
	}

	return nil
}

// Exists 判断对象是否存在。
func (o *OSS) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := o.bucket.IsObjectExist(key)
	if err != nil {
		return false, fmt.Errorf("storage: oss head %s: %w", key, err)
	}

	return ok, nil
}

// SignedURL 生成临时下载地址。
func (o *OSS) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	signed, err := o.bucket.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()))
	if err != nil {
		return "", fmt.Errorf("storage: oss sign %s: %w", key, err)
	}

	return signed, nil
}

func isOSSNotFound(err error) bool {
	var serviceErr oss.ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.StatusCode == http.StatusNotFound
	}

	return false
}
