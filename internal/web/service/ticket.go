package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/util/common"
	moneycurrency "github.com/Dark-Sky07/phoenix/internal/util/currency"
	"gorm.io/gorm"
)

// TicketService manages SupportTickets and SupportTicketMessages between
// Resellers and Panel Admins.
type TicketService struct{}

func normalizeTicketPriority(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "low":
		return "low"
	case "high":
		return "high"
	default:
		return "normal"
	}
}

func (s *TicketService) ListForAdmin(status string) ([]model.SupportTicket, error) {
	db := database.GetDB()
	q := db.Model(&model.SupportTicket{})
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	var rows []model.SupportTicket
	if err := q.Order("updated_at DESC, id DESC").Limit(500).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *TicketService) ListForReseller(resellerId int) ([]model.SupportTicket, error) {
	db := database.GetDB()
	var rows []model.SupportTicket
	if err := db.Where("reseller_id = ?", resellerId).
		Order("updated_at DESC, id DESC").
		Limit(200).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *TicketService) GetTicketWithMessages(ticketId int, resellerId int) (*model.SupportTicket, []model.SupportTicketMessage, error) {
	if ticketId <= 0 {
		return nil, nil, common.NewError("invalid ticket id")
	}
	db := database.GetDB()
	var t model.SupportTicket
	if err := db.First(&t, ticketId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, common.NewError("ticket not found")
		}
		return nil, nil, err
	}
	if resellerId > 0 && t.ResellerId != resellerId {
		return nil, nil, common.NewError("ticket not found")
	}
	var msgs []model.SupportTicketMessage
	if err := db.Where("ticket_id = ?", t.Id).Order("id ASC").Find(&msgs).Error; err != nil {
		return nil, nil, err
	}
	return &t, msgs, nil
}

