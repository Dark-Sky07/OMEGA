package service

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

func initBotNotifyTestDB(t *testing.T) BotNotifyStateService {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	return BotNotifyStateService{}
}

// A warning is claimed exactly once per (email, kind) episode: the first
// caller may send, every later caller is silent until the episode resets.
func TestBotNotifyState_ClaimOncePerEpisode(t *testing.T) {
	svc := initBotNotifyTestDB(t)

	if !svc.ClaimWarning("user@example.com", BotNotifyKindTraffic) {
		t.Fatal("first traffic claim must succeed")
	}
	if svc.ClaimWarning("user@example.com", BotNotifyKindTraffic) {
		t.Fatal("second traffic claim must be suppressed")
	}
	// The other kind is an independent episode.
	if !svc.ClaimWarning("user@example.com", BotNotifyKindExpire) {
		t.Fatal("first expire claim must succeed")
	}
	if svc.ClaimWarning("user@example.com", BotNotifyKindExpire) {
		t.Fatal("second expire claim must be suppressed")
	}

	// Resetting one kind must not clear the other.
	svc.ResetWarning("user@example.com", BotNotifyKindTraffic)
	if svc.ClaimWarning("user@example.com", BotNotifyKindExpire) {
		t.Fatal("expire episode must survive a traffic reset")
	}
	if !svc.ClaimWarning("user@example.com", BotNotifyKindTraffic) {
		t.Fatal("traffic must be claimable again after reset")
	}

	// ResetForEmail clears every kind for the client only.
	svc.ResetForEmail("user@example.com")
	if !svc.ClaimWarning("user@example.com", BotNotifyKindExpire) {
		t.Fatal("expire must be claimable again after ResetForEmail")
	}
	// Another client is independent: its own episode is unaffected by resets.
	if !svc.ClaimWarning("other@example.com", BotNotifyKindExpire) {
		t.Fatal("another client's first claim must succeed")
	}
	svc.ResetForEmail("user@example.com")
	if svc.ClaimWarning("other@example.com", BotNotifyKindExpire) {
		t.Fatal("another client's claim must survive a foreign reset")
	}
}

// Email matching is normalized (case/whitespace) like every other
// client-email key in the panel.
func TestBotNotifyState_EmailNormalization(t *testing.T) {
	svc := initBotNotifyTestDB(t)

	if !svc.ClaimWarning("  USER@Example.COM ", BotNotifyKindTraffic) {
		t.Fatal("first claim must succeed")
	}
	if svc.ClaimWarning("user@example.com", BotNotifyKindTraffic) {
		t.Fatal("same email in a different shape must be suppressed")
	}
	svc.ResetWarning("user@example.com", BotNotifyKindTraffic)
	if !svc.ClaimWarning("USER@example.com", BotNotifyKindTraffic) {
		t.Fatal("reset must clear the normalized key")
	}
}

// Concurrent report runs must never double-send: exactly one claimer wins.
func TestBotNotifyState_ConcurrentClaim(t *testing.T) {
	svc := initBotNotifyTestDB(t)

	const workers = 16
	var wg sync.WaitGroup
	results := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- svc.ClaimWarning("race@example.com", BotNotifyKindExpire)
		}()
	}
	wg.Wait()
	close(results)

	wins := 0
	for won := range results {
		if won {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one concurrent claim must win, got %d", wins)
	}
}
