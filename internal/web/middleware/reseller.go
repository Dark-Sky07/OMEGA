package middleware

import (
	"net/http"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// resellerAllowedPrefixes lists the panel-API namespaces a reseller
// (نمایندگی) session may reach. Everything else — settings, xray config, nodes,
// server management, API tokens, backups and other resellers — is admin-only.
//
// Ownership inside the allowed namespaces is enforced by the handlers
// themselves: a reseller only ever sees and edits its own inbounds/clients.
var resellerAllowedPrefixes = []string{
	"/panel/api/inbounds/",
	"/panel/api/clients/",
	"/panel/api/reseller/", // singular: the reseller's own endpoints
	"/panel/api/auth/",
}

// resellerDeniedExact are reseller-visible namespaces that are still off
// limits because they expose panel-wide data (client groups are global).
var resellerDeniedExact = []string{
	"/panel/api/clients/groups",
}

// ResellerGuard blocks reseller sessions from the admin-only parts of the panel
// API. Admin sessions (and API-token requests) pass through untouched.
func ResellerGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if session.GetLoginReseller(c) == nil {
			c.Next()
			return
		}

		path := requestPathWithoutBase(c)
		if isResellerAllowed(path) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"msg":     "this section is not available for reseller accounts",
		})
	}
}

func requestPathWithoutBase(c *gin.Context) string {
	path := c.Request.URL.Path
	basePath := strings.TrimSuffix(c.GetString("base_path"), "/")
	if basePath != "" && strings.HasPrefix(path, basePath) {
		path = strings.TrimPrefix(path, basePath)
	}
	return path
}

func isResellerAllowed(path string) bool {
	for _, denied := range resellerDeniedExact {
		if path == denied || strings.HasPrefix(path, denied+"/") {
			return false
		}
	}
	for _, prefix := range resellerAllowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
