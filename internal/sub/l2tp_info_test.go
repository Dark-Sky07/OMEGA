package sub

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestGetL2TPConnectionBySubIdIsScopedToClientSubscription(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	const targetSub = "sub-l2tp-target"
	const otherSub = "sub-l2tp-other"

	target := model.Client{
		Email:    "target@example.com",
		Password: "target-password",
		Enable:   true,
		SubID:    targetSub,
	}
	targetTwo := model.Client{
		Email:    "target-two@example.com",
		Password: "target-two-password",
		Enable:   true,
		SubID:    targetSub,
	}
	other := model.Client{
		Email:    "other@example.com",
		Password: "other-password",
		Enable:   true,
		SubID:    otherSub,
	}
	settings, err := json.Marshal(map[string]any{
		"psk":             "test-psk",
		"poolCIDR":        "10.252.0.0/24",
		"localIP":         "10.252.0.1",
		"poolStart":       "10.252.0.10",
		"poolEnd":         "10.252.0.250",
		"dns1":            "1.1.1.1",
		"dns2":            "8.8.8.8",
		"redirectGateway": true,
		"clients":         []model.Client{target, targetTwo, other},
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	inbound := &model.Inbound{
		UserId:   1,
		Tag:      "l2tp-native",
		Enable:   true,
		Port:     1701,
		Protocol: model.L2TP,
		Settings: string(settings),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("seed inbound: %v", err)
	}

	targetRecord := &model.ClientRecord{Email: target.Email, Password: target.Password, Enable: true, SubID: targetSub}
	targetTwoRecord := &model.ClientRecord{Email: targetTwo.Email, Password: targetTwo.Password, Enable: true, SubID: targetSub}
	otherRecord := &model.ClientRecord{Email: other.Email, Password: other.Password, Enable: true, SubID: otherSub}
	for _, record := range []*model.ClientRecord{targetRecord, targetTwoRecord, otherRecord} {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("seed client %s: %v", record.Email, err)
		}
	}
	links := []model.ClientInbound{
		{ClientId: targetRecord.Id, InboundId: inbound.Id},
		{ClientId: targetTwoRecord.Id, InboundId: inbound.Id},
		{ClientId: otherRecord.Id, InboundId: inbound.Id},
	}
	for i := range links {
		if err := db.Create(&links[i]).Error; err != nil {
			t.Fatalf("seed client_inbound: %v", err)
		}
	}

	s := NewSubService(false, "-ieo")
	connections, err := s.GetL2TPConnectionsBySubId(targetSub, "vpn.example.com")
	if err != nil {
		t.Fatalf("GetL2TPConnectionsBySubId: %v", err)
	}
	if len(connections) != 2 {
		t.Fatalf("target subscription should receive both L2TP clients, got %+v", connections)
	}
	if connections[0].ServerAddress != "vpn.example.com" || connections[0].Username != target.Email || connections[0].Password != target.Password {
		t.Fatalf("first connection identity = %+v, want target client and host", connections[0])
	}
	if connections[1].Username != targetTwo.Email || connections[1].Password != targetTwo.Password {
		t.Fatalf("second connection identity = %+v, want second target client", connections[1])
	}
	if connections[0].PSK != "test-psk" || len(connections[0].FixedPorts) != 3 || connections[0].FixedPorts[0] != 500 || connections[0].FixedPorts[1] != 4500 || connections[0].FixedPorts[2] != 1701 {
		t.Fatalf("connection daemon parameters = %+v", connections[0])
	}

	otherConnections, err := s.GetL2TPConnectionsBySubId("sub-does-not-exist", "vpn.example.com")
	if err != nil {
		t.Fatalf("GetL2TPConnectionsBySubId(other): %v", err)
	}
	if len(otherConnections) != 0 {
		t.Fatalf("unrelated subscription must not receive L2TP credentials: %+v", otherConnections)
	}
}
