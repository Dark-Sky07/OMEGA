package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/util/common"
	"github.com/Dark-Sky07/phoenix/internal/util/random"
)

var (
	// ErrCheckoutDisabled is returned while the checkout page is turned off.
	ErrCheckoutDisabled = errors.New("checkout is disabled")
	// ErrCheckoutInvalidRef is returned for malformed or tampered link tokens.
	ErrCheckoutInvalidRef = errors.New("invalid checkout link")
	// ErrCheckoutExpiredRef is returned for link tokens past their deadline.
	ErrCheckoutExpiredRef = errors.New("checkout link expired")
)

const (
	// checkoutTokenPurpose binds checkout tokens to this feature so the panel
	// secret can never be replayed across unrelated HMAC uses.
	checkoutTokenPurpose = "phoenix-checkout-v1"

	// checkoutRefTTL bounds how long a signed client link stays usable.
	checkoutRefTTL = 365 * 24 * time.Hour

	// One order may add at most this much traffic / validity at once.
	checkoutMaxGB     = 10000
	checkoutMaxMonths = 36

	// checkoutMaxPendingOrders bounds open (unpaid) orders per client email.
	checkoutMaxPendingOrders = 10

	// checkoutOrderPrefix marks order ids minted by the checkout page.
	checkoutOrderPrefix = "co_"
)

// CheckoutService serves the public client checkout page: it validates signed
// per-client links, prices plans from the panel settings and records pending
// client_renew orders that the payment webhook completes.
type CheckoutService struct {
	clientService   ClientService
	resellerService ResellerService
}

// CheckoutInfo is the public state of one client's checkout page.
type CheckoutInfo struct {
	Email         string  `json:"email"`
	Enabled       bool    `json:"enabled"`
	ExpiryTime    int64   `json:"expiryTime"`
	TotalBytes    int64   `json:"totalBytes"` // quota, 0 = unlimited
	Up            int64   `json:"up"`
	Down          int64   `json:"down"`
	Currency      string  `json:"currency"`
	PricePerGB    float64 `json:"pricePerGb"`
	PricePerMonth float64 `json:"pricePerMonth"`
}

