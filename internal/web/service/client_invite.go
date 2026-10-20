package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

var (
	// ErrInviteNotFound is returned when the code does not exist.
	ErrInviteNotFound = errors.New("invite not found")
	// ErrInviteExpired is returned when the code passed its deadline.
	ErrInviteExpired = errors.New("invite expired")
	// ErrInviteUsed is returned when the code was already redeemed.
	ErrInviteUsed = errors.New("invite already used")
)

// InviteLifetime is how long a generated invite stays redeemable.
const InviteLifetime = 24 * time.Hour

// ClientInviteService manages per-client Telegram invite links.
type ClientInviteService struct{}

// CreateInvite generates a fresh single-use code for the given client email.
// Codes are 128-bit random hex values that expire after InviteLifetime.
func (s *ClientInviteService) CreateInvite(email string) (string, int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", 0, ErrInviteNotFound
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", 0, err
	}
	code := hex.EncodeToString(buf[:])
	expiresAt := time.Now().Add(InviteLifetime).UnixMilli()
	row := model.ClientInvite{
		Code:      code,
		Email:     email,
		ExpiresAt: expiresAt,
	}
	if err := database.GetDB().Create(&row).Error; err != nil {
		return "", 0, err
	}
	return code, expiresAt, nil
}

// RedeemInvite atomically claims the code for one Telegram user and returns
// the client email it belongs to. Exactly one caller can succeed: the update
// only matches rows whose used_at is still zero, and RowsAffected decides.
func (s *ClientInviteService) RedeemInvite(code string, tgUserId int64) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", ErrInviteNotFound
	}
	db := database.GetDB()
	var invite model.ClientInvite
	if err := db.Where("code = ?", code).First(&invite).Error; err != nil {
		return "", ErrInviteNotFound
	}
	now := time.Now().UnixMilli()
	res := db.Model(&model.ClientInvite{}).
		Where("id = ? AND used_at = 0 AND expires_at > ?", invite.Id, now).
		Updates(map[string]any{"used_at": now, "used_by": tgUserId})
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected != 1 {
		// Lost the race or the row was already used/expired between the two
		// statements: distinguish the reason for a clear bot answer.
		var fresh model.ClientInvite
		if err := db.Where("code = ?", code).First(&fresh).Error; err != nil {
			return "", ErrInviteNotFound
		}
		if fresh.UsedAt != 0 {
			return "", ErrInviteUsed
		}
		return "", ErrInviteExpired
	}
	return invite.Email, nil
}

// RevokeInvitesForEmail invalidates every unused invite of a client. Used
// when the admin wants to cancel previously shared links (e.g. after sharing
// to the wrong person).
func (s *ClientInviteService) RevokeInvitesForEmail(email string) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, nil
	}
	res := database.GetDB().Model(&model.ClientInvite{}).
		Where("email = ? AND used_at = 0", email).
		Updates(map[string]any{"used_at": time.Now().UnixMilli(), "used_by": int64(0)})
	return res.RowsAffected, res.Error
}
