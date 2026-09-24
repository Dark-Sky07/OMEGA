package service

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

func (s *ClientService) ResetTrafficByEmail(inboundSvc *InboundService, email string) (bool, error) {
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	inboundIds, err := s.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return false, err
	}
	return s.resetTrafficForInboundIDs(inboundSvc, rec, email, inboundIds, false)
}

// ResetTrafficByEmailForInbounds scopes the runtime/stat reset to a client's
// complete association set. Because the traffic row is shared by canonical
// email, a client with any out-of-scope association is rejected as a no-op.
func (s *ClientService) ResetTrafficByEmailForInbounds(inboundSvc *InboundService, email string, inboundIDs []int) (bool, error) {
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	currentIDs, err := s.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return false, err
	}
	allowed := make(map[int]struct{}, len(inboundIDs))
	for _, id := range inboundIDs {
		if id > 0 {
			allowed[id] = struct{}{}
		}
	}
	scoped := make([]int, 0, len(currentIDs))
	for _, id := range currentIDs {
		if _, ok := allowed[id]; ok {
			scoped = append(scoped, id)
		}
	}
	if len(scoped) == 0 || len(scoped) != len(currentIDs) {
		// ClientTraffic is an email-level aggregate. Refuse a scoped reset
		// when the same canonical client is attached outside the supplied
		// tenant scope; otherwise resetting one tenant would erase another
		// tenant's usage too.
		return false, nil
	}
	return s.resetTrafficForInboundIDs(inboundSvc, rec, email, scoped, true)
}

func (s *ClientService) resetTrafficForInboundIDs(inboundSvc *InboundService, rec *model.ClientRecord, email string, inboundIDs []int, scoped bool) (bool, error) {
	needRestart := false
	if !rec.Enable {
		updated := rec.ToClient()
		updated.Enable = true
		var nr bool
		var uErr error
		if scoped {
			nr, uErr = s.UpdateByEmailForInbounds(inboundSvc, email, *updated, inboundIDs)
		} else {
			nr, uErr = s.Update(inboundSvc, rec.Id, *updated)
		}
		if uErr != nil {
			logger.Warning("Failed to auto-enable client during traffic reset:", uErr)
		}
		if nr {
			needRestart = true
		}
	}

	if len(inboundIDs) == 0 {
		if rErr := inboundSvc.ResetClientTrafficByEmail(email); rErr != nil {
			return false, rErr
		}
		return needRestart, nil
	}
	for _, ibId := range inboundIDs {
		nr, rErr := inboundSvc.ResetClientTraffic(ibId, email)
		if rErr != nil {
			return needRestart, rErr
		}
		if nr {
			needRestart = true
		}
	}
	return needRestart, nil
}

// BulkResetTrafficForInbounds resets only clients whose complete association
// set belongs to the supplied inbound scope. The aggregate traffic row is
// keyed by email in the legacy schema, so shared clients are deliberately
// skipped instead of allowing a reseller reset to erase another tenant's
// usage.
func (s *ClientService) BulkResetTrafficForInbounds(inboundSvc *InboundService, emails []string, inboundIDs []int) (int, bool, error) {
	if len(emails) == 0 || len(inboundIDs) == 0 {
		return 0, false, nil
	}
	seen := make(map[string]struct{}, len(emails))
	affected := 0
	needRestart := false
	for _, raw := range emails {
		email := transferEmailKey(raw)
		if email == "" {
			continue
		}
		if _, duplicate := seen[email]; duplicate {
			continue
		}
		seen[email] = struct{}{}
		nr, err := s.ResetTrafficByEmailForInbounds(inboundSvc, email, inboundIDs)
		if err != nil {
			return affected, needRestart, err
		}
		if nr {
			needRestart = true
		}
		// The scoped reset method is association-aware and is intentionally a
		// no-op for mapped clients that have no attachment in the scope.
		if rec, recErr := s.GetRecordByEmail(nil, email); recErr == nil && rec != nil {
			ids, idsErr := s.GetInboundIdsForRecord(rec.Id)
			if idsErr == nil && allInboundIDsInScope(ids, inboundIDs) {
				affected++
			}
		}
	}
	return affected, needRestart, nil
}

func allInboundIDsInScope(current, allowed []int) bool {
	if len(current) == 0 {
		return false
	}
	set := make(map[int]struct{}, len(allowed))
	for _, id := range allowed {
		set[id] = struct{}{}
	}
	for _, id := range current {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func (s *ClientService) BulkResetTraffic(inboundSvc *InboundService, emails []string) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}
	seen := map[string]struct{}{}
	cleanEmails := make([]string, 0, len(emails))
	for _, e := range emails {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		key := strings.ToLower(e)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleanEmails = append(cleanEmails, e)
	}
	if len(cleanEmails) == 0 {
		return 0, nil
	}

	for _, e := range cleanEmails {
		rec, err := s.GetRecordByEmail(nil, e)
		if err == nil && !rec.Enable {
			updated := rec.ToClient()
			updated.Enable = true
			s.Update(inboundSvc, rec.Id, *updated)
		}
	}

	affected := 0
	err := submitTrafficWrite(func() error {
		db := database.GetDB()
		return db.Transaction(func(tx *gorm.DB) error {
			for _, batch := range chunkStrings(cleanEmails, sqlInChunk) {
				res := tx.Model(xray.ClientTraffic{}).
					Where("LOWER(TRIM(email)) IN ?", batch).
					Updates(map[string]any{"enable": true, "up": 0, "down": 0})
				if res.Error != nil {
					return res.Error
				}
				affected += int(res.RowsAffected)
			}
			return clearGlobalTraffic(tx, cleanEmails...)
		})
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (s *ClientService) ResetAllClientTraffics(inboundSvc *InboundService, id int) error {
	return submitTrafficWrite(func() error {
		return s.resetAllClientTrafficsLocked(id)
	})
}

func (s *ClientService) resetAllClientTrafficsLocked(id int) error {
	db := database.GetDB()
	now := time.Now().Unix() * 1000

	if err := db.Transaction(func(tx *gorm.DB) error {
		whereText := "inbound_id "
		if id == -1 {
			whereText += " > ?"
		} else {
			whereText += " = ?"
		}

		var resetEmails []string
		if err := tx.Model(xray.ClientTraffic{}).
			Where(whereText, id).
			Pluck("email", &resetEmails).Error; err != nil {
			return err
		}

		result := tx.Model(xray.ClientTraffic{}).
			Where(whereText, id).
			Updates(map[string]any{"enable": true, "up": 0, "down": 0})

		if result.Error != nil {
			return result.Error
		}

		if err := clearGlobalTraffic(tx, resetEmails...); err != nil {
			return err
		}

		inboundWhereText := "id "
		if id == -1 {
			inboundWhereText += " > ?"
		} else {
			inboundWhereText += " = ?"
		}

		result = tx.Model(model.Inbound{}).
			Where(inboundWhereText, id).
			Update("last_traffic_reset_time", now)

		return result.Error
	}); err != nil {
		return err
	}
	return nil
}

func (s *ClientService) ResetAllTraffics() (bool, error) {
	db := database.GetDB()
	res := db.Model(&xray.ClientTraffic{}).
		Where("1 = 1").
		Updates(map[string]any{"up": 0, "down": 0})
	if res.Error != nil {
		return false, res.Error
	}
	if err := db.Where("1 = 1").Delete(&model.ClientGlobalTraffic{}).Error; err != nil {
		return false, err
	}
	return res.RowsAffected > 0, nil
}
