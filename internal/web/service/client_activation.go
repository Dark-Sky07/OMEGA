package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// SweepActivateBefore converts delayed-start clients that never activated
// before their activation deadline (Client.ActivateBefore) into expired
// clients. A delayed-start client stores a negative expiryTime (the duration
// that will start on first use); once the deadline passes un-activated, the
// expiry is written as the deadline itself — already in the past — so the
// ordinary quota/expiry machinery (disable, exhausted listings, bot reports)
// treats it exactly like any other expired client.
//
// The conversion runs through ResetClientExpiryTimeByEmail, the canonical
// client-update path, so the clients table, client_traffics rows and inbound
// settings JSON all stay in sync.
//
// Returns the number of clients converted. Errors are logged and skipped per
// client so one bad row cannot block the rest.
func (s *ClientService) SweepActivateBefore(inboundSvc *InboundService) int {
	db := database.GetDB()
	now := time.Now().UnixMilli()

	var rows []model.ClientRecord
	err := db.Model(&model.ClientRecord{}).
		Where("expiry_time < 0 AND activate_before > 0 AND activate_before <= ?", now).
		Find(&rows).Error
	if err != nil {
		logger.Warning("activate-before sweep: load clients failed:", err)
		return 0
	}
	converted := 0
	for i := range rows {
		email := rows[i].Email
		if email == "" {
			continue
		}
		if _, uErr := s.ResetClientExpiryTimeByEmail(inboundSvc, email, rows[i].ActivateBefore); uErr != nil {
			logger.Warning("activate-before sweep: convert failed for", email, uErr)
			continue
		}
		converted++
	}
	if converted > 0 {
		logger.Debugf("activate-before sweep: %d delayed-start clients expired at their activation deadline", converted)
	}
	return converted
}
