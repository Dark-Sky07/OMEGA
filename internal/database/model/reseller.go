package model

// Reseller is a panel sub-account (نمایندگی / نماینده) that manages a subset of
// the panel's inbounds and clients. A reseller signs in on the same panel with
// its own username/password, only ever sees assigned inbounds and explicitly
// mapped clients, and is bounded by two quotas: clients and total traffic. Inbounds are assigned by the admin and
// read-only for the reseller (an inbound_limit column lingering in upgraded
// databases is ignored).
//
// Ownership itself is stored in the mapping tables (ResellerInbound,
// ResellerClient) so the base inbound/client tables stay pristine and a single
// inbound can be handed over or taken back without touching its payload.
type Reseller struct {
	Id         int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Username   string `json:"username" form:"username" gorm:"uniqueIndex;not null" validate:"required" example:"reseller1"`
	Password   string `json:"-" form:"password" gorm:"not null"`
	Name       string `json:"name" form:"name" example:"Ali Reseller"`         // Display name shown in the panel
	Comment    string `json:"comment" form:"comment" example:"Telegram: @ali"` // Free-form note for the admin
	Enable     bool   `json:"enable" form:"enable" gorm:"default:true" example:"true"`
	LoginEpoch int64  `json:"-" gorm:"default:0"` // Bumped to invalidate existing sessions

	// Quotas. A zero value means "no limit".
	TrafficLimit int64 `json:"trafficLimit" form:"trafficLimit" gorm:"column:traffic_limit;default:0" example:"1099511627776"` // Bytes, cap on the sum of the reseller's client quotas
	ClientLimit  int   `json:"clientLimit" form:"clientLimit" gorm:"column:client_limit;default:0" example:"100"`
	ExpiryTime   int64 `json:"expiryTime" form:"expiryTime" gorm:"column:expiry_time;default:0" example:"1735689600000"` // Unix ms, 0 = never expires

	// Billing. PricePerGB is the rate the reseller is charged for every GB its
	// clients consume; Deposit is the prepaid credit on its account.
	PricePerGB float64 `json:"pricePerGb" form:"pricePerGb" gorm:"column:price_per_gb;default:0" example:"0.5"`
	Deposit    float64 `json:"deposit" form:"deposit" gorm:"default:0" example:"50"`

	CreatedAt int64 `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt int64 `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (Reseller) TableName() string { return "resellers" }

// ResellerInbound links an inbound to the reseller that owns it. This mapping
// grants access to the inbound only; it never grants access to clients living
// on it. Client visibility is represented separately by ResellerClient.
type ResellerInbound struct {
	ResellerId int   `json:"resellerId" gorm:"primaryKey;column:reseller_id;index"`
	InboundId  int   `json:"inboundId" gorm:"primaryKey;column:inbound_id;index"`
	CreatedAt  int64 `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (ResellerInbound) TableName() string { return "reseller_inbounds" }

// ResellerClient assigns a single client (by its unique email) to a reseller.
// It is used for clients that live on a shared inbound owned by the admin.
type ResellerClient struct {
	ResellerId int    `json:"resellerId" gorm:"primaryKey;column:reseller_id;index"`
	Email      string `json:"email" gorm:"primaryKey;column:email"`
	CreatedAt  int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (ResellerClient) TableName() string { return "reseller_clients" }

// ResellerTransaction is the reseller's money ledger: every deposit (charge)
// and settlement performed by the admin is appended here so the report can
// show how the current balance was reached.
type ResellerTransaction struct {
	Id         int     `json:"id" gorm:"primaryKey;autoIncrement"`
	ResellerId int     `json:"resellerId" gorm:"column:reseller_id;index;not null"`
	Type       string  `json:"type" gorm:"default:deposit" validate:"omitempty,oneof=deposit withdraw adjust"` // deposit | withdraw | adjust
	Amount     float64 `json:"amount"`
	Balance    float64 `json:"balance"` // Balance after the transaction
	Comment    string  `json:"comment"`
	CreatedAt  int64   `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (ResellerTransaction) TableName() string { return "reseller_transactions" }
