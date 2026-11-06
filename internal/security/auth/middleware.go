package auth

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/apperr"
	"github.com/cuiwenguang/go-saas-lite/pkg/obs"
	"github.com/cuiwenguang/go-saas-lite/pkg/response"
	"github.com/cuiwenguang/go-saas-lite/pkg/tenancy"
)

// Rule 描述一条入口链允许的令牌：audience + actor 必须同时命中。
type Rule struct {
	Audience string
	Actor    tenancy.ActorType
}

// Guard 同时完成鉴权与 actor_guard：
// 解析 Bearer、校验 aud、校验 actor_type，并把 Scope 写入 ctx。
func Guard(issuer *Issuer, rule Rule) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := Authenticate(c, issuer, rule)
		if !ok {
			return
		}

		ctx := tenancy.WithScope(c.Request.Context(), scope)
		ctx = obs.WithLogAttrs(ctx,
			slog.String("tenant_id", scope.TenantID),
			slog.String("user_id", scope.UserID),
			slog.String("actor", string(scope.Actor)),
		)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// Authenticate 解析 Bearer 令牌并返回 Scope；失败时已写入响应，调用方直接返回。
func Authenticate(c *gin.Context, issuer *Issuer, rule Rule) (tenancy.Scope, bool) {
	raw, err := bearerToken(c.GetHeader("Authorization"))
	if err != nil {
		response.Fail(c, apperr.New(apperr.CodeUnauthenticated, "missing bearer token"))
		return tenancy.Scope{}, false
	}

	claims, err := issuer.Parse(raw, rule.Audience)
	if err != nil {
		response.Fail(c, authError(err))
		return tenancy.Scope{}, false
	}

	if claims.Actor() != rule.Actor {
		response.Fail(c, apperr.New(apperr.CodeActorMismatch, "token actor is not allowed on this endpoint"))
		return tenancy.Scope{}, false
	}

	scope := tenancy.Scope{
		TenantID: claims.TenantID,
		UserID:   claims.Subject,
		ClientID: claims.ClientID,
		Actor:    claims.Actor(),
	}
	if scope.Actor == tenancy.ActorPlatform {
		scope.Role = claims.Scope
	}
	if scope.Actor == tenancy.ActorTenant && scope.TenantID == "" {
		response.Fail(c, apperr.New(apperr.CodeInvalidToken, "tenant token without tenant_id"))
		return tenancy.Scope{}, false
	}

	return scope, true
}

func bearerToken(header string) (string, error) {
	if header == "" {
		return "", ErrTokenMissing
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", ErrTokenMissing
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrTokenMissing
	}

	return token, nil
}

func authError(err error) *apperr.Error {
	switch {
	case err == nil:
		return nil
	case isError(err, ErrTokenExpired):
		return apperr.New(apperr.CodeTokenExpired, "token expired")
	case isError(err, ErrAudienceMismatch):
		return apperr.New(apperr.CodeInvalidToken, "token audience mismatch")
	case isError(err, ErrTokenMissing):
		return apperr.New(apperr.CodeUnauthenticated, "missing bearer token")
	default:
		return apperr.New(apperr.CodeInvalidToken, "invalid token")
	}
}
