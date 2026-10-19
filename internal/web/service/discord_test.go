package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

func initDiscordTestDB(t *testing.T) DiscordService {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	return DiscordService{}
}

func TestDiscordService_WebhookAndBotDelivery(t *testing.T) {
	svc := initDiscordTestDB(t)

	var webhookHits atomic.Int32
	var botHits atomic.Int32
	var lastAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("bad json: %v", err)
		}
		if r.URL.Path == "/webhook" {
			webhookHits.Add(1)
		} else if r.URL.Path == "/channels/12345/messages" {
			botHits.Add(1)
			lastAuth = r.Header.Get("Authorization")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc.APIBaseURL = srv.URL
	_ = svc.settingService.saveSetting("discordEnable", "true")
	_ = svc.settingService.saveSetting("discordWebhookURL", srv.URL+"/webhook")
	_ = svc.settingService.saveSetting("discordBotToken", "secret-bot-tok")
	_ = svc.settingService.saveSetting("discordChannelId", "12345")

	if !svc.IsEnabled() {
		t.Fatal("IsEnabled must be true when enabled and configured")
	}
	if err := svc.SendTestMessage(); err != nil {
		t.Fatalf("SendTestMessage: %v", err)
	}
	if webhookHits.Load() != 1 || botHits.Load() != 1 {
		t.Fatalf("hits = (webhook=%d, bot=%d), want (1, 1)", webhookHits.Load(), botHits.Load())
	}
	if lastAuth != "Bot secret-bot-tok" {
		t.Fatalf("Authorization = %q, want Bot secret-bot-tok", lastAuth)
	}

	svc.SendLoginNotify("admin", "1.2.3.4", "2026-10-18", "", true)
	if webhookHits.Load() != 2 {
		t.Fatalf("login notify did not post to webhook (hits=%d)", webhookHits.Load())
	}

	svc.SendPeriodicReport()
	if webhookHits.Load() != 3 {
		t.Fatalf("periodic report did not post to webhook (hits=%d)", webhookHits.Load())
	}
}
