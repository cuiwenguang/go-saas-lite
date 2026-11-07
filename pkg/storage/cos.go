// Package storage 的腾讯云 COS 驱动。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// COSOptions 是腾讯云 COS 驱动配置。
type COSOptions struct {
	Bucket    string
	Region    string
	SecretID  string
	SecretKey string
	// Endpoint 可选，默认 https://{bucket}.cos.{region}.myqcloud.com。
	Endpoint string
	// MaxBytes 是单对象上限；0 表示不限制（由上层校验）。
	MaxBytes int64
}

// COS 把对象保存在腾讯云 COS。
type COS struct {
	client    *cos.Client
	secretID  string
	secretKey string
	maxBytes  int64
}

// NewCOS 创建 COS 驱动；缺少必需参数时返回错误。
func NewCOS(opts COSOptions) (*COS, error) {
	bucket := strings.TrimSpace(opts.Bucket)
	region := strings.TrimSpace(opts.Region)
	secretID := strings.TrimSpace(opts.SecretID)
	secretKey := strings.TrimSpace(opts.SecretKey)
	if bucket == "" || secretID == "" || secretKey == "" {
		return nil, errors.New("storage: cos bucket, secret_id and secret_key are required")
	}

	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		if region == "" {
			return nil, errors.New("storage: cos region is required")
		}
		endpoint = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucket, region)
	}

	bucketURL, err := url.Parse(endpoint)
	if err != nil || bucketURL.Host == "" {
		return nil, fmt.Errorf("storage: invalid cos endpoint %q", endpoint)
	}

	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &cos.AuthorizationTransport{SecretID: secretID, SecretKey: secretKey},
	})

	return &COS{client: client, secretID: secretID, secretKey: secretKey, maxBytes: opts.MaxBytes}, nil
}

// Put 上传对象；已知 size 时写入 Content-Length。
func (c *COS) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if key == "" {
		return ErrInvalidKey
	}
	if c.maxBytes > 0 && size > c.maxBytes {
		return ErrTooLarge
	}

	header := &cos.ObjectPutHeaderOptions{ContentType: contentType}
	if size > 0 {
		header.ContentLength = size
	}

	_, err := c.client.Object.Put(ctx, key, r, &cos.ObjectPutOptions{ObjectPutHeaderOptions: header})
	if err != nil {
		return fmt.Errorf("storage: cos put %s: %w", key, err)
	}

	return nil
}

// Get 下载对象；404 转为 ErrNotFound。
func (c *COS) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := c.client.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, mapCOSError(key, err)
	}

	return resp.Body, nil
}

// Delete 删除对象；对象不存在时视为成功。
func (c *COS) Delete(ctx context.Context, key string) error {
	if _, err := c.client.Object.Delete(ctx, key); err != nil {
		if isCOSNotFound(err) {
			return nil
		}
		return fmt.Errorf("storage: cos delete %s: %w", key, err)
	}

	return nil
}

// Exists 判断对象是否存在。
func (c *COS) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := c.client.Object.IsExist(ctx, key)
	if err != nil {
		if isCOSNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("storage: cos head %s: %w", key, err)
	}

	return ok, nil
}

// SignedURL 生成临时下载地址。
func (c *COS) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	signed, err := c.client.Object.GetPresignedURL(ctx, http.MethodGet, key, c.secretID, c.secretKey, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("storage: cos sign %s: %w", key, err)
	}

	return signed.String(), nil
}

func mapCOSError(key string, err error) error {
	if isCOSNotFound(err) {
		return ErrNotFound
	}

	return fmt.Errorf("storage: cos get %s: %w", key, err)
}

func isCOSNotFound(err error) bool {
	if resp, ok := cos.IsCOSError(err); ok && resp.Response != nil {
		return resp.Response.StatusCode == http.StatusNotFound
	}

	return false
}
