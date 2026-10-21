package service

import (
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/xray"
)

// TestResellerBillingStatAndLedger covers the GB-based billing semantics:
// the per-GB price is set at creation and editable afterwards, the cost
// accrues from measured usage, and the balance is deposit minus cost with
// deposits entering exclusively through the audited ledger.
func TestResellerBillingStatAndLedger(t *testing.T) {
	dir := t.TempDir()
	if err := database.InitDB(dir + "/x-ui.db"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	var svc ResellerService

	// Add() keeps the requested per-GB price but forces the deposit to 0.
	r := &model.Reseller{
		Username: "billing-reseller", Name: "Billing Reseller", Enable: true,
		PricePerGB: 0.5, Deposit: 999,
	}
	if err := svc.Add(r, "pass1234"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if r.PricePerGB != 0.5 {
		t.Fatalf("PricePerGB = %v, want 0.5", r.PricePerGB)
	}
	if r.Deposit != 0 {
		t.Fatalf("Deposit = %v, want 0 (deposits must go through the ledger)", r.Deposit)
	}

	// A client owned by the reseller with measured usage.
	client := &model.ClientRecord{Email: "billing-user@x", UUID: "billing-uuid", SubID: "billing-sub", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerClient{ResellerId: r.Id, Email: "billing-user@x"}).Error; err != nil {
		t.Fatal(err)
	}
	// 3 GB of measured usage.
	usedBytes := int64(3 * bytesPerGB)
	if err := db.Create(&xray.ClientTraffic{
		Email: "billing-user@x", Up: usedBytes / 2, Down: usedBytes / 2, Enable: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Stat: cost = 3 GB * 0.5 = 1.5; balance = 0 - 1.5 = -1.5 (debt).
	stat, err := svc.Stat(r)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if stat.UsedTraffic != usedBytes {
		t.Fatalf("UsedTraffic = %d, want %d", stat.UsedTraffic, usedBytes)
	}
	if stat.Cost != 1.5 {
		t.Fatalf("Cost = %v, want 1.5", stat.Cost)
	}
	if stat.Balance != -1.5 {
		t.Fatalf("Balance = %v, want -1.5 (debt)", stat.Balance)
	}

	// AdjustBalance deposits credit through the ledger.
	if _, err := svc.AdjustBalance(r.Id, 10, "initial credit"); err != nil {
		t.Fatalf("AdjustBalance: %v", err)
	}
	fresh, err := svc.Get(r.Id)
	if err != nil {
		t.Fatal(err)
	}
	stat, err = svc.Stat(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Balance != 8.5 {
		t.Fatalf("Balance after deposit = %v, want 8.5", stat.Balance)
	}

	// Withdrawals cannot take the deposit below zero.
	if _, err := svc.AdjustBalance(r.Id, -11, "too much"); err == nil {
		t.Fatal("expected over-withdrawal to be refused")
	}

	// Update() can change the per-GB price; the price applies to the same usage.
	updated := &model.Reseller{
		Username: "billing-reseller", Name: "Billing Reseller", Enable: true,
		PricePerGB: 1, TrafficLimit: 0, ClientLimit: 0,
	}
	updated.Id = r.Id
	if err := svc.Update(updated, ""); err != nil {
		t.Fatalf("Update: %v", err)
	}
	afterUpdate, err := svc.Get(r.Id)
	if err != nil {
		t.Fatal(err)
	}
	stat, err = svc.Stat(afterUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Reseller.PricePerGB != 1 {
		t.Fatalf("PricePerGB after update = %v, want 1", stat.Reseller.PricePerGB)
	}
	if stat.Cost != 3 {
		t.Fatalf("Cost after price change = %v, want 3", stat.Cost)
	}

	// The ledger reflects the audited deposit.
	ledger, err := svc.Transactions(r.Id)
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if len(ledger) != 1 || ledger[0].Type != "deposit" || ledger[0].Amount != 10 || ledger[0].Balance != 10 {
		t.Fatalf("unexpected ledger: %+v", ledger)
	}
}
