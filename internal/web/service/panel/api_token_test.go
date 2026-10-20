package panel

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func initApiTokenTestDB(t *testing.T) ApiTokenService {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	return ApiTokenService{}
}

// Scope normalization: anything unknown or empty behaves as full access so
// legacy rows keep their original power.
func TestNormalizeTokenScope(t *testing.T) {
	cases := map[string]string{
		"":       ApiTokenScopeFull,
		"full":   ApiTokenScopeFull,
		"read":   ApiTokenScopeRead,
		"READ":   ApiTokenScopeRead,
		" read ": ApiTokenScopeRead,
		"write":  ApiTokenScopeFull,
	}
	for in, want := range cases {
		if got := NormalizeTokenScope(in); got != want {
			t.Fatalf("NormalizeTokenScope(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsReadOnlyAPIRequest(t *testing.T) {
	for _, m := range []string{"GET", "HEAD", "OPTIONS"} {
		if !IsReadOnlyAPIRequest(m) {
			t.Fatalf("%s must be read-only", m)
		}
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if IsReadOnlyAPIRequest(m) {
			t.Fatalf("%s must be mutating", m)
		}
	}
}

// Create shows the plaintext exactly once and stores only its hash plus an
// identifying prefix.
func TestApiToken_CreateShowsOnceAndStoresHash(t *testing.T) {
	svc := initApiTokenTestDB(t)

	view, err := svc.CreateWithOptions("main", ApiTokenScopeRead, 30)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Token == "" {
		t.Fatal("creation must return the plaintext token")
	}
	if view.Prefix != view.Token[:8] {
		t.Fatalf("prefix = %q, want first 8 chars of %q", view.Prefix, view.Token)
	}
	if view.Scope != ApiTokenScopeRead {
		t.Fatalf("scope = %q, want read", view.Scope)
	}
	if view.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatal("30-day token must expire in the future")
	}

	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = (%d, %v)", len(list), err)
	}
	if list[0].Token != "" {
		t.Fatal("List must never expose the token value")
	}

	// Duplicate names refused.
	if _, err := svc.Create("main"); err == nil {
		t.Fatal("duplicate name must fail")
	}
}

// Disabled and expired tokens never authenticate.
func TestApiToken_AuthenticateLifecycle(t *testing.T) {
	svc := initApiTokenTestDB(t)

	view, err := svc.Create("life")
	if err != nil {
		t.Fatal(err)
	}
	if tok, ok := svc.Authenticate(view.Token); !ok || tok.Name != "life" {
		t.Fatalf("fresh token must authenticate, got (%v, %v)", tok, ok)
	}

	// Disable → no match.
	if err := svc.SetEnabled(view.Id, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Authenticate(view.Token); ok {
		t.Fatal("disabled token must not authenticate")
	}
	if err := svc.SetEnabled(view.Id, true); err != nil {
		t.Fatal(err)
	}

	// Force-expire → no match.
	db := database.GetDB()
	if err := db.Model(model.ApiToken{}).Where("id = ?", view.Id).Update("expires_at", time.Now().UnixMilli()-1000).Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Authenticate(view.Token); ok {
		t.Fatal("expired token must not authenticate")
	}

	// Unknown / empty never match.
	if _, ok := svc.Authenticate("nope"); ok {
		t.Fatal("unknown token must not authenticate")
	}
	if _, ok := svc.Authenticate(""); ok {
		t.Fatal("empty token must not authenticate")
	}
}

// Last-used is recorded on successful authentication (the service throttles
// writes, so the very first use always lands).
func TestApiToken_RecordsLastUsed(t *testing.T) {
	svc := initApiTokenTestDB(t)

	view, err := svc.Create("used")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Authenticate(view.Token); !ok {
		t.Fatal("authenticate failed")
	}
	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = (%d, %v)", len(list), err)
	}
	if list[0].LastUsedAt == 0 {
		t.Fatal("lastUsedAt must be recorded on successful authentication")
	}
}
