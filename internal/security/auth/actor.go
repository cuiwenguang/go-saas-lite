package auth

import "errors"

// isError 复用 errors.Is，集中一处便于后续替换成错误码映射。
func isError(err, target error) bool {
	return errors.Is(err, target)
}
