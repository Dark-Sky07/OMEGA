package model

// ClientInvite is a single-use, time-limited Telegram invite code bound to
// one client email. The admin generates a link per client; the customer opens
// it in Telegram and their account is bound to that client (TgID), giving
// them self-service access (usage, subscription links, QR) without the admin
// having to look up numeric Telegram IDs.
//
// Codes are random 128-bit hex strings: unguessable and URL-safe. Redemption
// is an atomic claim (used_at must still be zero), so a code can never be
// redeemed twice even under concurrent requests.
type ClientInvite struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Code      string `json:"code" gorm:"uniqueIndex:idx_client_invites_code;size:64;not null"`
	Email     string `json:"email" gorm:"index:idx_client_invites_email;size:191;not null"`
	CreatedAt int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	ExpiresAt int64  `json:"expiresAt"`
	UsedAt    int64  `json:"usedAt"`
	UsedBy    int64  `json:"usedBy"`
}

func (ClientInvite) TableName() string { return "client_invites" }
