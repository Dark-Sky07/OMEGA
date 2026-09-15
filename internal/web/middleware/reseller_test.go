package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// TestIsResellerAllowed pins the reseller access boundary: only the four
// self-service namespaces may be reached with a reseller session, and the
// panel-wide client-group endpoints stay admin-only even though they live under
// an allowed prefix.
func TestIsResellerAllowed(t *testing.T) {
	allowed := []string{
		"/panel/api/inbounds/list",
		"/panel/api/inbounds/get/1",
		"/panel/api/inbounds/add",
		"/panel/api/clients/list",
		"/panel/api/clients/add",
		"/panel/api/clients/traffic/someone@example.com",
		"/panel/api/reseller/profile",
		"/panel/api/reseller/report",
		"/panel/api/reseller/password",
		"/panel/api/auth/me",
	}
	for _, path := range allowed {
		if !isResellerAllowed(path) {
			t.Errorf("isResellerAllowed(%q) = false, want true", path)
		}
	}

	denied := []string{
		"/panel/api/resellers/list", // managing other resellers
		"/panel/api/resellers/add",
		"/panel/api/clients/groups",         // panel-wide grouping
		"/panel/api/clients/groups",         // exact match
		"/panel/api/clients/groups/bulkAdd", // sub-path of a denied namespace
		"/panel/api/setting/all",
		"/panel/api/xray/",
		"/panel/api/nodes/list",
		"/panel/api/server/status",
		"/panel/setting/all",
		"/",
		"",
	}
	for _, path := range denied {
		if isResellerAllowed(path) {
			t.Errorf("isResellerAllowed(%q) = true, want false", path)
		}
	}
}

// TestResellerGuardBlocksAdminNamespaces drives the guard through a real gin
// router: an admin session passes, a reseller session is cut off with a 403 and
// the documented message, and the base path prefix is stripped before matching.
func TestResellerGuardBlocksAdminNamespaces(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newRouter := func() (*gin.Engine, cookie.Store) {
		router := gin.New()
		store := cookie.NewStore([]byte("01234567890123456789012345678901"))
		router.Use(sessions.Sessions("3x-ui", store))
		router.Use(func(c *gin.Context) {
			c.Set("base_path", "/base/")
			c.Next()
		})
		router.Use(ResellerGuard())
		return router, store
	}

	// Admin sessions are untouched, whatever the path.
	adminRouter, _ := newRouter()
	adminRouter.GET("/panel/api/setting/all", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	rec := httptest.NewRecorder()
	adminRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panel/api/setting/all", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin request status = %d, want %d", rec.Code, http.StatusOK)
	}

	// A reseller session is refused, with the base path stripped first.
	resellerRouter, store := newRouter()
	resellerRouter.GET("/panel/api/setting/all", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	resellerRouter.GET("/panel/api/inbounds/list", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	loginAsReseller := func(req *http.Request) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		sessions.Sessions("3x-ui", store)(c)
		c.Next()
		if err := session.SetLoginReseller(c, 1, false); err != nil {
			t.Fatalf("SetLoginReseller: %v", err)
		}
	}

	blockedReq := httptest.NewRequest(http.MethodGet, "/base/panel/api/setting/all", nil)
	loginAsReseller(blockedReq)
	rec = httptest.NewRecorder()
	resellerRouter.ServeHTTP(rec, blockedReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reseller admin-path status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	var body struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Success || body.Msg != "this section is not available for reseller accounts" {
		t.Errorf("body = %+v", body)
	}

	allowedReq := httptest.NewRequest(http.MethodGet, "/base/panel/api/inbounds/list", nil)
	loginAsReseller(allowedReq)
	rec = httptest.NewRecorder()
	resellerRouter.ServeHTTP(rec, allowedReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("reseller self-service path status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}
