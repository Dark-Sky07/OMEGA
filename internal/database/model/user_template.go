package model

// UserTemplate defines a reusable client plan preset (PasarGuard/Marzban-style
// User Template) that admins can apply in one click when creating single or
// bulk clients.
type UserTemplate struct {
	Id                 int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name               string `json:"name" gorm:"uniqueIndex;size:128;not null"`
	Description        string `json:"description" gorm:"size:512"`
	TotalGB            int64  `json:"totalGB"`            // Quota in bytes (0 = unlimited)
	ExpireDurationDays int    `json:"expireDurationDays"` // Duration in days (>0)
	DelayedStart       bool   `json:"delayedStart"`       // On-hold (start after first use)
	ActivateBeforeDays int    `json:"activateBeforeDays"` // Max days to first use when DelayedStart=true (0 = none)
	LimitIP            int    `json:"limitIp"`
	LimitDevice        int    `json:"limitDevice"`
	Reset              int    `json:"reset"`       // Renewal period in days
	RenewalMode        int    `json:"renewalMode"` // 0=rolling, 1=monthly, 2=weekly
	RenewalDay         int    `json:"renewalDay"`  // Day of month (1-31) or weekday (1-7)
	Group              string `json:"group" gorm:"size:128"`
	InboundIds         []int  `json:"inboundIds" gorm:"serializer:json"`
	CreatedAt          int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt          int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (UserTemplate) TableName() string { return "user_templates" }
