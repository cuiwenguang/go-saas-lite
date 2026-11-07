// Package storage 定义对象存储接口与本地驱动。
//
// 业务只依赖 Storage；腾讯 COS / 阿里 OSS 驱动放在子包，按配置启用。
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotFound 表示对象不存在。
	ErrNotFound = errors.New("storage: object not found")
	// ErrInvalidKey 表示对象 key 非法。
	ErrInvalidKey = errors.New("storage: invalid object key")
	// ErrTooLarge 表示对象超过驱动允许的大小。
	ErrTooLarge = errors.New("storage: object too large")
	// ErrSignatureExpired 表示签名下载地址已过期。
	ErrSignatureExpired = errors.New("storage: signed url expired")
	// ErrSignatureInvalid 表示签名不匹配。
	ErrSignatureInvalid = errors.New("storage: invalid signed url")
)

// Storage 是对象存储的最小接口。
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// URLVerifier 由本地驱动实现，供无鉴权下载路由校验签名。
type URLVerifier interface {
	Verify(key, expires, sig string) error
}
