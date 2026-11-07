package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LocalOptions 是本地磁盘驱动的配置。
type LocalOptions struct {
	// Root 是存储根目录。
	Root string
	// BaseURL 是签名下载路由地址，例如 http://localhost:8080/api/files/download。
	BaseURL string
	// SigningKey 用于签名下载地址。
	SigningKey string
	// MaxBytes 是单对象大小上限；0 表示不限制。
	MaxBytes int64
}

// Local 把对象保存在本地磁盘，key 使用斜杠分隔的相对路径。
type Local struct {
	root       string
	baseURL    string
	signingKey []byte
	maxBytes   int64
}

// NewLocal 创建本地驱动并确保根目录存在。
func NewLocal(opts LocalOptions) (*Local, error) {
	if strings.TrimSpace(opts.Root) == "" {
		return nil, errors.New("storage: local root is required")
	}
	if opts.SigningKey == "" {
		return nil, errors.New("storage: local signing key is required")
	}

	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve local root: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create local root: %w", err)
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "/api/files/download"
	}

	return &Local{
		root:       root,
		baseURL:    baseURL,
		signingKey: []byte(opts.SigningKey),
		maxBytes:   opts.MaxBytes,
	}, nil
}

// Put 原子写入对象：先写临时文件，再 rename。
func (l *Local) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	full, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("storage: create object dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return fmt.Errorf("storage: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	reader := r
	if l.maxBytes > 0 {
		reader = io.LimitReader(r, l.maxBytes+1)
	}

	written, copyErr := io.Copy(tmp, reader)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage: write object: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage: close object: %w", closeErr)
	}
	if l.maxBytes > 0 && written > l.maxBytes {
		_ = os.Remove(tmpName)
		return ErrTooLarge
	}

	if err := os.Rename(tmpName, full); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage: commit object: %w", err)
	}

	return nil
}

// Get 打开对象。
func (l *Local) Get(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := l.path(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Delete 删除对象；对象不存在时视为成功。
func (l *Local) Delete(_ context.Context, key string) error {
	full, err := l.path(key)
	if err != nil {
		return err
	}

	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// Exists 判断对象是否存在。
func (l *Local) Exists(_ context.Context, key string) (bool, error) {
	full, err := l.path(key)
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(full); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// SignedURL 返回带过期时间与 HMAC 签名的下载地址。
func (l *Local) SignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	if _, err := l.path(key); err != nil {
		return "", err
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	expires := time.Now().Add(ttl).Unix()
	separator := "?"
	if strings.Contains(l.baseURL, "?") {
		separator = "&"
	}

	return fmt.Sprintf("%s%skey=%s&expires=%d&sig=%s",
		l.baseURL,
		separator,
		url.QueryEscape(key),
		expires,
		l.sign(key, expires),
	), nil
}

// Verify 校验签名与过期时间。
func (l *Local) Verify(key, expires, sig string) error {
	if _, err := l.path(key); err != nil {
		return err
	}

	timestamp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return ErrSignatureInvalid
	}
	if time.Now().Unix() > timestamp {
		return ErrSignatureExpired
	}

	expected := l.sign(key, timestamp)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) != 1 {
		return ErrSignatureInvalid
	}

	return nil
}

func (l *Local) sign(key string, expires int64) string {
	mac := hmac.New(sha256.New, l.signingKey)
	_, _ = fmt.Fprintf(mac, "%s|%d", key, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

// path 把对象 key 解析为根目录下的绝对路径，并阻止路径穿越。
func (l *Local) path(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsRune(key, 0) {
		return "", ErrInvalidKey
	}
	if strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return "", ErrInvalidKey
	}

	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}

	full := filepath.Join(l.root, clean)
	rel, err := filepath.Rel(l.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}

	return full, nil
}
