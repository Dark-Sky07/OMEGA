package model

// BotNotifyState records, per client email and warning kind ("traffic" or
// "expire"), that the Telegram bot already sent an approaching-limit warning
// for the current depletion episode. The row is claimed exactly once per
// episode and cleared as soon as the condition goes away, so a client that
// approaches a limit is notified once (not on every report cycle) and can be
// notified again after renewing and re-approaching a limit.
//
// The table only grows with warned clients and is fully additive: deployments
// whose admins never set the notification thresholds see no rows and no
// behavior change beyond the de-duplication itself.
type BotNotifyState struct {
	Email    string `json:"email" gorm:"primaryKey;size:191;not null"`
	Kind     string `json:"kind" gorm:"primaryKey;size:16;not null"`
	WarnedAt int64  `json:"warnedAt"`
}

func (BotNotifyState) TableName() string { return "bot_notify_states" }
