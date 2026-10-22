// Package middleware provides HTTP middleware functions for the PHOENIX web panel,
// including domain validation utilities.
package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"

	"github.com/gin-gonic/gin"
)

// DomainValidatorMiddleware returns a Gin middleware that validates the request domain.
// It extracts the host from the request, strips any port number, and compares it
// against the configured domain. Requests from unauthorized domains are rejected
// with HTTP 403 Forbidden status.
func DomainValidatorMiddleware(domain string) gin.HandlerFunc {
	return func(c *gin.Context) {
		host := requestHost(c.Request.Host)

		if host != normalizeRequestDomain(domain) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}

// SubscriptionDomainValidatorMiddleware permits the configured panel
// subscription host and reseller-owned custom hosts. Reseller domains are
// refreshed every 15 seconds to avoid a database query per request.
func SubscriptionDomainValidatorMiddleware(domain string) gin.HandlerFunc {
	var mu sync.Mutex
	allowed := map[string]struct{}{}
	refreshAt := time.Time{}
	return func(c *gin.Context) {
		// Preserve the existing unrestricted host behavior when no panel-wide
		// sub-domain is configured; reseller custom domains are additive there.
		if strings.TrimSpace(domain) == "" {
			c.Next()
			return
		}
		host := requestHost(c.Request.Host)
		now := time.Now()
		mu.Lock()
		if now.After(refreshAt) {
			fallback := map[string]struct{}{normalizeRequestDomain(domain): {}}
			var domains []string
			if err := database.GetDB().Model(&model.Reseller{}).Where("sub_domain <> ''").Pluck("sub_domain", &domains).Error; err == nil {
				next := make(map[string]struct{}, len(domains)+1)
				next[normalizeRequestDomain(domain)] = struct{}{}
				for _, d := range domains {
					if d = normalizeRequestDomain(d); d != "" {
						next[d] = struct{}{}
					}
				}
				allowed = next
			} else {
				// Fail closed to the already-configured panel host if the
				// reseller domain lookup is temporarily unavailable.
				allowed = fallback
			}
			refreshAt = now.Add(15 * time.Second)
		}
		_, ok := allowed[host]
		restricted := len(allowed) > 0
		mu.Unlock()
		if restricted && !ok {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func requestHost(hostPort string) string {
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		return normalizeRequestDomain(h)
	}
	return normalizeRequestDomain(hostPort)
}

func normalizeRequestDomain(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
