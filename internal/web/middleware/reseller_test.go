package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestIsResellerAllowed pins the reseller access boundary: only the four
// self-service namespaces may be reached with a reseller session, and the
// panel-wide client-group endpoints stay admin-only even though they live under
// an allowed prefix.
func TestIsResellerAllowed(t *testing.T) {
	allowed := []string{
		"/panel/api/inbounds/",
		"/panel/api/inbounds/list",
		"/panel/api/inbounds/get/1",
		"/panel/api/inbounds/add",
		"/panel/api/clients/",
		"/panel/api/clients/list",
		"/panel/api/clients/add",
		"/panel/api/clients/traffic/someone@example.com",
		"/panel/api/reseller/", // singular: the reseller's own endpoints
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
		"/panel/api/resellers/assignInbound",
		"/panel/api/clients/groups",         // exact match: panel-wide grouping
		"/panel/api/clients/groups/bulkAdd", // sub-path of the denied namespace
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

// TestRequestPathWithoutBase makes sure the guard compares against the API path
// even when the panel is mounted under a custom base path.
func TestRequestPathWithoutBase(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		basePath string
		request  string
		want     string
	}{
		{"", "/panel/api/inbounds/list", "/panel/api/inbounds/list"},
		{"/", "/panel/api/inbounds/list", "/panel/api/inbounds/list"},
		{"/base", "/base/panel/api/inbounds/list", "/panel/api/inbounds/list"},
		{"/base/", "/base/panel/api/clients/list", "/panel/api/clients/list"},
		{"/base", "/panel/api/inbounds/list", "/panel/api/inbounds/list"},
	}

	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", tc.request, nil)
		c.Set("base_path", tc.basePath)
		if got := requestPathWithoutBase(c); got != tc.want {
			t.Errorf("requestPathWithoutBase(base=%q, req=%q) = %q, want %q", tc.basePath, tc.request, got, tc.want)
		}
	}
}
