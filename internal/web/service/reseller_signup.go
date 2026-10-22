package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

const resellerSignupTTL = 30 * 24 * time.Hour
const resellerSignupPurpose = "phoenix-reseller-signup-v1"

var (
	ErrResellerSignupLink = errors.New("invalid or expired reseller signup link")
	ErrResellerSignupPlan = errors.New("signup plan is unavailable")
)

type ResellerSignupInfo struct {
	ResellerName string `json:"resellerName"`
	BrandName    string `json:"brandName,omitempty"`
	BrandLogo    string `json:"brandLogo,omitempty"`
	BrandColor   string `json:"brandColor,omitempty"`
	BrandTheme   string `json:"brandTheme,omitempty"`
	PlanName     string `json:"planName"`
	Description  string `json:"description,omitempty"`
	TotalBytes   int64  `json:"totalBytes"`
	Days         int    `json:"days"`
}

type ResellerSignupService struct{}

// CreateLink creates a signed, expiring public registration token for a
// reseller-owned template. The template's inbound assignment is validated
// both here and on redemption so stale or edited plans cannot escape scope.
func (s *ResellerSignupService) CreateLink(resellerID, templateID int, secret []byte, now time.Time) (string, error) {
	if resellerID <= 0 || templateID <= 0 || len(secret) == 0 {
		return "", ErrResellerSignupLink
	}
	resellerSvc := &ResellerService{}
	reseller, err := resellerSvc.Get(resellerID)
	if err != nil {
		return "", err
	}
	if err := resellerSvc.EnsureActive(reseller); err != nil {
		return "", err
	}
	template, err := s.getTemplate(templateID)
	if err != nil {
		return "", err
	}
	if err := s.validateTemplateScope(resellerID, template); err != nil {
		return "", err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	payload := strings.Join([]string{
		strconv.Itoa(resellerID),
		strconv.Itoa(templateID),
		strconv.FormatInt(now.Add(resellerSignupTTL).Unix(), 10),
		base64.RawURLEncoding.EncodeToString(nonce[:]),
	}, ".")
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := signupMAC(secret, payload)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

func (s *ResellerSignupService) Info(token string, secret []byte, now time.Time) (*ResellerSignupInfo, error) {
	resellerID, templateID, err := parseResellerSignupToken(token, secret, now)
	if err != nil {
		return nil, err
	}
	resellerSvc := &ResellerService{}
	reseller, err := resellerSvc.Get(resellerID)
	if err != nil || resellerSvc.EnsureActive(reseller) != nil {
		return nil, ErrResellerSignupLink
	}
	template, err := s.getTemplate(templateID)
	if err != nil || s.validateTemplateScope(resellerID, template) != nil {
		return nil, ErrResellerSignupPlan
	}
	return &ResellerSignupInfo{
		ResellerName: firstNonEmptySignup(strings.TrimSpace(reseller.Name), reseller.Username),
		BrandName:    reseller.BrandName,
		BrandLogo:    reseller.BrandLogo,
		BrandColor:   reseller.BrandColor,
		BrandTheme:   reseller.BrandTheme,
		PlanName:     template.Name,
		Description:  template.Description,
		TotalBytes:   template.TotalGB,
		Days:         template.ExpireDurationDays,
	}, nil
}

// Register atomically checks current reseller limits, creates a client from
// the selected reusable plan, and assigns the new client to that reseller.
func (s *ResellerSignupService) Register(token, email string, secret []byte, now time.Time) (string, bool, error) {
	resellerID, templateID, err := parseResellerSignupToken(token, secret, now)
	if err != nil {
		return "", false, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 191 {
		return "", false, errors.New("a valid email or client identifier is required")
	}
	resellerSvc := &ResellerService{}
	reseller, err := resellerSvc.Get(resellerID)
	if err != nil {
		return "", false, ErrResellerSignupLink
	}
	if err := resellerSvc.EnsureActive(reseller); err != nil {
		return "", false, ErrResellerSignupLink
	}
	template, err := s.getTemplate(templateID)
	if err != nil || s.validateTemplateScope(resellerID, template) != nil {
		return "", false, ErrResellerSignupPlan
	}
	var clients ClientService
	if _, err := clients.GetRecordByEmail(nil, email); err == nil {
		return "", false, errors.New("this email is already registered")
	} else if !database.IsNotFound(err) {
		return "", false, err
	}
	if err := resellerSvc.CheckClientQuota(reseller, 1, template.TotalGB); err != nil {
		return "", false, err
	}

	client := model.Client{
		Email:       email,
		Enable:      true,
		Security:    "auto",
		TotalGB:     template.TotalGB,
		LimitIP:     template.LimitIP,
		LimitDevice: template.LimitDevice,
		Reset:       template.Reset,
		RenewalMode: template.RenewalMode,
		RenewalDay:  template.RenewalDay,
		Group:       template.Group,
	}
	if template.DelayedStart {
		client.ExpiryTime = -int64(template.ExpireDurationDays) * int64(24*time.Hour/time.Millisecond)
		if template.ActivateBeforeDays > 0 {
			client.ActivateBefore = now.Add(time.Duration(template.ActivateBeforeDays) * 24 * time.Hour).UnixMilli()
		}
	} else if template.ExpireDurationDays > 0 {
		client.ExpiryTime = now.Add(time.Duration(template.ExpireDurationDays) * 24 * time.Hour).UnixMilli()
	}

	var inboundSvc InboundService
	needRestart, err := clients.Create(&inboundSvc, &ClientCreatePayload{Client: client, InboundIds: template.InboundIds})
	if err != nil {
		return "", false, err
	}
	if err := resellerSvc.AssignClients(resellerID, []string{email}); err != nil {
		// Do not leave an unowned client behind if the visibility mapping fails.
		rollbackRestart, rollbackErr := clients.DeleteByEmail(&inboundSvc, email, false)
		if rollbackErr != nil {
			return "", needRestart || rollbackRestart, fmt.Errorf("client ownership failed: %w (rollback: %v)", err, rollbackErr)
		}
		return "", needRestart || rollbackRestart, err
	}
	created, err := clients.GetRecordByEmail(nil, email)
	if err != nil {
		return "", needRestart, err
	}
	return created.SubID, needRestart, nil
}

func (s *ResellerSignupService) EligibleTemplates(resellerID int) ([]model.UserTemplate, error) {
	resellerSvc := &ResellerService{}
	if _, err := resellerSvc.Get(resellerID); err != nil {
		return nil, err
	}
	inbounds, err := resellerSvc.OwnedInboundIdSet(resellerID)
	if err != nil {
		return nil, err
	}
	templates, err := (&UserTemplateService{}).List()
	if err != nil {
		return nil, err
	}
	eligible := make([]model.UserTemplate, 0, len(templates))
	for _, template := range templates {
		// Paid templates need a customer payment-order lifecycle before they can
		// be offered here; never silently grant a paid plan for free.
		if template.FixedPrice > 0 || len(template.InboundIds) == 0 {
			continue
		}
		valid := true
		for _, id := range template.InboundIds {
			if _, ok := inbounds[id]; !ok {
				valid = false
				break
			}
		}
		if valid {
			eligible = append(eligible, template)
		}
	}
	return eligible, nil
}

func (s *ResellerSignupService) getTemplate(id int) (*model.UserTemplate, error) {
	var template model.UserTemplate
	if err := database.GetDB().First(&template, id).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

func (s *ResellerSignupService) validateTemplateScope(resellerID int, template *model.UserTemplate) error {
	if template == nil || template.FixedPrice > 0 || template.TotalGB < 0 || template.TotalGB > 1<<60 ||
		template.ExpireDurationDays < 0 || template.ExpireDurationDays > 36500 ||
		template.ActivateBeforeDays < 0 || template.ActivateBeforeDays > 36500 ||
		len(template.InboundIds) == 0 {
		return ErrResellerSignupPlan
	}
	owned, err := (&ResellerService{}).OwnedInboundIdSet(resellerID)
	if err != nil {
		return err
	}
	for _, id := range template.InboundIds {
		if _, ok := owned[id]; !ok {
			return ErrResellerSignupPlan
		}
	}
	return nil
}

func parseResellerSignupToken(token string, secret []byte, now time.Time) (int, int, error) {
	if len(token) > 512 || len(secret) == 0 {
		return 0, 0, ErrResellerSignupLink
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || len(secret) == 0 {
		return 0, 0, ErrResellerSignupLink
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, 0, ErrResellerSignupLink
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(provided, signupMAC(secret, string(payloadBytes))) {
		return 0, 0, ErrResellerSignupLink
	}
	fields := strings.Split(string(payloadBytes), ".")
	if len(fields) != 4 {
		return 0, 0, ErrResellerSignupLink
	}
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(fields[3])
	if nonceErr != nil || len(nonce) != 16 {
		return 0, 0, ErrResellerSignupLink
	}
	resellerID, err1 := strconv.Atoi(fields[0])
	templateID, err2 := strconv.Atoi(fields[1])
	expiresAt, err3 := strconv.ParseInt(fields[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || resellerID <= 0 || templateID <= 0 || now.Unix() >= expiresAt {
		return 0, 0, ErrResellerSignupLink
	}
	return resellerID, templateID, nil
}

func signupMAC(secret []byte, payload string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(resellerSignupPurpose + "|" + payload))
	return mac.Sum(nil)
}

func firstNonEmptySignup(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fmt.Sprint("Reseller")
}
