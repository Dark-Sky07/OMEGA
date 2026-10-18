package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
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
	if p.TargetRef == "" {
		return nil, common.NewError("targetRef is required")
	}
	if p.Gateway == "" {
		p.Gateway = "webhook"
	}

	db := database.GetDB()
	// Idempotency check: if OrderId was already processed, return the existing order.
	var existing model.PaymentOrder
	if err := db.Where("order_id = ?", p.OrderId).First(&existing).Error; err == nil {
		return &existing, nil
	}

	switch p.TargetType {
	case "reseller_deposit":
		if p.Amount <= 0 {
			return nil, common.NewError("positive amount is required for reseller_deposit")
		}
		reseller, err := s.resellerService.GetByUsername(p.TargetRef)
		if err != nil {
			return nil, fmt.Errorf("reseller %q not found: %w", p.TargetRef, err)
		}
		comment := fmt.Sprintf("Webhook (%s) #%s", p.Gateway, p.OrderId)
		if _, err := s.resellerService.AdjustBalance(reseller.Id, p.Amount, comment); err != nil {
			return nil, err
		}
		if p.AddGB > 0 {
			addBytes := p.AddGB * bytesPerGB
			_ = db.Model(&model.Reseller{}).
				Where("id = ?", reseller.Id).
				Update("traffic_limit", gorm.Expr("traffic_limit + ?", addBytes)).Error
		}

	case "reseller_quota":
		reseller, err := s.resellerService.GetByUsername(p.TargetRef)
		if err != nil {
			return nil, fmt.Errorf("reseller %q not found: %w", p.TargetRef, err)
		}
		updates := map[string]any{"updated_at": time.Now().UnixMilli()}
		if p.AddGB > 0 {
			updates["traffic_limit"] = reseller.TrafficLimit + (p.AddGB * bytesPerGB)
		}
		if p.AddDays > 0 {
			base := time.Now().UnixMilli()
			if reseller.ExpiryTime > base {
				base = reseller.ExpiryTime
			}
			updates["expiry_time"] = base + int64(p.AddDays)*86400000
		}
		if err := db.Model(&model.Reseller{}).Where("id = ?", reseller.Id).Updates(updates).Error; err != nil {
			return nil, err
		}

	case "client_renew":
		rec, err := s.clientService.GetRecordByEmail(nil, p.TargetRef)
		if err != nil {
			return nil, fmt.Errorf("client %q not found: %w", p.TargetRef, err)
		}
		if p.ResetTraffic {
			_ = s.inboundService.ResetClientTrafficByEmail(p.TargetRef)
		}
		updated := *rec.ToClient()
		if p.AddGB > 0 {
			updated.TotalGB += p.AddGB * bytesPerGB
		}
		if p.AddDays > 0 {
			nowMs := time.Now().UnixMilli()
			if updated.ExpiryTime < 0 {
				updated.ExpiryTime -= int64(p.AddDays) * 86400000
			} else {
				base := nowMs
				if updated.ExpiryTime > nowMs {
					base = updated.ExpiryTime
				}
				updated.ExpiryTime = base + int64(p.AddDays)*86400000
			}
		}
		updated.Enable = true
		if _, err := s.clientService.UpdateByEmail(&s.inboundService, p.TargetRef, updated); err != nil {
			return nil, err
		}

	default:
		return nil, common.NewError("unsupported targetType:", p.TargetType)
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
		CreatedAt:    time.Now().UnixMilli(),
	}
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}
