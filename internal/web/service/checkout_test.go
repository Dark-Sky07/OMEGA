package service

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func enableCheckout(t *testing.T, pricePerGB, pricePerMonth float64) {
	t.Helper()
	var settingSvc SettingService
	if err := settingSvc.setBool("checkoutEnable", true); err != nil {
		t.Fatalf("setBool checkoutEnable: %v", err)
	}
	if err := settingSvc.setString("checkoutCurrency", "USD"); err != nil {
		t.Fatalf("setString checkoutCurrency: %v", err)
	}
	if err := settingSvc.setString("checkoutPricePerGB", strconv.FormatFloat(pricePerGB, 'f', -1, 64)); err != nil {
		t.Fatalf("set checkoutPricePerGB: %v", err)
	}
	if err := settingSvc.setString("checkoutPricePerMonth", strconv.FormatFloat(pricePerMonth, 'f', -1, 64)); err != nil {
		t.Fatalf("set checkoutPricePerMonth: %v", err)
	}
}

func seedCheckoutClient(t *testing.T) string {
	t.Helper()
	svc := &ClientService{}
	clients := []model.Client{
		{Email: "checkout-user@x", ID: "44444444-4444-4444-4444-444444444444", SubID: "schk", Enable: true},
	}
	ib := mkInbound(t, 20011, model.VLESS, clientsSettings(t, clients))
	if err := svc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("SyncInbound: %v", err)
	}
	return "checkout-user@x"
}

func TestCheckoutRefToken(t *testing.T) {
	secret := []byte("panel-secret")
	now := time.Now()

	// Round-trip, with case/trim normalization
	token, err := SignCheckoutRef("  Alice@X.COM ", secret, now)
	if err != nil {
		t.Fatalf("SignCheckoutRef: %v", err)
	}
	email, err := ParseCheckoutRef(token, secret, now)
	if err != nil || email != "alice@x.com" {
		t.Fatalf("ParseCheckoutRef round-trip: email=%q err=%v", email, err)
	}

	// Different secret must reject
	if _, err := ParseCheckoutRef(token, []byte("other-secret"), now); err != ErrCheckoutInvalidRef {
		t.Fatalf("expected ErrCheckoutInvalidRef for wrong secret, got %v", err)
	}

	// Tampered payload must reject
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected token shape: %q", token)
	}
	tampered := parts[0][:len(parts[0])-2] + "XX." + parts[1]
	if _, err := ParseCheckoutRef(tampered, secret, now); err == nil {
		t.Fatal("expected tampered payload to be rejected")
	}

	// Expired token must reject with ErrCheckoutExpiredRef
	old, err := SignCheckoutRef("alice@x.com", secret, now.Add(-366*24*time.Hour))
	if err != nil {
		t.Fatalf("SignCheckoutRef old: %v", err)
	}
	if _, err := ParseCheckoutRef(old, secret, now); err != ErrCheckoutExpiredRef {
		t.Fatalf("expected ErrCheckoutExpiredRef, got %v", err)
	}

	// Garbage tokens must reject
	for _, junk := range []string{"", "abc", "....", "a.b", token + "x"} {
		if _, err := ParseCheckoutRef(junk, secret, now); err == nil {
			t.Fatalf("expected junk token %q to be rejected", junk)
		}
	}

	// Empty email is refused at signing
	if _, err := SignCheckoutRef("   ", secret, now); err == nil {
		t.Fatal("expected empty email to fail signing")
	}
}

func TestCheckoutInfoDisabled(t *testing.T) {
	setupBulkDB(t) // fresh DB keeps checkoutEnable=false default
	var svc CheckoutService
	if _, err := svc.Info("anyone@x"); err != ErrCheckoutDisabled {
		t.Fatalf("expected ErrCheckoutDisabled on fresh settings, got %v", err)
	}
}

