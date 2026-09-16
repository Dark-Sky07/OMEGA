package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// resellerWriteContract is the wire shape every reseller rejection answers
// with: HTTP 403 and a body whose success flag is false with a human message.
type resellerWriteContract struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
}

// setupResellerInboundRouter builds a throwaway panel: a temp-dir database
// with one reseller row, a gin engine serving the real inbound routes, and a
// middleware that injects that reseller's login into every request session
// (the same keys SetLoginReseller writes, minus the login round-trip).
func setupResellerInboundRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	reseller := model.Reseller{Username: "sara", Password: "x", Name: "Sara", Enable: true, LoginEpoch: 1}
	if err := database.GetDB().Create(&reseller).Error; err != nil {
		t.Fatalf("seed reseller: %v", err)
	}

	r := gin.New()
	store := cookie.NewStore([]byte("reseller-contract-test"))
	r.Use(sessions.Sessions("3x-ui", store))
	r.Use(func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("LOGIN_RESELLER", reseller.Id)
		s.Set("LOGIN_RESELLER_EPOCH", reseller.LoginEpoch)
		c.Next()
	})
	NewInboundController(r.Group("/panel/api/inbounds"))
	return r
}

func doResellerInboundRequest(t *testing.T, r *gin.Engine, method, path string) (int, resellerWriteContract) {
	t.Helper()
	req := httptest.NewRequest(method, "/panel/api/inbounds"+path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body resellerWriteContract
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s %s: response is not JSON: %v (%q)", method, path, err, w.Body.String())
	}
	return w.Code, body
}

// TestResellerInboundWritesReturn403Contract pins the read-only boundary:
// every inbound POST endpoint rejects reseller sessions before parsing a
// single parameter, with HTTP 403 and {success:false, msg}.
func TestResellerInboundWritesReturn403Contract(t *testing.T) {
	r := setupResellerInboundRouter(t)
	paths := []string{
		"/add",
		"/del/1",
		"/bulkDel",
		"/update/1",
		"/setEnable/1",
		"/1/resetTraffic",
		"/1/delAllClients",
		"/resetAllTraffics",
		"/import",
		"/1/fallbacks",
		"/pushClientTraffics",
	}
	for _, path := range paths {
		code, body := doResellerInboundRequest(t, r, http.MethodPost, path)
		if code != http.StatusForbidden {
			t.Errorf("POST %s: status = %d, want 403", path, code)
		}
		if body.Success {
			t.Errorf("POST %s: success = true, want false", path)
		}
		if body.Msg != errResellerInboundReadOnly {
			t.Errorf("POST %s: msg = %q, want %q", path, body.Msg, errResellerInboundReadOnly)
		}
	}
}

// TestResellerInboundReadsStayScoped pins the other half of the boundary:
// listing still works for resellers, while an inbound they do not own fails
// closed as "missing" instead of leaking its existence.
func TestResellerInboundReadsStayScoped(t *testing.T) {
	r := setupResellerInboundRouter(t)

	code, body := doResellerInboundRequest(t, r, http.MethodGet, "/list")
	if code != http.StatusOK || !body.Success {
		t.Fatalf("GET /list: code = %d success = %v, want 200/true", code, body.Success)
	}

	inbound := model.Inbound{UserId: 1, Port: 12001, Protocol: model.VLESS, Settings: "{}", Tag: "unowned"}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	path := fmt.Sprintf("/get/%d", inbound.Id)
	code, body = doResellerInboundRequest(t, r, http.MethodGet, path)
	if code != http.StatusForbidden {
		t.Errorf("GET %s: status = %d, want 403", path, code)
	}
	if body.Success || body.Msg != errNotYourInbound {
		t.Errorf("GET %s: success = %v msg = %q, want false/%q", path, body.Success, body.Msg, errNotYourInbound)
	}
}
