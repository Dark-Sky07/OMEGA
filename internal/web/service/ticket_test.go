package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestSupportTicketsAndPaymentWebhook(t *testing.T) {
	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	var resellerSvc ResellerService
	r1 := &model.Reseller{Username: "reseller_one", Name: "Reseller One", Enable: true}
	if err := resellerSvc.Add(r1, "pass1234"); err != nil {
		t.Fatalf("Add r1 failed: %v", err)
	}
	r2 := &model.Reseller{Username: "reseller_two", Name: "Reseller Two", Enable: true}
	if err := resellerSvc.Add(r2, "pass1234"); err != nil {
		t.Fatalf("Add r2 failed: %v", err)
	}

	var ticketSvc TicketService
	tk, err := ticketSvc.CreateTicket(r1, "Need 100GB extra quota", "high", "Hello admin, please increase my traffic limit.")
	if err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	if tk.Status != "open" || tk.Priority != "high" {
		t.Fatalf("unexpected ticket: %+v", tk)
	}

	// Reseller 2 must NOT be able to view Reseller 1's ticket
	if _, _, err := ticketSvc.GetTicketWithMessages(tk.Id, r2.Id); err == nil {
		t.Fatal("expected reseller 2 to be denied access to reseller 1 ticket")
	}

	// Admin replies -> status becomes "answered"
	if _, err := ticketSvc.ReplyTicket(tk.Id, "admin", "root", "Done! Added 100GB.", 0); err != nil {
		t.Fatalf("ReplyTicket admin failed: %v", err)
	}
	loadedTk, msgs, err := ticketSvc.GetTicketWithMessages(tk.Id, r1.Id)
	if err != nil || loadedTk.Status != "answered" || len(msgs) != 2 {
		t.Fatalf("unexpected ticket state after admin reply: tk=%+v msgs=%d err=%v", loadedTk, len(msgs), err)
	}

	// Close ticket
	if err := ticketSvc.CloseTicket(tk.Id, r1.Id); err != nil {
		t.Fatalf("CloseTicket failed: %v", err)
	}
	closedTk, _, _ := ticketSvc.GetTicketWithMessages(tk.Id, r1.Id)
	if closedTk.Status != "closed" {
		t.Fatalf("expected closed status, got %q", closedTk.Status)
	}

	// Test HMAC signature verification
	body := []byte(`{"orderId":"ORD-1001","targetType":"reseller_deposit","targetRef":"reseller_one","amount":25.5}`)
	secret := "super-secret-key"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if !VerifyPaymentWebhookHMAC(body, sig, secret) {
		t.Fatal("expected HMAC verification to succeed")
	}
	if VerifyPaymentWebhookHMAC(body, sig, "wrong-secret") {
		t.Fatal("expected HMAC verification with wrong secret to fail")
	}

	// Test PaymentWebhookService idempotency & reseller_deposit
	var webhookSvc PaymentWebhookService
	order1, err := webhookSvc.ProcessWebhook(PaymentWebhookPayload{
		OrderId:    "ORD-1001",
		Gateway:    "cryptomus",
		TargetType: "reseller_deposit",
		TargetRef:  "reseller_one",
		Amount:     25.5,
		AddGB:      50,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook failed: %v", err)
	}
	// Duplicate webhook call with same OrderId must be idempotent (no double credit!)
	orderDup, err := webhookSvc.ProcessWebhook(PaymentWebhookPayload{
		OrderId:    "ORD-1001",
		Gateway:    "cryptomus",
		TargetType: "reseller_deposit",
		TargetRef:  "reseller_one",
		Amount:     25.5,
		AddGB:      50,
	})
	if err != nil || orderDup.Id != order1.Id {
		t.Fatalf("expected idempotent order return, got %+v (%v)", orderDup, err)
	}
	updatedR1, err := resellerSvc.Get(r1.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedR1.Deposit != 25.5 {
		t.Fatalf("expected Deposit=25.5 (not doubled), got %v", updatedR1.Deposit)
	}
}