func TestCheckoutOrderAndWebhookCompletion(t *testing.T) {
	setupBulkDB(t)
	email := seedCheckoutClient(t)
	enableCheckout(t, 0.5, 4)

	var svc CheckoutService

	// Info returns plan state + pricing
	info, err := svc.Info(email)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Email != email || info.Currency != "USD" || info.PricePerGB != 0.5 || info.PricePerMonth != 4 {
		t.Fatalf("unexpected info: %+v", info)
	}

	// Validation: nothing selected
	if _, err := svc.CreateOrder(email, 0, 0); err == nil {
		t.Fatal("expected empty order to fail")
	}
	// Validation: out of range
	if _, err := svc.CreateOrder(email, 10001, 0); err == nil {
		t.Fatal("expected gb>10000 to fail")
	}
	if _, err := svc.CreateOrder(email, 0, 37); err == nil {
		t.Fatal("expected months>36 to fail")
	}
	// Unknown client
	if _, err := svc.CreateOrder("ghost@x", 10, 0); err == ErrCheckoutInvalidRef {
		// ok
	} else if err == nil {
		t.Fatal("expected ghost client order to fail")
	}

	// Happy path: 20GB + 1 month = 20*0.5 + 4 = 14
	order, err := svc.CreateOrder(email, 20, 1)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.Amount != 14 || !strings.HasPrefix(order.OrderId, "co_") || order.GatewayURL != "" {
		t.Fatalf("unexpected order: %+v", order)
	}

	// Status of the fresh order is pending
	status, err := svc.Status(email, order.OrderId)
	if err != nil || status != "pending" {
		t.Fatalf("Status: %q err=%v", status, err)
	}
	// Another client's order id reads as unknown
	if st, _ := svc.Status("someoneelse@x", order.OrderId); st != "unknown" {
		t.Fatalf("expected unknown for foreign order, got %q", st)
	}

	// Pending cap: 10 open orders max
	for i := 0; i < 9; i++ {
		if _, err := svc.CreateOrder(email, 1, 0); err != nil {
			t.Fatalf("CreateOrder #%d: %v", i+2, err)
		}
	}
	if _, err := svc.CreateOrder(email, 1, 0); err == nil {
		t.Fatal("expected 11th pending order to be refused")
	}

	// Amount mismatch on webhook completion is refused
	var webhookSvc PaymentWebhookService
	if _, err := webhookSvc.ProcessWebhook(PaymentWebhookPayload{OrderId: order.OrderId, Amount: 999}); err == nil {
		t.Fatal("expected amount mismatch to fail")
	}

	// Webhook completes the pending order using only orderId+amount
	completed, err := webhookSvc.ProcessWebhook(PaymentWebhookPayload{OrderId: order.OrderId, Amount: 14})
	if err != nil {
		t.Fatalf("ProcessWebhook complete: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("expected completed status, got %q", completed.Status)
	}

	// Effects were applied: +20GB quota, +30d expiry, enabled
	var clientSvc ClientService
	rec, err := clientSvc.GetRecordByEmail(nil, email)
	if err != nil {
		t.Fatal(err)
	}
	if rec.TotalGB != 20*bytesPerGB {
		t.Fatalf("expected quota 20GB in bytes, got %d", rec.TotalGB)
	}
	if !rec.Enable {
		t.Fatal("expected client to be enabled after renewal")
	}
	if rec.ExpiryTime <= time.Now().UnixMilli() {
		t.Fatalf("expected future expiry, got %d", rec.ExpiryTime)
	}

	// Second webhook with same order id is idempotent: no double quota
	dup, err := webhookSvc.ProcessWebhook(PaymentWebhookPayload{OrderId: order.OrderId, Amount: 14})
	if err != nil || dup.Id != completed.Id || dup.Status != "completed" {
		t.Fatalf("expected idempotent completed return, got %+v (%v)", dup, err)
	}
	rec2, _ := clientSvc.GetRecordByEmail(nil, email)
	if rec2.TotalGB != 20*bytesPerGB {
		t.Fatalf("expected quota unchanged after duplicate webhook, got %d", rec2.TotalGB)
	}

	// Gateway URL placeholders get substituted
	var settingSvc SettingService
	if err := settingSvc.setString("checkoutGatewayURL", "https://pay.example.com/i?id={orderId}&a={amount}&r={ref}"); err != nil {
		t.Fatal(err)
	}
	order2, err := svc.CreateOrder(email, 10, 0)
	if err != nil {
		t.Fatalf("CreateOrder with gateway: %v", err)
	}
	want := "https://pay.example.com/i?id=" + order2.OrderId + "&a=5&r=" + email
	if order2.GatewayURL != want {
		t.Fatalf("gateway URL: got %q want %q", order2.GatewayURL, want)
	}
}

func TestCheckoutPendingOrderSurvivesDisabledCheckout(t *testing.T) {
	setupBulkDB(t)
	email := seedCheckoutClient(t)
	enableCheckout(t, 1, 2)

	var svc CheckoutService
	order, err := svc.CreateOrder(email, 5, 0)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	// Even after checkout is disabled again, a paid order still completes:
	// the gateway callback is proof of payment, not a new order.
	var settingSvc SettingService
	if err := settingSvc.setBool("checkoutEnable", false); err != nil {
		t.Fatal(err)
	}
	var webhookSvc PaymentWebhookService
	completed, err := webhookSvc.ProcessWebhook(PaymentWebhookPayload{OrderId: order.OrderId, Amount: 5})
	if err != nil {
		t.Fatalf("ProcessWebhook after disable: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("expected completed, got %q", completed.Status)
	}
}

func TestCheckoutFreshWebhookStillNeedsTargetRef(t *testing.T) {
	setupBulkDB(t)
	var webhookSvc PaymentWebhookService
	if _, err := webhookSvc.ProcessWebhook(PaymentWebhookPayload{OrderId: "ORD-NEVER-SEEN"}); err == nil {
		t.Fatal("expected unknown order without targetRef to be refused")
	}
}
