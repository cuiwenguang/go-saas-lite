package authz

import (
	"github.com/gin-gonic/gin"

	"github.com/cuiwenguang/go-saas-lite/pkg/response"
)

// Require 是权限中间件：当前用户必须拥有 perm 才能进入 handler。
// tenant_id 与 user_id 只来自 auth.Guard 写入 ctx 的 Scope。
func Require(authorizer Authorizer, perm Perm) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := authorizer.Enforce(c.Request.Context(), perm.Obj, perm.Act); err != nil {
			response.Fail(c, err)
			return
		}

		c.Next()
	}
}
