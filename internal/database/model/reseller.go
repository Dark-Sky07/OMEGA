package model

// Reseller is deliberately separate from User: upstream User currently grants
// full administrator privileges. No reseller may be inserted into that table.
// These additive tables are preparatory; authentication is not enabled yet.
type Reseller struct {
	Id int `json:"id" gorm:"primaryKey;autoIncrement"`
	Username string `json:"username" gorm:"uniqueIndex;not null"`
	PasswordHash string `json:"-" gorm:"not null"`
	Enabled bool `json:"enabled"`
	ExpiresAt int64 `json:"expiresAt"`
	MaxBytes int64 `json:"maxBytes"`
	MaxClients int64 `json:"maxClients"`
	LoginEpoch int64 `json:"-" gorm:"default:0"`
}

// ResellerInbound grants access without changing ownership of an inbound.
type ResellerInbound struct {
	ResellerID int `json:"resellerId" gorm:"primaryKey;autoIncrement:false"`
	InboundID int `json:"inboundId" gorm:"primaryKey;autoIncrement:false"`
}

// ResellerClient records ownership by immutable database ID, not a mutable
// email, subscription ID or group name. One client has at most one reseller.
// Allocation reservation/recovery must be implemented before exposing CRUD.
type ResellerClient struct {
	ClientID int `json:"clientId" gorm:"primaryKey;autoIncrement:false"`
	ResellerID int `json:"resellerId" gorm:"index;not null"`
}
