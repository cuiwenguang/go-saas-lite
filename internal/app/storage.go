package app

import (
	"fmt"
	"strings"

	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
)

// newStorage 按配置选择对象存储驱动；默认本地磁盘。
func newStorage(cfg Config) (storage.Storage, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Storage.Driver))
	maxBytes := int64(cfg.Storage.MaxUploadMB) * 1024 * 1024
	switch driver {
	case "", "local":
		signingKey := cfg.Storage.Local.SigningKey
		if signingKey == "" {
			signingKey = cfg.Auth.Secret
		}

		local, err := storage.NewLocal(storage.LocalOptions{
			Root:       cfg.Storage.Local.Root,
			BaseURL:    cfg.Storage.Local.BaseURL,
			SigningKey: signingKey,
			MaxBytes:   maxBytes,
		})
		if err != nil {
			return nil, err
		}
		return local, nil
	case "cos":
		return storage.NewCOS(storage.COSOptions{
			Bucket:    cfg.Storage.COS.Bucket,
			Region:    cfg.Storage.COS.Region,
			SecretID:  cfg.Storage.COS.SecretID,
			SecretKey: cfg.Storage.COS.SecretKey,
			Endpoint:  cfg.Storage.COS.Endpoint,
			MaxBytes:  maxBytes,
		})
	case "oss":
		return storage.NewOSS(storage.OSSOptions{
			Bucket:          cfg.Storage.OSS.Bucket,
			Endpoint:        cfg.Storage.OSS.Endpoint,
			AccessKeyID:     cfg.Storage.OSS.AccessKeyID,
			AccessKeySecret: cfg.Storage.OSS.AccessKeySecret,
			MaxBytes:        maxBytes,
		})
	default:
		return nil, fmt.Errorf("unsupported storage driver %q", cfg.Storage.Driver)
	}
}
