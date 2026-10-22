package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

// Reseller notification kinds. Each kind maps to a frontend i18n key under
// resellers.notif.<kind>. Keep the strings short: they double as the episode
// key stored in bot_notify_states.kind (column size 16).
const (
	ResellerNotifQuota80     = "quota80"
	ResellerNotifQuota95     = "quota95"
	ResellerNotifQuotaOut    = "quota_out"
	ResellerNotifClient80    = "climit80"
	ResellerNotifClientMax   = "climit_max"
	ResellerNotifExpirySoon  = "expiry_soon"
	ResellerNotifExpired     = "expired"
	resellerNotifyKeyPrefix  = "reseller:"
	resellerNotifyDefaultCap = 50
	resellerNotifyMaxCap     = 200
)

// Default thresholds preserve the existing reseller notification behavior.
const (
	ResellerQuotaWarnPercent  = 80
	ResellerQuotaCritPercent  = 95
	ResellerClientWarnPercent = 80
	ResellerExpiryWarningDays = 7
	resellerDayMilliseconds   = 24 * 60 * 60 * 1000
)

// ResellerNotifyConfig controls warning thresholds and delivery channels.
type ResellerNotifyConfig struct {
	TrafficWarningPercent  int
	TrafficCriticalPercent int
	ClientWarningPercent   int
	ExpiryWarningDays      int
	InPanelEnable          bool
	TelegramEnable         bool
}

func defaultResellerNotifyConfig() ResellerNotifyConfig {
	return ResellerNotifyConfig{
		TrafficWarningPercent:  ResellerQuotaWarnPercent,
		TrafficCriticalPercent: ResellerQuotaCritPercent,
		ClientWarningPercent:   ResellerClientWarnPercent,
		ExpiryWarningDays:      ResellerExpiryWarningDays,
		InPanelEnable:          true,
		TelegramEnable:         true,
	}
}

// Config reads the current thresholds and channels, falling back to safe
// legacy defaults if stored settings are unavailable or invalid.
func (s *ResellerNotifyService) Config() ResellerNotifyConfig {
	config := defaultResellerNotifyConfig()
	settings, err := (&SettingService{}).GetAllSetting()
	if err != nil || settings == nil {
		return config
	}
	candidate := ResellerNotifyConfig{
		TrafficWarningPercent:  settings.ResellerNotifTrafficWarningPercent,
		TrafficCriticalPercent: settings.ResellerNotifTrafficCriticalPercent,
		ClientWarningPercent:   settings.ResellerNotifClientWarningPercent,
		ExpiryWarningDays:      settings.ResellerNotifExpiryWarningDays,
		InPanelEnable:          settings.ResellerNotifInPanelEnable,
		TelegramEnable:         settings.ResellerNotifTelegramEnable,
	}
	if candidate.TrafficWarningPercent < 1 || candidate.TrafficWarningPercent > 98 ||
		candidate.TrafficCriticalPercent <= candidate.TrafficWarningPercent || candidate.TrafficCriticalPercent > 99 ||
		candidate.ClientWarningPercent < 1 || candidate.ClientWarningPercent > 99 ||
		candidate.ExpiryWarningDays < 1 || candidate.ExpiryWarningDays > 365 {
		return config
	}
	return candidate
}

// ResellerNotifyService writes and reads the in-panel notifications shown on
// a reseller's own dashboard. Episodes are de-duplicated through the shared
// BotNotifyState table using the synthetic email "reseller:<id>", which can
// never collide with a real client email.
type ResellerNotifyService struct{}

// ResellerNotifyClaimKey returns the synthetic bot_notify_states key that
// scopes warning episodes to one reseller.
func ResellerNotifyClaimKey(resellerId int) string {
	return fmt.Sprintf("%s%d", resellerNotifyKeyPrefix, resellerId)
}

func (s *ResellerNotifyService) claimEpisode(resellerId int, kind string) bool {
	notify := &BotNotifyStateService{}
	return notify.ClaimWarning(ResellerNotifyClaimKey(resellerId), kind)
}

func (s *ResellerNotifyService) resetEpisode(resellerId int, kinds ...string) {
	notify := &BotNotifyStateService{}
	for _, kind := range kinds {
		notify.ResetWarning(ResellerNotifyClaimKey(resellerId), kind)
	}
}

// Notify appends one notification row for the reseller and returns it.
// params may be nil.
func (s *ResellerNotifyService) Notify(resellerId int, kind string, params map[string]any) (*model.ResellerNotification, error) {
	if resellerId <= 0 || kind == "" {
		return nil, errors.New("reseller id and kind are required")
	}
	paramsJSON := "{}"
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsJSON = string(encoded)
	}
	row := &model.ResellerNotification{
		ResellerId: resellerId,
		Kind:       kind,
		Params:     paramsJSON,
	}
	db := database.GetDB()
	if err := db.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// Evaluate inspects one reseller's live usage snapshot and emits at most one
// notification per kind per depletion episode. Conditions that go away clear
// the episode marker so the reseller can be warned again on the next cycle.
// It is safe to call on every job run; the freshly created notifications are
// returned so callers (the notify job) can push them to Telegram.
func (s *ResellerNotifyService) Evaluate(stat *ResellerStat) []*model.ResellerNotification {
	return s.EvaluateWithConfig(stat, s.Config())
}

