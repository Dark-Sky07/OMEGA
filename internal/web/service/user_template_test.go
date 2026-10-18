package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestUserTemplateCRUDAndRevokeSubscription(t *testing.T) {
	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	var tplSvc UserTemplateService
	created, err := tplSvc.Create(&model.UserTemplate{
		Name:               "Gold 50GB / 30d",
		Description:        "Monthly standard plan",
		TotalGB:            50 * 1024 * 1024 * 1024,
		ExpireDurationDays: 30,
		DelayedStart:       true,
		ActivateBeforeDays: 7,
		LimitIP:            2,
		LimitDevice:        3,
		Reset:              30,
		RenewalMode:        1,
		RenewalDay:         1,
		Group:              "VIP",
		InboundIds:         []int{1, 2, 1},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.Id <= 0 || len(created.InboundIds) != 2 {
		t.Fatalf("unexpected created template: %+v", created)
	}

	// Duplicate name (case-insensitive) should fail.
	if _, err := tplSvc.Create(&model.UserTemplate{Name: "gold 50gb / 30d"}); err == nil {
		t.Fatal("expected duplicate template name to fail")
	}

	list, err := tplSvc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List failed: %v, len=%d", err, len(list))
	}

	updated, err := tplSvc.Update(created.Id, &model.UserTemplate{
		Name:               "Platinum 100GB / 30d",
		TotalGB:            100 * 1024 * 1024 * 1024,
		ExpireDurationDays: 30,
		DelayedStart:       false,
		ActivateBeforeDays: 10, // should be cleared when DelayedStart=false
		InboundIds:         []int{2, 3},
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "Platinum 100GB / 30d" || updated.ActivateBeforeDays != 0 {
		t.Fatalf("unexpected updated template: %+v", updated)
	}

	if err := tplSvc.Delete(created.Id); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	listAfter, err := tplSvc.List()
	if err != nil || len(listAfter) != 0 {
		t.Fatalf("expected 0 templates after delete, got %d (%v)", len(listAfter), err)
	}

	// Test RevokeSubscription
	var inboundSvc InboundService
	var clientSvc ClientService
	ib, _, err := inboundSvc.AddInbound(&model.Inbound{
		UserId:         1,
		Port:           20443,
		Protocol:       model.VLESS,
		Settings:       `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"revoke@test.local","subId":"oldsubid12345678","enable":true}],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Tag:            "inbound-20443",
		Sniffing:       `{"enabled":true,"destOverride":["http","tls"]}`,
	})
	if err != nil {
		t.Fatalf("AddInbound failed: %v", err)
	}

	newSubID, err := clientSvc.RevokeSubscription(&inboundSvc, "revoke@test.local")
	if err != nil {
		t.Fatalf("RevokeSubscription failed: %v", err)
	}
	if len(newSubID) != 16 || newSubID == "oldsubid12345678" {
		t.Fatalf("unexpected newSubID: %q", newSubID)
	}
	rec, err := clientSvc.GetRecordByEmail(nil, "revoke@test.local")
	if err != nil || rec.SubID != newSubID {
		t.Fatalf("expected ClientRecord.SubID=%q, got %+v (%v)", newSubID, rec, err)
	}
	reloadedIb, err := inboundSvc.GetInbound(ib.Id)
	if err != nil {
		t.Fatalf("GetInbound failed: %v", err)
	}
	if !strings.Contains(reloadedIb.Settings, newSubID) {
		t.Fatalf("expected inbound settings to contain new subId %q, got %s", newSubID, reloadedIb.Settings)
	}
}
