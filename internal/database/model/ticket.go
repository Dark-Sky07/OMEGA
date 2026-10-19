package model

// SupportTicket represents a support thread opened by a reseller for the panel admin.
type SupportTicket struct {
	Id               int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ResellerId       int    `json:"resellerId" gorm:"column:reseller_id;index;not null"`
	ResellerUsername string `json:"resellerUsername" gorm:"column:reseller_username"`
	Subject          string `json:"subject"`
	Status           string `json:"status" gorm:"default:open;index"` // open | answered | closed
	Priority         string `json:"priority" gorm:"default:normal"`   // low | normal | high
	CreatedAt        int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt        int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (SupportTicket) TableName() string { return "support_tickets" }

// SupportTicketMessage is an individual reply in a SupportTicket conversation.
type SupportTicketMessage struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	TicketId   int    `json:"ticketId" gorm:"column:ticket_id;index;not null"`
	SenderRole string `json:"senderRole" gorm:"column:sender_role"` // reseller | admin
	SenderName string `json:"senderName" gorm:"column:sender_name"`
	Message    string `json:"message"`
	CreatedAt  int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (SupportTicketMessage) TableName() string { return "support_ticket_messages" }

// PaymentOrder records an idempotent external payment/order webhook event that
// either tops up a reseller's prepaid deposit or renews/extends a client.
type PaymentOrder struct {
	Id           int     `json:"id" gorm:"primaryKey;autoIncrement"`
	OrderId      string  `json:"orderId" gorm:"column:order_id;uniqueIndex;not null"`
	Gateway      string  `json:"gateway"`
	TargetType   string  `json:"targetType" gorm:"column:target_type"` // reseller_deposit | client_renew
	TargetRef    string  `json:"targetRef" gorm:"column:target_ref"`   // reseller username or client email
	Amount       float64 `json:"amount"`
	AddGB        int64   `json:"addGb" gorm:"column:add_gb"`
	AddDays      int     `json:"addDays" gorm:"column:add_days"`
	ResetTraffic bool    `json:"resetTraffic" gorm:"column:reset_traffic"`
	Status       string  `json:"status" gorm:"default:completed"`
	CreatedAt    int64   `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (PaymentOrder) TableName() string { return "payment_orders" }
