package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func TestResellerTelegramLinkLifecycle(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "tg-link", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	svc := &ResellerTelegramService{}

	// No code yet: redeem fails as not found.
	if _, err := svc.RedeemLinkCode("deadbeef", 111); !errors.Is(err, ErrResellerTgLinkNotFound) {
		t.Fatalf("redeem without code err = %v, want ErrResellerTgLinkNotFound", err)
	}

	// Generate and redeem a code.
	code, expiresAt, err := svc.CreateLinkCode(reseller.Id)
	if err != nil {
		t.Fatalf("CreateLinkCode: %v", err)
	}
	if code == "" || expiresAt <= time.Now().UnixMilli() {
		t.Fatalf("code=%q expiresAt=%d", code, expiresAt)
	}
	linked, err := svc.RedeemLinkCode(code, 111)
	if err != nil {
		t.Fatalf("RedeemLinkCode: %v", err)
	}
	if linked.Id != reseller.Id || linked.TgID != 111 {
		t.Fatalf("linked = id %d tg %d, want id %d tg 111", linked.Id, linked.TgID, reseller.Id)
	}

	// The code is single-use: a second redeem is "not found" (code cleared).
	if _, err := svc.RedeemLinkCode(code, 222); !errors.Is(err, ErrResellerTgLinkNotFound) {
		t.Fatalf("second redeem err = %v, want ErrResellerTgLinkNotFound", err)
	}

	// The linked reseller resolves by chat id.
	found, err := svc.ResellerByTgID(111)
	if err != nil || found.Id != reseller.Id {
		t.Fatalf("ResellerByTgID = %v, %v; want id %d", found, err, reseller.Id)
	}

	// Rebinding the same Telegram account to another reseller is rejected.
	other := &model.Reseller{Username: "tg-link-other", Password: "hash", Enable: true}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create other reseller: %v", err)
	}
	otherCode, _, err := svc.CreateLinkCode(other.Id)
	if err != nil {
		t.Fatalf("CreateLinkCode other: %v", err)
	}
	if _, err := svc.RedeemLinkCode(otherCode, 111); !errors.Is(err, ErrResellerTgAlreadyLinked) {
		t.Fatalf("cross-bind err = %v, want ErrResellerTgAlreadyLinked", err)
	}

	// Rebinding the same reseller with a fresh code is allowed.
	newCode, _, err := svc.CreateLinkCode(reseller.Id)
	if err != nil {
		t.Fatalf("CreateLinkCode again: %v", err)
	}
	if _, err := svc.RedeemLinkCode(newCode, 333); err != nil {
		t.Fatalf("rebind same reseller: %v", err)
	}
	if found, _ := svc.ResellerByTgID(333); found == nil || found.Id != reseller.Id {
		t.Fatalf("rebind did not take effect")
	}

	// Unlink clears the binding.
	if err := svc.Unlink(reseller.Id); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if found, err := svc.ResellerByTgID(333); err == nil || found != nil {
		t.Fatalf("ResellerByTgID after unlink = %v, %v; want not found", found, err)
	}
}

func TestResellerTelegramLinkExpiry(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "tg-expiry", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	svc := &ResellerTelegramService{}

	code, _, err := svc.CreateLinkCode(reseller.Id)
	if err != nil {
		t.Fatalf("CreateLinkCode: %v", err)
	}
	// Force the code into the past, then redeem must report expiry.
	if err := db.Model(model.Reseller{}).Where("id = ?", reseller.Id).
		Update("tg_code_expiry", time.Now().Add(-time.Minute).UnixMilli()).Error; err != nil {
		t.Fatalf("age code: %v", err)
	}
	if _, err := svc.RedeemLinkCode(code, 111); !errors.Is(err, ErrResellerTgLinkExpired) {
		t.Fatalf("expired redeem err = %v, want ErrResellerTgLinkExpired", err)
	}

	// A new code invalidates the stale one.
	if _, _, err := svc.CreateLinkCode(reseller.Id); err != nil {
		t.Fatalf("CreateLinkCode refresh: %v", err)
	}
	var refreshed model.Reseller
	if err := db.Where("id = ?", reseller.Id).First(&refreshed).Error; err != nil {
		t.Fatalf("reload reseller: %v", err)
	}
	if refreshed.TgCode == code {
		t.Fatalf("old code still set after refresh")
	}
}

func TestResellerTelegramDisabledResellerUnlinked(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "tg-disabled", Password: "hash", Enable: true, TgID: 555}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	svc := &ResellerTelegramService{}
	if found, err := svc.ResellerByTgID(555); err != nil || found == nil {
		t.Fatalf("enabled reseller should resolve, got %v, %v", found, err)
	}
	if err := db.Model(model.Reseller{}).Where("id = ?", reseller.Id).Update("enable", false).Error; err != nil {
		t.Fatalf("disable reseller: %v", err)
	}
	if found, err := svc.ResellerByTgID(555); err == nil || found != nil {
		t.Fatalf("disabled reseller must be treated as unlinked, got %v, %v", found, err)
	}
}
