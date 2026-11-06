package conf

import (
	"context"
	"encoding/json"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
)

// Get 读取配置并反序列化为 T，业务不直接查表。
func Get[T any](ctx context.Context, service *Service, namespace, key string) (T, error) {
	var zero T

	raw, err := service.Get(ctx, namespace, key)
	if err != nil {
		return zero, err
	}

	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return zero, apperr.Wrap(err, apperr.CodeInternal, "decode tenant config")
	}

	return value, nil
}
