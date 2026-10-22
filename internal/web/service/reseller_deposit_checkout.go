package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/util/common"
	"github.com/Dark-Sky07/phoenix/internal/util/currency"
	"github.com/Dark-Sky07/phoenix/internal/util/random"
)

const resellerDepositLinkTTL = 365 * 24 * time.Hour
const resellerDepositTokenPurpose = "phoenix-reseller-deposit-v1"
const resellerDepositOrderPrefix = "rd_"
const resellerDepositMaxAmount = 1000000.0
const resellerDepositMaxPending = 10

var ErrResellerDepositLink = errors.New("invalid or expired reseller deposit link")

type ResellerDepositCheckoutService struct{}

type ResellerDepositInfo struct {
	ResellerName string `json:"resellerName"`
	BrandName    string `json:"brandName,omitempty"`
	BrandLogo    string `json:"brandLogo,omitempty"`
	BrandColor   string `json:"brandColor,omitempty"`
	Currency     string `json:"currency"`
}

type ResellerDepositOrder struct {
	OrderId    string  `json:"orderId"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	GatewayURL string  `json:"gatewayUrl,omitempty"`
	PayInfo    string  `json:"payInfo,omitempty"`
}

func (s *ResellerDepositCheckoutService) CreateLink(resellerID int, secret []byte, now time.Time) (string, error) {
	if resellerID <= 0 || len(secret) == 0 {
		return "", ErrResellerDepositLink
	}
	resellerSvc := &ResellerService{}
	reseller, err := resellerSvc.Get(resellerID)
	if err != nil {
		return "", err
	}
	if err := resellerSvc.EnsureActive(reseller); err != nil {
		return "", err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	payload := strings.Join([]string{
		strconv.Itoa(resellerID),
		strconv.FormatInt(now.Add(resellerDepositLinkTTL).Unix(), 10),
		base64.RawURLEncoding.EncodeToString(nonce[:]),
	}, ".")
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(resellerDepositMAC(secret, payload)), nil
}

func (s *ResellerDepositCheckoutService) Info(token string, secret []byte, now time.Time) (*ResellerDepositInfo, error) {
	var settingSvc SettingService
	if enabled, err := settingSvc.GetCheckoutEnable(); err != nil || !enabled {
		return nil, ErrCheckoutDisabled
	}
	reseller, err := s.resolveReseller(token, secret, now)
	if err != nil {
		return nil, err
	}
	cfg, err := loadCheckoutConfig()
	if err != nil {
		return nil, err
	}
	return &ResellerDepositInfo{
		ResellerName: firstNonEmptySignup(strings.TrimSpace(reseller.Name), reseller.Username),
		BrandName:    reseller.BrandName,
		BrandLogo:    reseller.BrandLogo,
		BrandColor:   reseller.BrandColor,
		Currency:     cfg.currency,
	}, nil
}

func (s *ResellerDepositCheckoutService) CreateOrder(token string, amount float64, secret []byte, now time.Time) (*ResellerDepositOrder, error) {
	var settingSvc SettingService
	if enabled, err := settingSvc.GetCheckoutEnable(); err != nil || !enabled {
		return nil, ErrCheckoutDisabled
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount > resellerDepositMaxAmount {
		return nil, common.NewError("amount must be greater than 0 and no more than ", resellerDepositMaxAmount)
	}
	reseller, err := s.resolveReseller(token, secret, now)
	if err != nil {
		return nil, err
	}
	cfg, err := loadCheckoutConfig()
	if err != nil {
		return nil, err
	}
	amount = RoundMoney(amount, cfg.currency)
	if amount <= 0 {
		return nil, common.NewError("amount is below the minimum unit for ", cfg.currency)
	}
	db := database.GetDB()
	var pending int64
	if err := db.Model(&model.PaymentOrder{}).
		Where("target_ref = ? AND target_type = ? AND status = ?", reseller.Username, "reseller_deposit", "pending").
		Count(&pending).Error; err != nil {
		return nil, err
	}
	if pending >= resellerDepositMaxPending {
		return nil, common.NewError("too many unpaid orders, please complete or wait for the previous ones")
	}
	order := &model.PaymentOrder{
		OrderId:    resellerDepositOrderPrefix + random.Seq(16),
		Gateway:    "checkout",
		TargetType: "reseller_deposit",
		TargetRef:  reseller.Username,
		Amount:     amount,
		Status:     "pending",
	}
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}
	out := &ResellerDepositOrder{OrderId: order.OrderId, Amount: amount, Currency: cfg.currency, PayInfo: cfg.payInfo}
	if cfg.gatewayURL != "" {
		out.GatewayURL = strings.NewReplacer(
			"{orderId}", order.OrderId,
			"{amount}", strconv.FormatFloat(amount, 'f', currency.FractionDigits(cfg.currency), 64),
			"{ref}", reseller.Username,
		).Replace(cfg.gatewayURL)
	}
	if out.GatewayURL == "" && out.PayInfo == "" {
		out.PayInfo = "Contact your provider to complete the payment."
	}
	return out, nil
}

func (s *ResellerDepositCheckoutService) Status(token, orderID string, secret []byte, now time.Time) (string, error) {
	reseller, err := s.resolveReseller(token, secret, now)
	if err != nil {
		return "", err
	}
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return "", common.NewError("orderId is required")
	}
	var order model.PaymentOrder
	if err := database.GetDB().Where("order_id = ?", orderID).First(&order).Error; err != nil {
		return "unknown", nil
	}
	if order.TargetType != "reseller_deposit" || order.TargetRef != reseller.Username {
		return "unknown", nil
	}
	return order.Status, nil
}

func (s *ResellerDepositCheckoutService) resolveReseller(token string, secret []byte, now time.Time) (*model.Reseller, error) {
	resellerID, err := parseResellerDepositToken(token, secret, now)
	if err != nil {
		return nil, err
	}
	resellerSvc := &ResellerService{}
	reseller, err := resellerSvc.Get(resellerID)
	if err != nil || resellerSvc.EnsureActive(reseller) != nil {
		return nil, ErrResellerDepositLink
	}
	return reseller, nil
}

func parseResellerDepositToken(token string, secret []byte, now time.Time) (int, error) {
	if len(token) > 512 || len(secret) == 0 {
		return 0, ErrResellerDepositLink
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return 0, ErrResellerDepositLink
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, ErrResellerDepositLink
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(provided, resellerDepositMAC(secret, string(payload))) {
		return 0, ErrResellerDepositLink
	}
	fields := strings.Split(string(payload), ".")
	if len(fields) != 3 {
		return 0, ErrResellerDepositLink
	}
	resellerID, idErr := strconv.Atoi(fields[0])
	expiresAt, expiryErr := strconv.ParseInt(fields[1], 10, 64)
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(fields[2])
	if idErr != nil || expiryErr != nil || nonceErr != nil || resellerID <= 0 || len(nonce) != 16 || now.Unix() >= expiresAt {
		return 0, ErrResellerDepositLink
	}
	return resellerID, nil
}

func resellerDepositMAC(secret []byte, payload string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(resellerDepositTokenPurpose + "|" + payload))
	return mac.Sum(nil)
}
