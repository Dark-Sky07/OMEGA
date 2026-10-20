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
	// ErrResellerTgLinkNotFound is returned when the code does not exist or
	// was already redeemed (redeeming clears it).
	ErrResellerTgLinkNotFound = errors.New("reseller link code not found")
	// ErrResellerTgLinkExpired is returned when the code passed its deadline.
	ErrResellerTgLinkExpired = errors.New("reseller link code expired")
	// ErrResellerTgAlreadyLinked is returned when the Telegram account is
	// already bound to a different reseller.
	ErrResellerTgAlreadyLinked = errors.New("telegram account already linked to another reseller")
)

// ResellerTgLinkLifetime is how long a generated reseller link code stays
// redeemable.
const ResellerTgLinkLifetime = 15 * time.Minute

// ResellerTelegramService manages the link between a reseller account and a
// Telegram user: the reseller generates a one-time code from their own
// profile page, then sends "/start reseller_<code>" to the panel bot.
type ResellerTelegramService struct{}

// CreateLinkCode generates a fresh one-time code for the reseller. Any
// previously issued code is invalidated. Codes are 64-bit random hex values.
func (s *ResellerTelegramService) CreateLinkCode(resellerId int) (string, int64, error) {
	if resellerId <= 0 {
		return "", 0, errors.New("reseller id is required")
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", 0, err
	}
	code := hex.EncodeToString(buf[:])
	expiresAt := time.Now().Add(ResellerTgLinkLifetime).UnixMilli()
	db := database.GetDB()
	if err := db.Model(model.Reseller{}).
		Where("id = ?", resellerId).
		Updates(map[string]any{
			"tg_code":        code,
			"tg_code_expiry": expiresAt,
		}).Error; err != nil {
		return "", 0, err
	}
	return code, expiresAt, nil
}

// RedeemLinkCode atomically claims the code for one Telegram user and returns
// the freshly linked reseller. Rebinding the same Telegram account to the
// same reseller is allowed; binding it to a second reseller is not. The
// update only matches the row while the code is still set and unexpired, so
// exactly one concurrent caller can win.
func (s *ResellerTelegramService) RedeemLinkCode(code string, tgUserId int64) (*model.Reseller, error) {
	code = strings.TrimSpace(code)
	if code == "" || tgUserId == 0 {
		return nil, ErrResellerTgLinkNotFound
	}
	db := database.GetDB()

	var owner model.Reseller
	if err := db.Where("tg_code = ?", code).First(&owner).Error; err != nil {
		return nil, ErrResellerTgLinkNotFound
	}

	// One Telegram account may only drive one reseller account.
	var clash model.Reseller
	if err := db.Where("tg_id = ? AND id <> ?", tgUserId, owner.Id).First(&clash).Error; err == nil {
		return nil, ErrResellerTgAlreadyLinked
	}

	res := db.Model(model.Reseller{}).
		Where("id = ? AND tg_code = ? AND tg_code_expiry > ?", owner.Id, code, time.Now().UnixMilli()).
		Updates(map[string]any{
			"tg_id":          tgUserId,
			"tg_code":        "",
			"tg_code_expiry": 0,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, ErrResellerTgLinkExpired
	}

	owner.TgID = tgUserId
	owner.TgCode = ""
	owner.TgCodeExpiry = 0
	return &owner, nil
}

// Unlink detaches the reseller's Telegram account.
func (s *ResellerTelegramService) Unlink(resellerId int) error {
	if resellerId <= 0 {
		return errors.New("reseller id is required")
	}
	db := database.GetDB()
	return db.Model(model.Reseller{}).
		Where("id = ?", resellerId).
		Updates(map[string]any{
			"tg_id":          0,
			"tg_code":        "",
			"tg_code_expiry": 0,
		}).Error
}

// ResellerByTgID returns the enabled reseller linked to a Telegram chat id.
// Disabled resellers are treated as unlinked so the bot stops serving them.
func (s *ResellerTelegramService) ResellerByTgID(tgUserId int64) (*model.Reseller, error) {
	if tgUserId == 0 {
		return nil, ErrResellerTgLinkNotFound
	}
	var reseller model.Reseller
	if err := database.GetDB().
		Where("tg_id = ? AND enable = ?", tgUserId, true).
		First(&reseller).Error; err != nil {
		return nil, err
	}
	return &reseller, nil
}
