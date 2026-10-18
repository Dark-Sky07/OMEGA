package middleware

import (
	"net/http"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// AdminRBACGuard enforces role- and module-based permissions for secondary
// admin accounts (support, viewer, custom-permission admin). Primary
// superadmin (id=1 or role=="" / "superadmin"), reseller sessions, and
// API-token requests pass through untouched.
func AdminRBACGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if session.IsResellerLogin(c) || c.GetBool("api_authed") {
			c.Next()
			return
		}
		user := session.GetLoginUser(c)
		if user == nil {
			c.Next()
			return
		}
		path := requestPathWithoutBase(c)
		if panel.HasAdminPermission(user, path, c.Request.Method) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"msg":     "insufficient admin role permissions for this operation",
		})
	}
}