// EvaluateWithConfig evaluates one reseller using an already-loaded config so
// the periodic job reads settings once rather than once for every reseller.
func (s *ResellerNotifyService) EvaluateWithConfig(stat *ResellerStat, config ResellerNotifyConfig) []*model.ResellerNotification {
	fresh := make([]*model.ResellerNotification, 0)
	if stat == nil || stat.Reseller == nil || stat.Reseller.Id <= 0 || (!config.InPanelEnable && !config.TelegramEnable) {
		return fresh
	}
	emit := func(kind string, params map[string]any) {
		paramsJSON := "{}"
		if params != nil {
			encoded, err := json.Marshal(params)
			if err != nil {
				return
			}
			paramsJSON = string(encoded)
		}
		row := &model.ResellerNotification{
			ResellerId: stat.Reseller.Id,
			Kind:       kind,
			Params:     paramsJSON,
			CreatedAt:  time.Now().UnixMilli(),
		}
		if config.InPanelEnable {
			if persisted, err := s.Notify(stat.Reseller.Id, kind, params); err == nil {
				row = persisted
			}
		}
		fresh = append(fresh, row)
	}
	reseller := stat.Reseller
	id := reseller.Id

	// Traffic quota: percentage of the limit already allocated to clients.
	if reseller.TrafficLimit > 0 {
		pct := int(stat.AllocatedTraffic * 100 / reseller.TrafficLimit)
		quotaOut := stat.AllocatedTraffic > reseller.TrafficLimit || stat.UsedTraffic >= reseller.TrafficLimit
		switch {
		case quotaOut:
			if s.claimEpisode(id, ResellerNotifQuotaOut) {
				emit(ResellerNotifQuotaOut, map[string]any{"pct": pct})
			}
		case pct >= config.TrafficCriticalPercent:
			if s.claimEpisode(id, ResellerNotifQuota95) {
				emit(ResellerNotifQuota95, map[string]any{"pct": pct})
			}
		case pct >= config.TrafficWarningPercent:
			if s.claimEpisode(id, ResellerNotifQuota80) {
				emit(ResellerNotifQuota80, map[string]any{"pct": pct})
			}
		default:
			s.resetEpisode(id, ResellerNotifQuota80, ResellerNotifQuota95, ResellerNotifQuotaOut)
		}
	}

	// Client count quota.
	if reseller.ClientLimit > 0 {
		pct := stat.ClientCount * 100 / reseller.ClientLimit
		switch {
		case stat.ClientCount >= reseller.ClientLimit:
			if s.claimEpisode(id, ResellerNotifClientMax) {
				emit(ResellerNotifClientMax, map[string]any{
					"count": stat.ClientCount,
					"limit": reseller.ClientLimit,
				})
			}
		case pct >= config.ClientWarningPercent:
			if s.claimEpisode(id, ResellerNotifClient80) {
				emit(ResellerNotifClient80, map[string]any{"pct": pct})
			}
		default:
			s.resetEpisode(id, ResellerNotifClient80, ResellerNotifClientMax)
		}
	}

	// Reseller account expiry.
	if reseller.ExpiryTime > 0 {
		switch {
		case IsExpired(reseller.ExpiryTime):
			if s.claimEpisode(id, ResellerNotifExpired) {
				emit(ResellerNotifExpired, nil)
			}
		case reseller.ExpiryTime <= time.Now().UnixMilli()+int64(config.ExpiryWarningDays)*resellerDayMilliseconds:
			days := int((reseller.ExpiryTime - time.Now().UnixMilli()) / resellerDayMilliseconds)
			if s.claimEpisode(id, ResellerNotifExpirySoon) {
				emit(ResellerNotifExpirySoon, map[string]any{"days": days})
			}
		default:
			s.resetEpisode(id, ResellerNotifExpirySoon, ResellerNotifExpired)
		}
	}
	return fresh
}

// List returns the newest notifications for the reseller together with the
// unread count. limit <= 0 falls back to the default cap and values above
// resellerNotifyMaxCap are clamped so a caller can never force a huge
// allocation.
func (s *ResellerNotifyService) List(resellerId int, limit int) ([]*model.ResellerNotification, int64, error) {
	if resellerId <= 0 {
		return nil, 0, errors.New("reseller id is required")
	}
	if limit <= 0 {
		limit = resellerNotifyDefaultCap
	}
	if limit > resellerNotifyMaxCap {
		limit = resellerNotifyMaxCap
	}
	db := database.GetDB()
	var unread int64
	if err := db.Model(model.ResellerNotification{}).
		Where("reseller_id = ? AND is_read = ?", resellerId, false).
		Count(&unread).Error; err != nil {
		return nil, 0, err
	}
	// No capacity hint: limit originates from a request query parameter, and
	// the row count is bounded (<= resellerNotifyMaxCap) anyway.
	rows := make([]*model.ResellerNotification, 0)
	if err := db.Where("reseller_id = ?", resellerId).
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, unread, nil
}

// MarkRead flags one notification (id > 0) or every notification (id == 0)
// of the reseller as read.
func (s *ResellerNotifyService) MarkRead(resellerId int, id int) error {
	if resellerId <= 0 {
		return errors.New("reseller id is required")
	}
	db := database.GetDB()
	query := db.Model(model.ResellerNotification{}).
		Where("reseller_id = ? AND is_read = ?", resellerId, false)
	if id > 0 {
		query = query.Where("id = ?", id)
	}
	return query.Update("is_read", true).Error
}

// PurgeRead removes already-read notifications older than the given number
// of days so the table stays small on long-lived panels.
func (s *ResellerNotifyService) PurgeRead(olderThanDays int) error {
	if olderThanDays <= 0 {
		return errors.New("days must be positive")
	}
	cutoff := time.Now().UnixMilli() - int64(olderThanDays)*resellerDayMilliseconds
	db := database.GetDB()
	return db.Where("is_read = ? AND created_at < ?", true, cutoff).
		Delete(model.ResellerNotification{}).Error
}
