package controller

import (
	"bytes"
	"embed"
	"encoding/json"
	htmlpkg "html"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Dark-Sky07/phoenix/internal/config"
	"github.com/Dark-Sky07/phoenix/internal/logger"
	"github.com/Dark-Sky07/phoenix/internal/web/session"
)

var distFS embed.FS

func SetDistFS(fs embed.FS) {
	distFS = fs
}

var distPageBuildTime = time.Now()

// ServeOpenAPISpec returns the generated OpenAPI 3.0 description of the
// panel API. Postman / Insomnia / openapi-generator consume this URL
// directly; the in-panel Swagger UI page also fetches it. The spec is
// produced at frontend build time by scripts/build-openapi.mjs and
// embedded into the binary via the dist FS.
func ServeOpenAPISpec(c *gin.Context) {
	body, err := distFS.ReadFile("dist/openapi.json")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "msg": "openapi.json not found"})
		return
	}

	// The embedded spec ships with `servers: [{url: "/"}]`. When the panel runs
	// under a non-root web base path, Swagger UI "Try it out" and external
	// generators must target that prefix, so rewrite the single server entry to
	// the runtime base path before serving.
	if basePath := c.GetString("base_path"); basePath != "" && basePath != "/" {
		if rebuilt, err := withServerBasePath(body, basePath); err != nil {
			logger.Warning("openapi.json: could not inject base path:", err)
		} else {
			body = rebuilt
		}
	}

	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// withServerBasePath rewrites the spec's `servers` entry so requests target the
// panel's configured web base path. Only the top-level `servers` field is
// replaced; every other field is preserved verbatim via json.RawMessage.
func withServerBasePath(spec []byte, basePath string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, err
	}
	servers, err := json.Marshal([]map[string]string{{
		"url":         strings.TrimSuffix(basePath, "/"),
		"description": "Current panel",
	}})
	if err != nil {
		return nil, err
	}
	doc["servers"] = servers
	return json.Marshal(doc)
}

func serveDistPage(c *gin.Context, name string) {
	body, err := distFS.ReadFile("dist/" + name)
	if err != nil {
		c.String(http.StatusInternalServerError, "missing embedded page: %s", name)
		return
	}

	basePath := c.GetString("base_path")
	if basePath == "" {
		basePath = "/"
	}

	if basePath != "/" {
		body = bytes.ReplaceAll(body, []byte(`src="/assets/`), []byte(`src="`+basePath+`assets/`))
		body = bytes.ReplaceAll(body, []byte(`href="/assets/`), []byte(`href="`+basePath+`assets/`))
		body = bytes.ReplaceAll(body, []byte(`href="/phoenix-icon.png"`), []byte(`href="`+basePath+`phoenix-icon.png"`))
	}

	jsEscape := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"<", `<`,
		">", `>`,
		"&", `&`,
	)
	escapedBase := jsEscape.Replace(basePath)
	csrfToken, err := session.EnsureCSRFToken(c)
	if err != nil {
		logger.Warning("Unable to mint CSRF token for", name+":", err)
		csrfToken = ""
	}
	csrfMeta := []byte(`<meta name="csrf-token" content="` + htmlpkg.EscapeString(csrfToken) + `">`)
	basePathMeta := []byte(`<meta name="base-path" content="` + htmlpkg.EscapeString(basePath) + `">`)

	nonceAttr := ""
	if nonce := c.GetString("csp_nonce"); nonce != "" {
		nonceAttr = ` nonce="` + htmlpkg.EscapeString(nonce) + `"`
	}
	script := `<script` + nonceAttr + `>window.X_UI_BASE_PATH="` + escapedBase + `"`
	if name != "login.html" {
		escapedVer := jsEscape.Replace(config.GetVersion())
		script += `;window.X_UI_CUR_VER="` + escapedVer + `"`
		script += `;window.X_UI_DB_TYPE="` + config.GetDBKind() + `"`
	}
	script += `;</script>`
	manifestMeta := []byte(`<link rel="manifest" href="` + htmlpkg.EscapeString(basePath) + `manifest.webmanifest"><meta name="theme-color" content="#ff5500">`)
	inject := []byte(script)
	inject = append(inject, csrfMeta...)
	inject = append(inject, basePathMeta...)
	inject = append(inject, manifestMeta...)
	inject = append(inject, []byte(`</head>`)...)
	out := bytes.Replace(body, []byte("</head>"), inject, 1)

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Last-Modified", distPageBuildTime.UTC().Format(http.TimeFormat))
	c.Data(http.StatusOK, "text/html; charset=utf-8", out)
}

const pwaIconDataURI = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 512 512'%3E%3Crect width='512' height='512' rx='112' fill='%230b0708'/%3E%3Cpath d='M256 76c-32 48-68 78-68 128 0 28 14 54 36 70-48-12-94-48-116-102-12 66 20 136 82 168-30 6-62 2-88-14 26 48 80 78 138 78v32l16 20 16-20v-32c58 0 112-30 138-78-26 16-58 20-88 14 62-32 94-102 82-168-22 54-68 90-116 102 22-16 36-42 36-70 0-50-36-80-68-128z' fill='%23ff6a00'/%3E%3C/svg%3E"

const pwaServiceWorkerJS = `// PHOENIX Safe Network-First Service Worker
// Caches immutable /assets/* bundles for fast startup while NEVER caching
// /panel/api/*, authentication routes, or live subscription payloads.
const CACHE_NAME = 'phoenix-static-v1';

self.addEventListener('install', (event) => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (!url.pathname.includes('/assets/')) return;

  event.respondWith(
    fetch(req)
      .then((res) => {
        if (res && res.status === 200) {
          const clone = res.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(req, clone)).catch(() => {});
        }
        return res;
      })
      .catch(() => caches.match(req))
  );
});
`

// ServePWAManifest serves a dynamic Web App Manifest scoped to the active base_path.
func ServePWAManifest(c *gin.Context) {
	basePath := c.GetString("base_path")
	if basePath == "" {
		basePath = "/"
	}
	manifest := map[string]any{
		"name":             "PHOENIX Panel",
		"short_name":       "PHOENIX",
		"description":      "PHOENIX — Reborn from Fire & Ash • Unified Multi-Protocol Proxy & VPN Management Panel",
		"start_url":        basePath,
		"scope":            basePath,
		"display":          "standalone",
		"background_color": "#0b0708",
		"theme_color":      "#ff5500",
		"icons": []map[string]string{
			{
				"src":     basePath + "phoenix-icon.png",
				"sizes":   "192x192",
				"type":    "image/png",
				"purpose": "any maskable",
			},
			{
				"src":     pwaIconDataURI,
				"sizes":   "512x512",
				"type":    "image/svg+xml",
				"purpose": "any maskable",
			},
		},
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/manifest+json; charset=utf-8", func() []byte {
		b, _ := json.Marshal(manifest)
		return b
	}())
}

// ServePWAServiceWorker serves the safe network-first service worker script.
func ServePWAServiceWorker(c *gin.Context) {
	c.Header("Service-Worker-Allowed", "/")
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(pwaServiceWorkerJS))
}