// CheckoutOrder is the result of creating a checkout order.
type CheckoutOrder struct {
	OrderId    string  `json:"orderId"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	GatewayURL string  `json:"gatewayUrl,omitempty"`
	PayInfo    string  `json:"payInfo,omitempty"`
}

// SignCheckoutRef builds a tamper-proof, expiring link token for one client.
func SignCheckoutRef(email string, secret []byte, now time.Time) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", errors.New("email is required")
	}
	payload := email + "|" + strconv.FormatInt(now.Add(checkoutRefTTL).Unix(), 10)
	sig := checkoutRefMAC(payload, secret)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(sig), nil
}

// ParseCheckoutRef validates a checkout link token and returns the client
// email it was minted for.
func ParseCheckoutRef(token string, secret []byte, now time.Time) (string, error) {
	payload, sigHex, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || payload == "" || sigHex == "" {
		return "", ErrCheckoutInvalidRef
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", ErrCheckoutInvalidRef
	}
	email, expStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", ErrCheckoutInvalidRef
	}
	email = strings.ToLower(strings.TrimSpace(email))
	expiry, err := strconv.ParseInt(expStr, 10, 64)
	if email == "" || !strings.Contains(email, "@") || len(email) > 254 || err != nil {
		return "", ErrCheckoutInvalidRef
	}
	if now.Unix() >= expiry {
		return "", ErrCheckoutExpiredRef
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != 16 {
		return "", ErrCheckoutInvalidRef
	}
	if !hmac.Equal(sig, checkoutRefMAC(string(raw), secret)) {
		return "", ErrCheckoutInvalidRef
	}
	return email, nil
}

func checkoutRefMAC(payload string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(checkoutTokenPurpose + "|" + payload))
	return mac.Sum(nil)[:16]
}

// checkoutConfig reads the pricing settings once.
type checkoutConfig struct {
	currency    string
	pricePerGB  float64
	pricePerMon float64
	gatewayURL  string
	payInfo     string
}

func loadCheckoutConfig() (*checkoutConfig, error) {
	var settingSvc SettingService
	currency, err := settingSvc.GetCheckoutCurrency()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(currency) == "" {
		currency = defaultValueMap["checkoutCurrency"]
	}
	pricePerGB, err := settingSvc.GetCheckoutPricePerGB()
	if err != nil {
		return nil, err
	}
	pricePerMon, err := settingSvc.GetCheckoutPricePerMonth()
	if err != nil {
		return nil, err
	}
	gatewayURL, err := settingSvc.GetCheckoutGatewayURL()
	if err != nil {
		return nil, err
	}
	payInfo, err := settingSvc.GetCheckoutPayInfo()
	if err != nil {
		return nil, err
	}
	return &checkoutConfig{
		currency:    strings.TrimSpace(currency),
		pricePerGB:  pricePerGB,
		pricePerMon: pricePerMon,
		gatewayURL:  strings.TrimSpace(gatewayURL),
		payInfo:     strings.TrimSpace(payInfo),
	}, nil
}

// Info renders the client's public checkout page state.
func (s *CheckoutService) Info(email string) (*CheckoutInfo, error) {
	var settingSvc SettingService
	if enabled, err := settingSvc.GetCheckoutEnable(); err != nil || !enabled {
		return nil, ErrCheckoutDisabled
	}
	cfg, err := loadCheckoutConfig()
	if err != nil {
		return nil, err
	}
	rec, err := s.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return nil, ErrCheckoutInvalidRef
	}
	info := &CheckoutInfo{
		Email:         rec.Email,
		Enabled:       rec.Enable,
		ExpiryTime:    rec.ExpiryTime,
		TotalBytes:    rec.TotalGB,
		Currency:      cfg.currency,
		PricePerGB:    cfg.pricePerGB,
		PricePerMonth: cfg.pricePerMon,
	}
	usage, err := s.resellerService.ClientUsage([]string{rec.Email})
	if err == nil {
		if row := usage[strings.ToLower(rec.Email)]; row != nil {
			info.Up = row.Up
			info.Down = row.Down
		}
	}
	return info, nil
}

// CreateOrder prices the chosen plan and records a pending client_renew order.
// The payment webhook later flips it to completed and applies the effects.
func (s *CheckoutService) CreateOrder(email string, gb int64, months int) (*CheckoutOrder, error) {
	var settingSvc SettingService
	if enabled, err := settingSvc.GetCheckoutEnable(); err != nil || !enabled {
		return nil, ErrCheckoutDisabled
	}
	if gb < 0 || gb > checkoutMaxGB {
		return nil, common.NewError("traffic must be between 0 and ", checkoutMaxGB, " GB")
	}
	if months < 0 || months > checkoutMaxMonths {
		return nil, common.NewError("months must be between 0 and ", checkoutMaxMonths)
	}
	if gb == 0 && months == 0 {
		return nil, common.NewError("choose traffic or renewal months first")
	}
	cfg, err := loadCheckoutConfig()
	if err != nil {
		return nil, err
	}
	amount := Round2(float64(gb)*cfg.pricePerGB + float64(months)*cfg.pricePerMon)
	if amount <= 0 {
		return nil, common.NewError("checkout prices are not configured")
	}
	if _, err := s.clientService.GetRecordByEmail(nil, email); err != nil {
		return nil, ErrCheckoutInvalidRef
	}

	db := database.GetDB()
	var pending int64
	if err := db.Model(model.PaymentOrder{}).
		Where("target_ref = ? AND target_type = ? AND status = ?", email, "client_renew", "pending").
		Count(&pending).Error; err != nil {
		return nil, err
	}
	if pending >= checkoutMaxPendingOrders {
		return nil, common.NewError("too many unpaid orders, please complete or wait for the previous ones")
	}

	order := &model.PaymentOrder{
		OrderId:    checkoutOrderPrefix + random.Seq(16),
		Gateway:    "checkout",
		TargetType: "client_renew",
		TargetRef:  email,
		Amount:     amount,
		AddGB:      gb,
		AddDays:    months * 30,
		Status:     "pending",
	}
	if err := db.Create(order).Error; err != nil {
		return nil, err
	}

	out := &CheckoutOrder{
		OrderId: order.OrderId,
		Amount:  amount,
		PayInfo: cfg.payInfo,
	}
	if cfg.gatewayURL != "" {
		out.GatewayURL = strings.NewReplacer(
			"{orderId}", order.OrderId,
			"{amount}", strconv.FormatFloat(amount, 'f', -1, 64),
			"{ref}", email,
		).Replace(cfg.gatewayURL)
	}
	if out.GatewayURL == "" && out.PayInfo == "" {
		out.PayInfo = "Contact your provider to complete the payment."
	}
	return out, nil
}

// Status reports an order's state without leaking other clients' orders: an
// order that does not belong to the linked email reads as unknown.
func (s *CheckoutService) Status(email string, orderId string) (string, error) {
	orderId = strings.TrimSpace(orderId)
	if orderId == "" {
		return "", common.NewError("orderId is required")
	}
	var order model.PaymentOrder
	if err := database.GetDB().Where("order_id = ?", orderId).First(&order).Error; err != nil {
		return "unknown", nil
	}
	if order.TargetRef != email {
		return "unknown", nil
	}
	return order.Status, nil
}