func (s *TicketService) CreateTicket(reseller *model.Reseller, subject, priority, message string) (*model.SupportTicket, error) {
	if reseller == nil || reseller.Id <= 0 {
		return nil, common.NewError("reseller is required")
	}
	subject = strings.TrimSpace(subject)
	message = strings.TrimSpace(message)
	if subject == "" {
		return nil, common.NewError("subject is required")
	}
	if message == "" {
		return nil, common.NewError("message is required")
	}
	db := database.GetDB()
	now := time.Now().UnixMilli()
	var created model.SupportTicket
	err := db.Transaction(func(tx *gorm.DB) error {
		created = model.SupportTicket{
			ResellerId:       reseller.Id,
			ResellerUsername: reseller.Username,
			Subject:          subject,
			Status:           "open",
			Priority:         normalizeTicketPriority(priority),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		msg := model.SupportTicketMessage{
			TicketId:   created.Id,
			SenderRole: "reseller",
			SenderName: reseller.Username,
			Message:    message,
			CreatedAt:  now,
		}
		return tx.Create(&msg).Error
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *TicketService) ReplyTicket(ticketId int, senderRole, senderName, message string, resellerId int) (*model.SupportTicketMessage, error) {
	if ticketId <= 0 {
		return nil, common.NewError("invalid ticket id")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, common.NewError("message is required")
	}
	db := database.GetDB()
	var t model.SupportTicket
	if err := db.First(&t, ticketId).Error; err != nil {
		return nil, common.NewError("ticket not found")
	}
	if resellerId > 0 && t.ResellerId != resellerId {
		return nil, common.NewError("ticket not found")
	}
	now := time.Now().UnixMilli()
	nextStatus := "open"
	if senderRole == "admin" {
		nextStatus = "answered"
	}
	var msg model.SupportTicketMessage
	err := db.Transaction(func(tx *gorm.DB) error {
		msg = model.SupportTicketMessage{
			TicketId:   t.Id,
			SenderRole: senderRole,
			SenderName: senderName,
			Message:    message,
			CreatedAt:  now,
		}
		if err := tx.Create(&msg).Error; err != nil {
			return err
		}
		return tx.Model(&model.SupportTicket{}).
			Where("id = ?", t.Id).
			Updates(map[string]any{
				"status":     nextStatus,
				"updated_at": now,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *TicketService) CloseTicket(ticketId int, resellerId int) error {
	if ticketId <= 0 {
		return common.NewError("invalid ticket id")
	}
	db := database.GetDB()
	var t model.SupportTicket
	if err := db.First(&t, ticketId).Error; err != nil {
		return common.NewError("ticket not found")
	}
	if resellerId > 0 && t.ResellerId != resellerId {
		return common.NewError("ticket not found")
	}
	return db.Model(&model.SupportTicket{}).
		Where("id = ?", t.Id).
		Updates(map[string]any{
			"status":     "closed",
			"updated_at": time.Now().UnixMilli(),
		}).Error
}

// ---------------------------------------------------------------------------
// Payment / Order Webhook Service
// ---------------------------------------------------------------------------

type PaymentWebhookPayload struct {
	OrderId      string  `json:"orderId"`
	Gateway      string  `json:"gateway"`
	TargetType   string  `json:"targetType"` // reseller_deposit | reseller_quota | client_renew
	TargetRef    string  `json:"targetRef"`  // reseller username or client email
	Amount       float64 `json:"amount"`
	AddGB        int64   `json:"addGb"`
	AddDays      int     `json:"addDays"`
	ResetTraffic bool    `json:"resetTraffic"`
}

type PaymentWebhookService struct {
	resellerService ResellerService
	clientService   ClientService
	inboundService  InboundService
}

func VerifyPaymentWebhookHMAC(rawBody []byte, signatureHex string, secret string) bool {
	secret = strings.TrimSpace(secret)
	signatureHex = strings.TrimSpace(strings.TrimPrefix(signatureHex, "sha256="))
	if secret == "" || signatureHex == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, got)
}

func (s *PaymentWebhookService) ListOrders(limit int) ([]model.PaymentOrder, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	db := database.GetDB()
	var rows []model.PaymentOrder
	if err := db.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *PaymentWebhookService) ProcessWebhook(p PaymentWebhookPayload) (*model.PaymentOrder, error) {
	p.OrderId = strings.TrimSpace(p.OrderId)
	p.Gateway = strings.TrimSpace(p.Gateway)
	p.TargetType = strings.ToLower(strings.TrimSpace(p.TargetType))
	p.TargetRef = strings.TrimSpace(p.TargetRef)

	if p.OrderId == "" {
		return nil, common.NewError("orderId is required")
	}

	db := database.GetDB()
	// Idempotency check: if OrderId was already processed, return the existing order.
	var existing model.PaymentOrder
	if err := db.Where("order_id = ?", p.OrderId).First(&existing).Error; err == nil {
		if existing.Status != "pending" {
			return &existing, nil
		}
		// A pending checkout order is being confirmed by the gateway: the
		// effects were chosen when the order was created, the webhook only
		// proves the payment happened. targetRef/targetType of the payload
		// are ignored — the stored order stays authoritative.
		var settingSvc SettingService
		configuredCurrency, _ := settingSvc.GetCheckoutCurrency()
		amountTolerance := 0.5 / math.Pow10(moneycurrency.FractionDigits(configuredCurrency))
		if p.Amount > 0 && existing.Amount > 0 && math.Abs(p.Amount-existing.Amount) > amountTolerance {
			return nil, common.NewError("webhook amount does not match pending order ", p.OrderId)
		}
		if err := s.applyOrderEffects(&existing); err != nil {
			return nil, err
		}
		if err := db.Model(&model.PaymentOrder{}).Where("id = ?", existing.Id).
			Update("status", "completed").Error; err != nil {
			return nil, err
		}
		existing.Status = "completed"
		return &existing, nil
	}

	// Fresh (non-checkout) orders describe their own target; pending checkout
	// completion above is the only flow allowed to omit targetRef.
	if p.TargetRef == "" {
		return nil, common.NewError("targetRef is required")
	}
	if p.Gateway == "" {
		p.Gateway = "webhook"
	}

	order := &model.PaymentOrder{
		OrderId:      p.OrderId,
		Gateway:      p.Gateway,
		TargetType:   p.TargetType,
		TargetRef:    p.TargetRef,
		Amount:       p.Amount,
		AddGB:        p.AddGB,
		AddDays:      p.AddDays,
		ResetTraffic: p.ResetTraffic,
		Status:       "completed",
	}
	if err := s.applyOrderEffects(order); err != nil {
		return nil, err
	}
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

// applyOrderEffects performs one order's side effects: topping up a reseller
// deposit, extending a reseller quota, or renewing a client.
func (s *PaymentWebhookService) applyOrderEffects(order *model.PaymentOrder) error {
	db := database.GetDB()
	switch order.TargetType {
	case "reseller_deposit":
		if order.Amount <= 0 {
			return common.NewError("positive amount is required for reseller_deposit")
		}
		reseller, err := s.resellerService.GetByUsername(order.TargetRef)
		if err != nil {
			return fmt.Errorf("reseller %q not found: %w", order.TargetRef, err)
		}
		comment := fmt.Sprintf("Webhook (%s) #%s", order.Gateway, order.OrderId)
		if _, err := s.resellerService.AdjustBalance(reseller.Id, order.Amount, comment); err != nil {
			return err
		}
		if order.AddGB > 0 {
			addBytes := order.AddGB * bytesPerGB
			_ = db.Model(&model.Reseller{}).
				Where("id = ?", reseller.Id).
				Update("traffic_limit", gorm.Expr("traffic_limit + ?", addBytes)).Error
		}

	case "reseller_quota":
		reseller, err := s.resellerService.GetByUsername(order.TargetRef)
		if err != nil {
			return fmt.Errorf("reseller %q not found: %w", order.TargetRef, err)
		}
		updates := map[string]any{"updated_at": time.Now().UnixMilli()}
		if order.AddGB > 0 {
			updates["traffic_limit"] = reseller.TrafficLimit + (order.AddGB * bytesPerGB)
		}
		if order.AddDays > 0 {
			base := time.Now().UnixMilli()
			if reseller.ExpiryTime > base {
				base = reseller.ExpiryTime
			}
			updates["expiry_time"] = base + int64(order.AddDays)*86400000
		}
		if err := db.Model(&model.Reseller{}).Where("id = ?", reseller.Id).Updates(updates).Error; err != nil {
			return err
		}

	case "client_renew":
		rec, err := s.clientService.GetRecordByEmail(nil, order.TargetRef)
		if err != nil {
			return fmt.Errorf("client %q not found: %w", order.TargetRef, err)
		}
		if order.ResetTraffic {
			_ = s.inboundService.ResetClientTrafficByEmail(order.TargetRef)
		}
		updated := *rec.ToClient()
		if order.AddGB > 0 {
			updated.TotalGB += order.AddGB * bytesPerGB
		}
		if order.AddDays > 0 {
			nowMs := time.Now().UnixMilli()
			if updated.ExpiryTime < 0 {
				updated.ExpiryTime -= int64(order.AddDays) * 86400000
			} else {
				base := nowMs
				if updated.ExpiryTime > nowMs {
					base = updated.ExpiryTime
				}
				updated.ExpiryTime = base + int64(order.AddDays)*86400000
			}
		}
		updated.Enable = true
		if _, err := s.clientService.UpdateByEmail(&s.inboundService, order.TargetRef, updated); err != nil {
			return err
		}

	default:
		return common.NewError("unsupported targetType:", order.TargetType)
	}
	return nil
}
