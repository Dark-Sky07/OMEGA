package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func TestResellerSignupSignedLinkAndTemplateScope(t *testing.T) {
	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	var inboundSvc InboundService
	inbound, _, err := inboundSvc.AddInbound(&model.Inbound{
		Port: 20443, Protocol: model.VLESS,
		Settings:       `{"clients":[],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Tag:            "signup-inbound", Sniffing: `{"enabled":true,"destOverride":["http","tls"]}`,
	})
	if err != nil {
		t.Fatalf("AddInbound: %v", err)
	}
	reseller := &model.Reseller{Username: "signup-reseller", Password: "test-password", Name: "Signup Shop", BrandName: "Northstar", BrandLogo: "/assets/northstar.svg", BrandColor: "#224466", Enable: true, ClientLimit: 1}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerInbound{ResellerId: reseller.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatal(err)
	}
	template := &model.UserTemplate{
		Name: "Signup Basic", TotalGB: bytesPerGB * 10, ExpireDurationDays: 30,
		InboundIds: []int{inbound.Id},
	}
	if err := db.Create(template).Error; err != nil {
		t.Fatal(err)
	}
	outsideTemplate := &model.UserTemplate{Name: "Out of scope", InboundIds: []int{5}}
	if err := db.Create(outsideTemplate).Error; err != nil {
		t.Fatal(err)
	}

	svc := &ResellerSignupService{}
	secret := []byte("test-signing-secret")
	now := time.Unix(1_800_000_000, 0)
	token, err := svc.CreateLink(reseller.Id, template.Id, secret, now)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if strings.Count(token, ".") != 1 {
		t.Fatalf("token should contain encoded payload and signature, got %q", token)
	}
	info, err := svc.Info(token, secret, now)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ResellerName != "Signup Shop" || info.BrandName != "Northstar" || info.BrandLogo != "/assets/northstar.svg" || info.BrandColor != "#224466" || info.PlanName != "Signup Basic" || info.TotalBytes != 10*bytesPerGB {
		t.Fatalf("unexpected public info: %+v", info)
	}

	eligible, err := svc.EligibleTemplates(reseller.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(eligible) != 1 || eligible[0].Id != template.Id {
		t.Fatalf("eligible templates = %+v, expected only reseller-owned plan", eligible)
	}
	subID, needRestart, err := svc.Register(token, "Customer@Example.com", secret, now)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if subID == "" || !needRestart {
		t.Fatalf("Register returned subId=%q needRestart=%v", subID, needRestart)
	}
	var clientSvc ClientService
	created, err := clientSvc.GetRecordByEmail(nil, "customer@example.com")
	if err != nil || created.TotalGB != template.TotalGB || created.ExpiryTime != now.Add(30*24*time.Hour).UnixMilli() {
		t.Fatalf("created client does not match plan: %+v (%v)", created, err)
	}
	owned, err := (&ResellerService{}).OwnsClient(reseller.Id, "customer@example.com")
	if err != nil || !owned {
		t.Fatalf("new client should be assigned to reseller: owned=%v err=%v", owned, err)
	}
	if _, _, err := svc.Register(token, "another@example.com", secret, now); err == nil {
		t.Fatal("expected client limit to block additional registrations")
	}
	if _, err := svc.CreateLink(reseller.Id, outsideTemplate.Id, secret, now); err == nil {
		t.Fatal("expected link generation for an unowned inbound template to fail")
	}
	if _, err := svc.Info(token+"x", secret, now); err == nil {
		t.Fatal("expected modified signature to fail")
	}
	if _, err := svc.Info(token, secret, now.Add(resellerSignupTTL+time.Second)); err == nil {
		t.Fatal("expected expired registration link to fail")
	}
}
