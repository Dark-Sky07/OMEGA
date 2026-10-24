package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	simurghDefaultAddress = "http://127.0.0.1:18787"
	simurghPrefixHeader   = "X-Phoenix-Simurgh-Prefix"
	simurghCSRFHeader     = "X-Phoenix-CSRF"
	simurghNonceHeader    = "X-Phoenix-CSP-Nonce"
)

// simurghProxy exposes the independent Simurgh panel only through PHOENIX's
// authenticated panel session. The service itself remains loopback-only.
type simurghProxy struct {
	proxy         *httputil.ReverseProxy
	stateFile     string
	updaterPath   string
	dockerMode    func() bool
	updateMu      sync.Mutex
	updateRunning bool
}

type simurghUpdateResponse struct {
	OK              bool   `json:"ok"`
	Error           string `json:"error,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Version         string `json:"version,omitempty"`
	Latest          string `json:"latest,omitempty"`
	Tag             string `json:"tag,omitempty"`
	NotesURL        string `json:"notes_url,omitempty"`
	Action          string `json:"action,omitempty"`
	Message         string `json:"message,omitempty"`
	RestartRequired bool   `json:"restart_required"`
	Restarting      bool   `json:"restarting"`
}

type simurghCredentials struct {
	PanelAuth struct {
		User     string `json:"user"`
		Password string `json:"password"`
	} `json:"panel_auth"`
}

func newSimurghProxy(target string) *simurghProxy {
	if target == "" {
		target = simurghDefaultAddress
	}
	targetURL, err := url.Parse(target)
	if err != nil || targetURL.Scheme != "http" || targetURL.Host == "" {
		targetURL, _ = url.Parse(simurghDefaultAddress)
	}
	stateFile := os.Getenv("PHOENIX_SIMURGH_STATE_FILE")
	if stateFile == "" {
		stateFile = "/etc/phoenix-simurgh/state.json"
	}
	updaterPath := os.Getenv("PHOENIX_SIMURGH_UPDATER")
	if updaterPath == "" {
		updaterPath = "/usr/bin/phoenix-simurgh-update"
	}
	s := &simurghProxy{
		stateFile:   stateFile,
		updaterPath: updaterPath,
		dockerMode: func() bool {
			if strings.EqualFold(os.Getenv("XUI_IN_DOCKER"), "true") {
				return true
			}
			_, err := os.Stat("/.dockerenv")
			return err == nil
		},
	}
	s.proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			prefix := req.Header.Get(simurghPrefixHeader)
			path := strings.TrimPrefix(req.URL.Path, prefix)
			if path == "" {
				path = "/"
			}
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = path
			req.URL.RawPath = ""
			req.Host = targetURL.Host
			// Never forward PHOENIX's cookies or browser-supplied credentials to
			// the private service. It receives only its own credentials.
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			req.Header.Del("Accept-Encoding")
			req.Header.Del("If-None-Match")
			req.Header.Del("If-Modified-Since")
			if user, password := s.credentials(); user != "" && password != "" {
				req.SetBasicAuth(user, password)
			}
		},
		Transport: &simurghTransport{base: &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          8,
			IdleConnTimeout:       30 * time.Second,
			DisableCompression:    true,
			ResponseHeaderTimeout: 5 * time.Second,
		}},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Set-Cookie")
			if location := resp.Header.Get("Location"); strings.HasPrefix(location, "/") {
				resp.Header.Set("Location", resp.Request.Header.Get(simurghPrefixHeader)+location)
			}
			resp.Header.Del("Content-Security-Policy")
			ctype := strings.ToLower(resp.Header.Get("Content-Type"))
			if !strings.Contains(ctype, "text/html") && !strings.Contains(ctype, "javascript") && !strings.Contains(ctype, "manifest+json") {
				return nil
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
			prefix := resp.Request.Header.Get(simurghPrefixHeader)
			text := string(body)
			switch {
			case strings.Contains(ctype, "text/html"):
				text = rewriteSimurghHTML(text, prefix, resp.Request.Header.Get(simurghCSRFHeader), resp.Request.Header.Get(simurghNonceHeader))
			case strings.Contains(ctype, "javascript"):
				text = rewriteSimurghJS(text, prefix)
			case strings.Contains(ctype, "manifest+json"):
				text = rewriteSimurghManifest(text, prefix)
			}
			resp.Body = io.NopCloser(strings.NewReader(text))
			resp.ContentLength = int64(len(text))
			resp.Header.Set("Content-Length", fmt.Sprint(len(text)))
			resp.Header.Set("Cache-Control", "no-store")
			if strings.Contains(ctype, "text/html") {
				nonce := resp.Request.Header.Get(simurghNonceHeader)
				resp.Header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'nonce-"+nonce+"'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "Simurgh service is not available", http.StatusServiceUnavailable)
		},
	}
	return s
}

// simurghTransport keeps request-scoped rewrite data available to the response
// handler but strips it before sending anything to the private service.
type simurghTransport struct{ base http.RoundTripper }

func (t *simurghTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	outbound := req.Clone(req.Context())
	outbound.Header = req.Header.Clone()
	outbound.Header.Del(simurghPrefixHeader)
	outbound.Header.Del(simurghCSRFHeader)
	outbound.Header.Del(simurghNonceHeader)
	resp, err := t.base.RoundTrip(outbound)
	if resp != nil {
		resp.Request = req
	}
	return resp, err
}

func (s *simurghProxy) credentials() (string, string) {
	data, err := os.ReadFile(filepath.Clean(s.stateFile))
	if err != nil {
		return "", ""
	}
	var credentials simurghCredentials
	if json.Unmarshal(data, &credentials) != nil {
		return "", ""
	}
	return credentials.PanelAuth.User, credentials.PanelAuth.Password
}

func (s *simurghProxy) serve(w http.ResponseWriter, r *http.Request, prefix, csrf, nonce string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimPrefix(r.URL.Path, prefix) == "/api/update" {
		s.serveUpdateAPI(w, r)
		return
	}
	r.Header.Set(simurghPrefixHeader, prefix)
	r.Header.Set(simurghCSRFHeader, csrf)
	r.Header.Set(simurghNonceHeader, nonce)
	w.Header().Del("X-Frame-Options")
	w.Header().Del("Content-Security-Policy")
	s.proxy.ServeHTTP(w, r)
}

func (s *simurghProxy) serveUpdateAPI(w http.ResponseWriter, r *http.Request) {
	writeJSON := func(status int, result simurghUpdateResponse) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	}
	fail := func(status int, message string) {
		writeJSON(status, simurghUpdateResponse{OK: false, Error: message})
	}

	if s.dockerMode != nil && s.dockerMode() {
		fail(http.StatusConflict, "Simurgh is managed by the Docker image. Pull and recreate PHOENIX to update it.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		fail(http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	result, err := s.runUpdater(ctx, "--check", "--json")
	if err != nil {
		message := err.Error()
		if result.Error != "" {
			message = result.Error
		}
		fail(http.StatusBadGateway, message)
		return
	}
	if r.Method == http.MethodGet || !result.UpdateAvailable {
		writeJSON(http.StatusOK, result)
		return
	}

	s.updateMu.Lock()
	if s.updateRunning {
		s.updateMu.Unlock()
		fail(http.StatusConflict, "a Simurgh update is already running")
		return
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		s.updateMu.Unlock()
		fail(http.StatusServiceUnavailable, "Python 3 is required to update Simurgh")
		return
	}
	if _, err := os.Stat(s.updaterPath); err != nil {
		s.updateMu.Unlock()
		fail(http.StatusServiceUnavailable, "the PHOENIX Simurgh updater is missing; update PHOENIX first")
		return
	}
	cmd := exec.Command(python, s.updaterPath, "--yes", "--json")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	s.updateRunning = true
	if err := cmd.Start(); err != nil {
		s.updateRunning = false
		s.updateMu.Unlock()
		fail(http.StatusInternalServerError, fmt.Sprintf("could not start the Simurgh updater: %v", err))
		return
	}
	s.updateMu.Unlock()

	go func() {
		err := cmd.Wait()
		s.updateMu.Lock()
		s.updateRunning = false
		s.updateMu.Unlock()
		if err != nil {
			log.Printf("Simurgh panel update failed: %v", err)
		}
	}()

	result.Action = "update started"
	result.RestartRequired = true
	result.Restarting = true
	writeJSON(http.StatusOK, result)
}

func (s *simurghProxy) runUpdater(ctx context.Context, args ...string) (simurghUpdateResponse, error) {
	var result simurghUpdateResponse
	python, err := exec.LookPath("python3")
	if err != nil {
		return result, fmt.Errorf("Python 3 is required to check Simurgh updates")
	}
	if _, err := os.Stat(s.updaterPath); err != nil {
		return result, fmt.Errorf("the PHOENIX Simurgh updater is missing; update PHOENIX first")
	}
	cmdArgs := append([]string{s.updaterPath}, args...)
	output, err := exec.CommandContext(ctx, python, cmdArgs...).CombinedOutput()
	if len(output) > 64*1024 {
		return result, fmt.Errorf("the Simurgh updater returned an unexpectedly large response")
	}
	if decodeErr := json.Unmarshal(output, &result); decodeErr != nil {
		if err != nil {
			return result, fmt.Errorf("the Simurgh updater failed: %s", strings.TrimSpace(string(output)))
		}
		return result, fmt.Errorf("the Simurgh updater returned invalid JSON: %w", decodeErr)
	}
	if err != nil {
		if result.Error != "" {
			return result, fmt.Errorf("%s", result.Error)
		}
		return result, fmt.Errorf("the Simurgh updater failed: %w", err)
	}
	if !result.OK {
		return result, fmt.Errorf("%s", result.Error)
	}
	return result, nil
}

var simurghRootPath = regexp.MustCompile(`(['"])(/(?:api|assets|app\.css|app\.js|login\.js|manifest\.webmanifest|favicon\.ico|logout|login|metrics|healthz|ws)(?:[^'"]*)?)`)

func rewriteSimurghHTML(body, prefix, csrf, nonce string) string {
	for _, attribute := range []string{`href="/`, `src="/`, `action="/`} {
		body = strings.ReplaceAll(body, attribute, attribute[:len(attribute)-1]+prefix+`/`)
	}
	bootstrap, _ := json.Marshal(csrf)
	injected := `<script nonce="` + nonce + `">window.__PHOENIX_CSRF__=` + string(bootstrap) + `;</script>`
	body = strings.Replace(body, `</head>`, injected+`</head>`, 1)
	return body
}

func rewriteSimurghJS(body, prefix string) string {
	body = simurghRootPath.ReplaceAllString(body, `${1}`+prefix+`${2}`)
	body = strings.ReplaceAll(body, `headers: { 'Content-Type': 'application/json' }`, `headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': window.__PHOENIX_CSRF__ || '' }`)
	body = strings.ReplaceAll(body, "attempt < 40", "attempt < 180")
	body = strings.ReplaceAll(body, `location.href = '/';`, `location.href = '`+prefix+`/';`)
	return body
}

var simurghManifestRootPath = regexp.MustCompile(`("(?:start_url|scope)"\s*:\s*")/`)

func rewriteSimurghManifest(body, prefix string) string {
	for _, path := range []string{"/assets/", "/favicon.ico"} {
		body = strings.ReplaceAll(body, `"`+path, `"`+prefix+path)
	}
	return simurghManifestRootPath.ReplaceAllString(body, `${1}`+prefix+`/`)
}
