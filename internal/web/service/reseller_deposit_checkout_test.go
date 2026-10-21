package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func TestResellerDepositCheckoutWebhookAndToken(t *testing.T) {
	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	var settingSvc SettingService
	if err := settingSvc.setBool("checkoutEnable", true); err != nil {
		t.Fatal(err)
	}
	if err := settingSvc.setString("checkoutCurrency", "USD"); err != nil {
		t.Fatal(err)
	}
	if err := settingSvc.setString("checkoutGatewayURL", "https://pay.example.test/?id={orderId}&amount={amount}&ref={ref}"); err != nil {
		t.Fatal(err)
	}
	if err := settingSvc.setString("checkoutPayInfo", "Pay the amount shown."); err != nil {
		t.Fatal(err)
	}

	reseller := &model.Reseller{Username: "deposit-shop", Name: "Deposit Shop", Password: "hash", Enable: true}
	if err := database.GetDB().Create(reseller).Error; err != nil {
		t.Fatal(err)
	}
	secret := []byte("reseller-deposit-test-secret")
	now := time.Unix(1_800_000_000, 0)
	svc := &ResellerDepositCheckoutService{}
	token, err := svc.CreateLink(reseller.Id, secret, now)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	info, err := svc.Info(token, secret, now)
	if err != nil || info.ResellerName != "Deposit Shop" || info.Currency != "USD" {
		t.Fatalf("Info = %+v, err=%v", info, err)
	}
	if _, err := svc.Info(token, secret, now.Add(resellerDepositLinkTTL+time.Second)); err == nil {
		t.Fatal("expired link must be rejected")
	}

	order, err := svc.CreateOrder(token, 25.567, secret, now)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.Amount != 25.57 || order.Currency != "USD" || order.GatewayURL == "" || order.OrderId == "" {
		t.Fatalf("unexpected deposit order: %+v", order)
	}
	status, err := svc.Status(token, order.OrderId, secret, now)
	if err != nil || status != "pending" {
		t.Fatalf("initial Status = %q, err=%v", status, err)
	}

	var paymentSvc PaymentWebhookService
	completed, err := paymentSvc.ProcessWebhook(PaymentWebhookPayload{
		OrderId: order.OrderId, Gateway: "checkout", Amount: order.Amount,
	})
	if err != nil || completed.Status != "completed" {
		t.Fatalf("ProcessWebhook = %+v, err=%v", completed, err)
	}
	completedAgain, err := paymentSvc.ProcessWebhook(PaymentWebhookPayload{
		OrderId: order.OrderId, Gateway: "checkout", Amount: order.Amount,
	})
	if err != nil || completedAgain.Status != "completed" {
		t.Fatalf("idempotent ProcessWebhook = %+v, err=%v", completedAgain, err)
	}
	fresh, err := (&ResellerService{}).Get(reseller.Id)
	if err != nil || fresh.Deposit != 25.57 {
		t.Fatalf("reseller deposit = %v, err=%v; want 25.57", fresh.Deposit, err)
	}
	status, err = svc.Status(token, order.OrderId, secret, now)
	if err != nil || status != "completed" {
		t.Fatalf("final Status = %q, err=%v", status, err)
	}
}
