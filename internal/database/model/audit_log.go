package model

// AuditLog records security-sensitive mutations performed by admins, resellers,
// API tokens, or automated background watchdogs.
type AuditLog struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ActorType string `json:"actorType" gorm:"index;size:32"` // "admin", "reseller", "api_token", "system"
	ActorId   int    `json:"actorId" gorm:"index"`
	ActorName string `json:"actorName" gorm:"index;size:128"`
	Action    string `json:"action" gorm:"index;size:192"`
	Module    string `json:"module" gorm:"index;size:64"`
	Target    string `json:"target" gorm:"size:512"`
	IP        string `json:"ip" gorm:"size:64"`
	Status    int    `json:"status"`
	CreatedAt int64  `json:"createdAt" gorm:"index;autoCreateTime:milli"`
}

func (AuditLog) TableName() string { return "audit_logs" }
