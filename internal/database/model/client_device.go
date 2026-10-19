package model

// ClientDevice records one HWID-identified device that fetched a client's
// subscription while a per-client device limit (Client.LimitDevice) is
// active. Devices are tracked only for clients with LimitDevice > 0, so
// deployments that never enable the limit accumulate no rows and see no
// behavior change.
type ClientDevice struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Email      string `json:"email" gorm:"uniqueIndex:idx_client_devices_email_hwid;size:191;not null"`
	Hwid       string `json:"hwid" gorm:"uniqueIndex:idx_client_devices_email_hwid;size:128;not null"`
	IP         string `json:"ip" gorm:"column:ip;size:64"`
	UserAgent  string `json:"userAgent" gorm:"column:user_agent;size:255"`
	CreatedAt  int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	LastSeenAt int64  `json:"lastSeenAt" gorm:"autoUpdateTime:milli"`
}

func (ClientDevice) TableName() string { return "client_devices" }
