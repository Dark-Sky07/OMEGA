package model

// ResellerVoucher represents a single-use or multi-use gift/voucher code
// that resellers can redeem to top up their wallet balance and/or traffic quota.
type ResellerVoucher struct {
	Id        int     `json:"id" gorm:"primaryKey;autoIncrement"`
	Code      string  `json:"code" gorm:"uniqueIndex;size:64;not null"`
	Amount    float64 `json:"amount" gorm:"not null"`
	TrafficGB int64   `json:"trafficGB" gorm:"default:0"`
	MaxUses   int     `json:"maxUses" gorm:"default:1"`
	UsedCount int     `json:"usedCount" gorm:"default:0"`
	UsedByIds []int   `json:"usedByIds" gorm:"serializer:json"`
	ExpiresAt int64   `json:"expiresAt" gorm:"default:0"`
	Note      string  `json:"note" gorm:"size:256"`
	CreatedAt int64   `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (ResellerVoucher) TableName() string { return "reseller_vouchers" }
