package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServePWAManifestAndServiceWorker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("base_path", "/secret-panel/")
		c.Next()
	})
	r.GET("/manifest.webmanifest", ServePWAManifest)
	r.GET("/sw.js", ServePWAServiceWorker)

	req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for manifest, got %d", rec.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}
	if doc["start_url"] != "/secret-panel/" || doc["scope"] != "/secret-panel/" {
		t.Fatalf("unexpected manifest start_url/scope: %+v", doc)
	}

	reqSw := httptest.NewRequest(http.MethodGet, "/sw.js", nil)
	recSw := httptest.NewRecorder()
	r.ServeHTTP(recSw, reqSw)
	if recSw.Code != http.StatusOK {
		t.Fatalf("expected 200 for sw.js, got %d", recSw.Code)
	}
	if recSw.Header().Get("Service-Worker-Allowed") != "/" {
		t.Fatalf("expected Service-Worker-Allowed=/ header")
	}
	if !strings.Contains(recSw.Body.String(), "if (!url.pathname.includes('/assets/')) return;") {
		t.Fatalf("service worker should only cache /assets/ paths")
	}
}
