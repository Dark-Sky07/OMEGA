package model

// ResellerNotification is an in-panel alert addressed to one reseller. Rows
// are written by the periodic reseller notify job when a quota approaches its
// limit, gets exhausted, or the reseller account itself nears expiry. The
// title text is not stored: Kind plus the JSON params map let the frontend
// render the message in the reseller's own language. Kind is one of:
// quota80, quota95, quota_out, climit80, climit_max, expiry_soon, expired.
//
// De-duplication (one notification per depletion episode, not one per job
// cycle) is tracked in bot_notify_states using the synthetic key
// "reseller:<id>" so this table stays append-only and read-mostly.
type ResellerNotification struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ResellerId int    `json:"resellerId" gorm:"column:reseller_id;index;not null"`
	Kind       string `json:"kind" gorm:"size:64;not null"`
	Params     string `json:"params" gorm:"size:512"`
	IsRead     bool   `json:"isRead" gorm:"default:false;index"`
	CreatedAt  int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (ResellerNotification) TableName() string { return "reseller_notifications" }
