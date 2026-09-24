package middleware

import (
	"net/http"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// resellerAllowedPrefixes lists the panel-API namespaces a reseller
// (نمایندگی) session may reach. Everything else — settings, xray config, nodes,
// server management, API tokens, backups and other resellers — is admin-only,
// with the single exception of resellerAllowedExact (the shared, non-sensitive
// default-settings read used by the UI chrome).
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

// resellerAllowedExact are read-only, non-sensitive endpoints outside the
// self-service namespaces that the shared UI still needs: the default-settings
// payload drives the datepicker (Jalali calendar), page size, client share
// links and expiry/traffic thresholds. It carries no per-tenant data and no
// credentials. Everything else under /panel/api/setting/ (all, update,
// updateUser, restartPanel, apiTokens) stays admin-only.
var resellerAllowedExact = []string{
	"/panel/api/setting/defaultSettings",
}

// ResellerGuard blocks reseller sessions from the admin-only parts of the panel
// API. Admin sessions (and API-token requests) pass through untouched.
func ResellerGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		reseller := session.GetLoginReseller(c)
		if reseller == nil {
			c.Next()
			return
		}
		// Session epochs invalidate password/enable changes, but an expiry
		// timestamp can pass without an epoch bump. Enforce the account state at
		// the API boundary as well, so an already-open reseller session cannot
		// continue to read or mutate tenant data after it expires or is disabled.
		if err := (&service.ResellerService{}).EnsureActive(reseller); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"msg":     err.Error(),
			})
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
	for _, exact := range resellerAllowedExact {
		if path == exact {
			return true
		}
	}
	for _, prefix := range resellerAllowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
