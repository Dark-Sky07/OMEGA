package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/xray"
)

// A delayed-start client (negative expiryTime) whose activation deadline has
// passed must be converted to an expired client at exactly the deadline —
// the ordinary expiry machinery then disables it like any other expired
// client. Clients not yet at their deadline (or without one) stay untouched.
func TestSweepActivateBefore_ConvertsExpiredDeadlines(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	now := time.Now().UnixMilli()
	const msPerDay = int64(86400000)

	deadlinePassed := now - 60000  // 1 minute past the deadline
	deadlineAhead := now + 3600000 // 1 hour left

	inbound := &model.Inbound{Tag: "act-vless", Enable: true, Port: 51001, Protocol: model.VLESS,
		StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	settings, _ := json.Marshal(map[string]any{"clients": []map[string]any{
		{
			"email": "late@example.com", "id": "ce8d33df-3a64-4f10-8f9b-91c3a8e0c211",
			"enable": true, "expiryTime": -30 * msPerDay, "subId": "sub-act-1",
			"activateBefore": deadlinePassed,
		},
		{
			"email": "soon@example.com", "id": "ce8d33df-3a64-4f10-8f9b-91c3a8e0c212",
			"enable": true, "expiryTime": -30 * msPerDay, "subId": "sub-act-1",
			"activateBefore": deadlineAhead,
		},
		{
			"email": "nodeadline@example.com", "id": "ce8d33df-3a64-4f10-8f9b-91c3a8e0c213",
			"enable": true, "expiryTime": -30 * msPerDay, "subId": "sub-act-1",
		},
		{
			"email": "started@example.com", "id": "ce8d33df-3a64-4f10-8f9b-91c3a8e0c214",
			"enable": true, "expiryTime": now + 30*msPerDay, "subId": "sub-act-1",
			"activateBefore": deadlinePassed,
		},
	}})
	inbound.Settings = string(settings)
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	// Canonical clients rows — the sweep reads activation state from here.
	rows := []model.ClientRecord{
		{Email: "late@example.com", SubID: "sub-act-1", ExpiryTime: -30 * msPerDay, ActivateBefore: deadlinePassed},
		{Email: "soon@example.com", SubID: "sub-act-1", ExpiryTime: -30 * msPerDay, ActivateBefore: deadlineAhead},
		{Email: "nodeadline@example.com", SubID: "sub-act-1", ExpiryTime: -30 * msPerDay},
		{Email: "started@example.com", SubID: "sub-act-1", ExpiryTime: now + 30*msPerDay, ActivateBefore: deadlinePassed},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create client row %s: %v", rows[i].Email, err)
		}
		// Production clients always have their client_inbounds association and
		// traffic row — the canonical update path (ResetClientExpiryTimeByEmail)
		// resolves inbounds through them.
		assoc := model.ClientInbound{ClientId: rows[i].Id, InboundId: inbound.Id}
		if err := db.Create(&assoc).Error; err != nil {
			t.Fatalf("create association %s: %v", rows[i].Email, err)
		}
		traffic := xray.ClientTraffic{
			Email:      rows[i].Email,
			InboundId:  inbound.Id,
			ExpiryTime: rows[i].ExpiryTime,
			Enable:     true,
		}
		if err := db.Create(&traffic).Error; err != nil {
			t.Fatalf("create traffic %s: %v", rows[i].Email, err)
		}
	}

	clientSvc := ClientService{}
	inboundSvc := InboundService{}
	converted := clientSvc.SweepActivateBefore(&inboundSvc)
	if converted != 1 {
		t.Fatalf("converted = %d, want only the deadline-passed delayed client", converted)
	}

	var late model.ClientRecord
	if err := db.Where("email = ?", "late@example.com").First(&late).Error; err != nil {
		t.Fatalf("load late client: %v", err)
	}
	if late.ExpiryTime != deadlinePassed {
		t.Fatalf("late expiryTime = %d, want the deadline %d", late.ExpiryTime, deadlinePassed)
	}

	for _, email := range []string{"soon@example.com", "nodeadline@example.com"} {
		var rec model.ClientRecord
		if err := db.Where("email = ?", email).First(&rec).Error; err != nil {
			t.Fatalf("load %s: %v", email, err)
		}
		if rec.ExpiryTime != -30*msPerDay {
			t.Fatalf("%s must stay delayed, got expiryTime %d", email, rec.ExpiryTime)
		}
	}

	// The started client is already running (positive expiry) — untouched.
	var started model.ClientRecord
	if err := db.Where("email = ?", "started@example.com").First(&started).Error; err != nil {
		t.Fatalf("load started client: %v", err)
	}
	if started.ExpiryTime != now+30*msPerDay {
		t.Fatalf("started client expiry changed: %d", started.ExpiryTime)
	}
}
