package file

import (
	"errors"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
)

// VerifySignedURL 校验本地驱动的签名下载参数。
func (s *Service) VerifySignedURL(key, expires, sig string) error {
	verifier, ok := s.storage.(storage.URLVerifier)
	if !ok {
		return apperr.New(apperr.CodeNotFound, "signed url is not supported by this driver")
	}

	if err := verifier.Verify(key, expires, sig); err != nil {
		if errors.Is(err, storage.ErrSignatureExpired) || errors.Is(err, storage.ErrSignatureInvalid) {
			return apperr.New(apperr.CodeForbidden, "invalid or expired signature")
		}
		return apperr.Wrap(err, apperr.CodeForbidden, "verify signature")
	}

	return nil
}
