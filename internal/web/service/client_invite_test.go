package service

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/database"
)

func initClientInviteTestDB(t *testing.T) ClientInviteService {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	return ClientInviteService{}
}

// A generated invite is single-use: the first redemption wins, later attempts
// are refused as used.
func TestClientInvite_SingleUse(t *testing.T) {
	svc := initClientInviteTestDB(t)

	code, expiresAt, err := svc.CreateInvite("User@Example.com")
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if len(code) != 32 {
		t.Fatalf("code must be 32 hex chars, got %q", code)
	}
	if expiresAt <= 0 {
		t.Fatal("expiresAt must be in the future")
	}

	email, err := svc.RedeemInvite(code, 111)
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if email != "user@example.com" {
		t.Fatalf("email = %q, want normalized user@example.com", email)
	}

	if _, err := svc.RedeemInvite(code, 222); err != ErrInviteUsed {
		t.Fatalf("second redeem must fail with ErrInviteUsed, got %v", err)
	}
}

// Unknown and expired codes fail with the matching error.
func TestClientInvite_UnknownAndExpired(t *testing.T) {
	svc := initClientInviteTestDB(t)

	if _, err := svc.RedeemInvite("doesnotexist", 1); err != ErrInviteNotFound {
		t.Fatalf("unknown code must fail with ErrInviteNotFound, got %v", err)
	}

	code, _, err := svc.CreateInvite("x@example.com")
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	// Force-expire the row (unit-level: no clock injection needed).
	db := database.GetDB()
	if err := db.Exec("UPDATE client_invites SET expires_at = 1 WHERE code = ?", code).Error; err != nil {
		t.Fatalf("force expire: %v", err)
	}
	if _, err := svc.RedeemInvite(code, 1); err != ErrInviteExpired {
		t.Fatalf("expired code must fail with ErrInviteExpired, got %v", err)
	}
}

// RevokeInvitesForEmail invalidates unused invites of one client only.
func TestClientInvite_Revoke(t *testing.T) {
	svc := initClientInviteTestDB(t)

	codeA, _, err := svc.CreateInvite("a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	codeB, _, err := svc.CreateInvite("b@example.com")
	if err != nil {
		t.Fatal(err)
	}
	// Redeem one of A's invites, then revoke the rest.
	codeA2, _, err := svc.CreateInvite("a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RedeemInvite(codeA, 5); err != nil {
		t.Fatal(err)
	}
	n, err := svc.RevokeInvitesForEmail("a@example.com")
	if err != nil || n != 1 {
		t.Fatalf("revoke = (%d, %v), want exactly 1 unused invite revoked", n, err)
	}
	if _, err := svc.RedeemInvite(codeA2, 6); err != ErrInviteUsed {
		t.Fatalf("revoked invite must read as used, got %v", err)
	}
	// B's invite is untouched.
	if _, err := svc.RedeemInvite(codeB, 7); err != nil {
		t.Fatalf("other client's invite must survive: %v", err)
	}
}

// Concurrent redemption must yield exactly one winner.
func TestClientInvite_ConcurrentRedeem(t *testing.T) {
	svc := initClientInviteTestDB(t)

	code, _, err := svc.CreateInvite("race@example.com")
	if err != nil {
		t.Fatal(err)
	}

	const workers = 12
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, err := svc.RedeemInvite(code, id)
			results <- err
		}(int64(100 + i))
	}
	wg.Wait()
	close(results)

	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one concurrent redeem must win, got %d", wins)
	}
}
