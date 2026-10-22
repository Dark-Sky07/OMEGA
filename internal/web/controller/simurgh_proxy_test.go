package controller

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimurghProxyKeepsServicePrivateAndRewritesPaths(t *testing.T) {
	const user, password = "panel-user", "panel-secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" {
			t.Errorf("upstream path = %q, want /api/status", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+password)) {
			t.Errorf("upstream Authorization = %q", got)
		}
		for _, key := range []string{"Cookie", simurghPrefixHeader, simurghCSRFHeader, simurghNonceHeader} {
			if got := r.Header.Get(key); got != "" {
				t.Errorf("private upstream received %s", key)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	stateFile := filepath.Join(t.TempDir(), "state.json")
	state := `{"panel_auth":{"user":"` + user + `","password":"` + password + `"}}`
	if err := os.WriteFile(stateFile, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	proxy := newSimurghProxy(upstream.URL)
	proxy.stateFile = stateFile

	req := httptest.NewRequest(http.MethodGet, "/phoenix/panel/simurgh/api/status", nil)
	req.AddCookie(&http.Cookie{Name: "phoenix", Value: "must-not-forward"})
	rec := httptest.NewRecorder()
	proxy.serve(rec, req, "/phoenix/panel/simurgh", "csrf-token", "csp-nonce")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Fatalf("proxy response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSimurghHTMLAndScriptStayUnderPanelPrefix(t *testing.T) {
	const prefix = "/custom/panel/simurgh"
	html := `<html><head><link href="/app.css"><script src="/app.js"></script></head></html>`
	gotHTML := rewriteSimurghHTML(html, prefix, "csrf-value", "nonce-value")
	for _, want := range []string{prefix + `/app.css`, prefix + `/app.js`, `window.__PHOENIX_CSRF__="csrf-value"`, `nonce="nonce-value"`} {
		if !strings.Contains(gotHTML, want) {
			t.Errorf("rewritten HTML does not contain %q: %s", want, gotHTML)
		}
	}

	js := `const defaults = { headers: { 'Content-Type': 'application/json' } }; fetch('/api/status'); fetch('/api/mappings'); location.href = '/';`
	gotJS := rewriteSimurghJS(js, prefix)
	for _, want := range []string{prefix + `/api/status`, prefix + `/api/mappings`, prefix + `/`} {
		if !strings.Contains(gotJS, want) {
			t.Errorf("rewritten script does not contain %q: %s", want, gotJS)
		}
	}
	if !strings.Contains(gotJS, `X-CSRF-Token`) {
		t.Errorf("script API helper is missing the PHOENIX CSRF token: %s", gotJS)
	}
}

func TestSimurghManifestRewrite(t *testing.T) {
	input := `{"icons":[{"src":"/assets/simurgh.png"}], "start_url": "/", "scope": "/"}`
	got := rewriteSimurghManifest(input, "/panel/simurgh")
	if !strings.Contains(got, `"src":"/panel/simurgh/assets/simurgh.png"`) {
		t.Fatalf("manifest asset path was not rewritten: %s", got)
	}
	var manifest struct {
		StartURL string `json:"start_url"`
		Scope    string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(got), &manifest); err != nil {
		t.Fatalf("rewritten manifest is invalid JSON: %v", err)
	}
	if manifest.StartURL != "/panel/simurgh/" || manifest.Scope != "/panel/simurgh/" {
		t.Fatalf("manifest paths = (%q, %q), want the panel prefix", manifest.StartURL, manifest.Scope)
	}
}
