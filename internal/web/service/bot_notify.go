package service

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm/clause"
)

// Telegram bot approaching-limit warning kinds. A client can hold at most one
// claimed warning per kind per depletion episode.
const (
	BotNotifyKindTraffic = "traffic"
	BotNotifyKindExpire  = "expire"
)

// BotNotifyStateService tracks which approaching-limit warnings the Telegram
// bot already sent, so each client is warned exactly once per depletion
// episode instead of on every periodic report.
type BotNotifyStateService struct{}

// ClaimWarning atomically claims the warning slot for (email, kind). It
// returns true when this is the first claim of the episode (the caller should
// send the notification) and false when a warning was already sent. The
// insert is an INSERT .. ON CONFLICT DO NOTHING, so concurrent report runs
// can never double-send: exactly one claimer sees RowsAffected == 1.
func (s *BotNotifyStateService) ClaimWarning(email string, kind string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || kind == "" {
		return false
	}
	db := database.GetDB()
	row := model.BotNotifyState{
		Email:    email,
		Kind:     kind,
		WarnedAt: time.Now().UnixMilli(),
	}
	tx := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if tx.Error != nil {
		// On storage errors fall back to "already warned": losing one
		// notification is preferable to spamming every cycle.
		return false
	}
	return tx.RowsAffected == 1
}

// ResetWarning clears the episode marker for (email, kind) so a future
// approach to the limit can be announced again. Safe to call every cycle.
func (s *BotNotifyStateService) ResetWarning(email string, kind string) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || kind == "" {
		return
	}
	db := database.GetDB()
	db.Where("email = ? AND kind = ?", email, kind).Delete(&model.BotNotifyState{})
}

// ResetForEmail clears every warning marker for a client (both kinds). Used
// when a client is removed so a later client re-using the same email is
// treated as a fresh episode.
func (s *BotNotifyStateService) ResetForEmail(email string) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return
	}
	db := database.GetDB()
	db.Where("email = ?", email).Delete(&model.BotNotifyState{})
}
