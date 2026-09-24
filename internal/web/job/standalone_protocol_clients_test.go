package job

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func setupStandaloneJobDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func seedStandaloneClient(t *testing.T, protocol model.Protocol) *model.Inbound {
	t.Helper()
	db := database.GetDB()
	settings := `{"clients":[{"email":"Alice@Example.test","enable":true,"password":"secret"}]}`
	if protocol == model.OpenVPN {
		settings = `{"proto":"udp","psk":"unused","clients":[{"email":"Alice@Example.test","enable":true,"password":"secret"}]}`
	}
	ib := &model.Inbound{
		Tag:      "daemon-" + string(protocol),
		Enable:   true,
		Protocol: protocol,
		Port:     45000,
		Settings: settings,
	}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client := model.Client{
		Email:    "Alice@Example.test",
		ID:       "55555555-5555-5555-5555-555555555555",
		SubID:    "daemon-client",
		Enable:   true,
		Password: "secret",
	}
	if err := (&service.ClientService{}).SyncInbound(nil, ib.Id, []model.Client{client}); err != nil {
		t.Fatalf("SyncInbound: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{
		InboundId: ib.Id,
		Email:     " alice@example.test ",
		Enable:    false,
		Total:     1000,
	}).Error; err != nil {
		t.Fatalf("create disabled traffic: %v", err)
	}
	ib.ClientStats = []xray.ClientTraffic{{Email: " ALICE@EXAMPLE.TEST ", Enable: false}}
	return ib
}

func TestOpenVPNEnabledClientsUseCanonicalEmailKeyAndQuotaFlag(t *testing.T) {
	setupStandaloneJobDB(t)
	ib := seedStandaloneClient(t, model.OpenVPN)
	clients, ok := (&OpenvpnJob{}).enabledClientsForInbound(ib)
	if !ok {
		t.Fatal("enabled-client lookup failed")
	}
	if len(clients) != 0 {
		t.Fatalf("quota-disabled OpenVPN client was selected: %v", clients)
	}

	// A distinct case/whitespace representation must still select the client
	// once the traffic row is reset and enabled.
	if err := database.GetDB().Model(&xray.ClientTraffic{}).
		Where("LOWER(TRIM(email)) = LOWER(?)", "alice@example.test").
		Update("enable", true).Error; err != nil {
		t.Fatalf("reenable traffic row: %v", err)
	}
	clients, ok = (&OpenvpnJob{}).enabledClientsForInbound(ib)
	if !ok || len(clients) != 1 || clients[0] != "Alice@Example.test" {
		t.Fatalf("enabled OpenVPN client was not selected canonically: %#v", clients)
	}
}

func TestL2TPEnabledClientsUseCanonicalEmailKeyAndQuotaFlag(t *testing.T) {
	setupStandaloneJobDB(t)
	ib := seedStandaloneClient(t, model.L2TP)
	clients, ok := (&L2TPJob{}).enabledClientsForInbound(ib)
	if !ok {
		t.Fatal("enabled-client lookup failed")
	}
	if len(clients) != 0 {
		t.Fatalf("quota-disabled L2TP client was selected: %#v", clients)
	}

	if err := database.GetDB().Model(&xray.ClientTraffic{}).
		Where("LOWER(TRIM(email)) = LOWER(?)", "alice@example.test").
		Update("enable", true).Error; err != nil {
		t.Fatalf("reenable traffic row: %v", err)
	}
	clients, ok = (&L2TPJob{}).enabledClientsForInbound(ib)
	if !ok || len(clients) != 1 || clients[0].Email != "Alice@Example.test" {
		t.Fatalf("enabled L2TP client was not selected canonically: %#v", clients)
	}
}
