package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *InboundService) AddTraffic(inboundTraffics []*xray.Traffic, clientTraffics []*xray.ClientTraffic) (needRestart bool, clientsDisabled bool, err error) {
	var disabledNodeIDs []int
	err = submitTrafficWrite(func() error {
		var inner error
		needRestart, clientsDisabled, disabledNodeIDs, inner = s.addTrafficLocked(inboundTraffics, clientTraffics)
		return inner
	})
	if err == nil && len(disabledNodeIDs) > 0 {
		s.restartRemoteNodesOnDisable(disabledNodeIDs)
	}
	return
}

func (s *InboundService) addTrafficLocked(inboundTraffics []*xray.Traffic, clientTraffics []*xray.ClientTraffic) (bool, bool, []int, error) {
	var err error
	db := database.GetDB()
	tx := db.Begin()

	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	err = s.addInboundTraffic(tx, inboundTraffics)
	if err != nil {
		return false, false, nil, err
	}
	err = s.addClientTraffic(tx, clientTraffics)
	if err != nil {
		return false, false, nil, err
	}

	needRestart0, count, err := s.autoRenewClients(tx)
	if err != nil {
		logger.Warning("Error in renew clients:", err)
	} else if count > 0 {
		logger.Debugf("%v clients renewed", count)
	}

	disabledClientsCount := int64(0)
	needRestart1, count, disabledNodeIDs, err := s.disableInvalidClients(tx)
	if err != nil {
		logger.Warning("Error in disabling invalid clients:", err)
	} else if count > 0 {
		logger.Debugf("%v clients disabled", count)
		disabledClientsCount = count
	}

	needRestart2, count, err := s.disableInvalidInbounds(tx)
	if err != nil {
		logger.Warning("Error in disabling invalid inbounds:", err)
	} else if count > 0 {
		logger.Debugf("%v inbounds disabled", count)
	}
	return needRestart0 || needRestart1 || needRestart2, disabledClientsCount > 0, disabledNodeIDs, nil
}

func (s *InboundService) addInboundTraffic(tx *gorm.DB, traffics []*xray.Traffic) error {
	if len(traffics) == 0 {
		return nil
	}

	var err error

	for _, traffic := range traffics {
		if traffic.IsInbound {
			err = tx.Model(&model.Inbound{}).Where("tag = ? AND node_id IS NULL", traffic.Tag).
				Updates(map[string]any{
					"up":   gorm.Expr("up + ?", traffic.Up),
					"down": gorm.Expr("down + ?", traffic.Down),
				}).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *InboundService) addClientTraffic(tx *gorm.DB, traffics []*xray.ClientTraffic) (err error) {
	if len(traffics) == 0 {
		return nil
	}

	emails := make([]string, 0, len(traffics))
	seenEmails := make(map[string]struct{}, len(traffics))
	for _, traffic := range traffics {
		if traffic == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(traffic.Email))
		if key == "" {
			continue
		}
		traffic.Email = strings.TrimSpace(traffic.Email)
		if _, seen := seenEmails[key]; !seen {
			seenEmails[key] = struct{}{}
			emails = append(emails, key)
		}
	}
	dbClientTraffics := make([]*xray.ClientTraffic, 0, len(traffics))
	// Match purely by email. client_traffics is email-keyed (one shared row per
	// email regardless of how many inbounds the client is attached to), and these
	// emails come from the local xray's report, so they always belong to a client
	// attached to a local inbound. The old `inbound_id NOT IN (node inbounds)`
	// filter dropped the local traffic of a client attached to both a node and the
	// mother inbound whenever the node inbound happened to be attached first — its
	// shared row then carried the node inbound's id (AddClientStat uses OnConflict
	// DoNothing and never refreshes it), so the local poll skipped it entirely.
	err = tx.Model(xray.ClientTraffic{}).
		Where("LOWER(TRIM(email)) IN (?)", emails).
		Find(&dbClientTraffics).Error
	if err != nil {
		return err
	}

	// Avoid empty slice error
	if len(dbClientTraffics) == 0 {
		return nil
	}

	dbClientTraffics, err = s.adjustTraffics(tx, dbClientTraffics)
	if err != nil {
		return err
	}

	// Index by normalized email for O(N) merge — the previous nested loop was
	// O(N²) and dominated each cron tick on inbounds with thousands of active
	// clients (7500 × 7500 = 56M string comparisons every 10 seconds). A
	// snapshot can contain more than one daemon/inbound delta for the same
	// email, so aggregate those deltas instead of letting the last map entry
	// overwrite the earlier ones.
	type clientTrafficDelta struct {
		up     int64
		down   int64
		online bool
	}
	trafficByEmail := make(map[string]clientTrafficDelta, len(traffics))
	for _, traffic := range traffics {
		if traffic == nil {
			continue
		}
		key := transferEmailKey(traffic.Email)
		if key == "" {
			continue
		}
		delta := trafficByEmail[key]
		delta.up += traffic.Up
		delta.down += traffic.Down
		delta.online = delta.online || traffic.Up > 0 || traffic.Down > 0
		trafficByEmail[key] = delta
	}
	now := time.Now().UnixMilli()
	for dbTrafficIndex := range dbClientTraffics {
		delta, ok := trafficByEmail[transferEmailKey(dbClientTraffics[dbTrafficIndex].Email)]
		if !ok {
			continue
		}
		dbClientTraffics[dbTrafficIndex].Up += delta.up
		dbClientTraffics[dbTrafficIndex].Down += delta.down
		if delta.online {
			dbClientTraffics[dbTrafficIndex].LastOnline = now
		}
	}

	// Persist only accounting fields. In particular, never Save the traffic
	// payload's Enable bit: OpenVPN/L2TP collectors emit deltas with Enable
	// set to true for transport purposes, while the database row may have been
	// disabled by quota enforcement. A full Save here would silently re-enable
	// that client on the next standalone-protocol poll.
	for _, traffic := range dbClientTraffics {
		updates := map[string]any{
			"up":          traffic.Up,
			"down":        traffic.Down,
			"expiry_time": traffic.ExpiryTime,
			"last_online": traffic.LastOnline,
		}
		if err := tx.Model(xray.ClientTraffic{}).Where("id = ?", traffic.Id).Updates(updates).Error; err != nil {
			logger.Warning("AddClientTraffic update data ", err)
			return err
		}
	}

	return nil
}

func (s *InboundService) adjustTraffics(tx *gorm.DB, dbClientTraffics []*xray.ClientTraffic) ([]*xray.ClientTraffic, error) {
	now := time.Now().UnixMilli()

	// "Start After First Use" stores a negative expiry (the duration). On the
	// first traffic tick it becomes an absolute deadline of now+duration. Compute
	// it once per email so every inbound the client is attached to lands on the
	// same value (recomputing per inbound would skip all but the first one).
	newExpiryByEmail := make(map[string]int64, len(dbClientTraffics))
	for traffic_index := range dbClientTraffics {
		if dbClientTraffics[traffic_index].ExpiryTime < 0 {
			newExpiryByEmail[transferEmailKey(dbClientTraffics[traffic_index].Email)] = now - dbClientTraffics[traffic_index].ExpiryTime
		}
	}
	if len(newExpiryByEmail) == 0 {
		return dbClientTraffics, nil
	}

	delayedEmails := make([]string, 0, len(newExpiryByEmail))
	for email := range newExpiryByEmail {
		delayedEmails = append(delayedEmails, email)
	}

	// Resolve the owning inbounds through the client_inbounds link, which is
	// authoritative. client_traffics.inbound_id goes stale when an inbound is
	// deleted and recreated, which would leave the negative expiry unconverted.
	var inboundIds []int
	err := tx.Table("client_inbounds").
		Joins("JOIN clients ON clients.id = client_inbounds.client_id").
		Where("LOWER(TRIM(clients.email)) IN (?)", delayedEmails).
		Distinct().
		Pluck("client_inbounds.inbound_id", &inboundIds).Error
	if err != nil {
		return nil, err
	}
	if len(inboundIds) == 0 {
		return dbClientTraffics, nil
	}

	var inbounds []*model.Inbound
	err = tx.Model(model.Inbound{}).Where("id IN (?)", inboundIds).Find(&inbounds).Error
	if err != nil {
		return nil, err
	}
	for inbound_index := range inbounds {
		settings := map[string]any{}
		json.Unmarshal([]byte(inbounds[inbound_index].Settings), &settings)
		clients, ok := settings["clients"].([]any)
		if ok {
			var newClients []any
			for client_index := range clients {
				c := clients[client_index].(map[string]any)
				email, _ := c["email"].(string)
				if newExpiry, ok := newExpiryByEmail[transferEmailKey(email)]; ok {
					c["expiryTime"] = newExpiry
					c["updated_at"] = now
				}
				if _, ok := c["created_at"]; !ok {
					c["created_at"] = now
				}
				if _, ok := c["updated_at"]; !ok {
					c["updated_at"] = now
				}
				newClients = append(newClients, any(c))
			}
			settings["clients"] = newClients
			modifiedSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return nil, err
			}

			inbounds[inbound_index].Settings = string(modifiedSettings)
		}
	}

	for traffic_index := range dbClientTraffics {
		if newExpiry, ok := newExpiryByEmail[transferEmailKey(dbClientTraffics[traffic_index].Email)]; ok {
			dbClientTraffics[traffic_index].ExpiryTime = newExpiry
		}
	}

	err = tx.Save(inbounds).Error
	if err != nil {
		logger.Warning("AddClientTraffic update inbounds ", err)
		logger.Error(inbounds)
	} else {
		for _, ib := range inbounds {
			if ib == nil {
				continue
			}
			cs, gcErr := s.GetClients(ib)
			if gcErr != nil {
				logger.Warning("AddClientTraffic sync clients: GetClients failed", gcErr)
				continue
			}
			if syncErr := s.clientService.SyncInbound(tx, ib.Id, cs); syncErr != nil {
				logger.Warning("AddClientTraffic sync clients: SyncInbound failed", syncErr)
			}
		}
	}

	return dbClientTraffics, nil
}

func (s *InboundService) autoRenewClients(tx *gorm.DB) (bool, int64, error) {
	// check for time expired
	var traffics []*xray.ClientTraffic
	now := time.Now().Unix() * 1000
	var err, err1 error

	err = tx.Model(xray.ClientTraffic{}).
		Where("reset > 0 and expiry_time > 0 and expiry_time <= ?", now).
		Where("inbound_id NOT IN (?)", tx.Model(&model.Inbound{}).Select("id").Where("node_id IS NOT NULL")).
		Find(&traffics).Error
	if err != nil {
		return false, 0, err
	}
	// return if there is no client to renew
	if len(traffics) == 0 {
		return false, 0, nil
	}

	var inbound_ids []int
	var inbounds []*model.Inbound
	needRestart := false
	var clientsToAdd []struct {
		protocol string
		tag      string
		client   map[string]any
	}

	// Resolve the inbounds to renew through the client_inbounds link rather than
	// client_traffics.inbound_id, which goes stale after an inbound is deleted and
	// recreated and would otherwise skip the renew entirely.
	renewEmails := make([]string, 0, len(traffics))
	for _, traffic := range traffics {
		if key := transferEmailKey(traffic.Email); key != "" {
			renewEmails = append(renewEmails, key)
		}
	}
	for _, batch := range chunkStrings(renewEmails, sqliteMaxVars) {
		var ids []int
		if err = tx.Table("client_inbounds").
			Joins("JOIN clients ON clients.id = client_inbounds.client_id").
			Where("LOWER(TRIM(clients.email)) IN ?", batch).
			Distinct().
			Pluck("client_inbounds.inbound_id", &ids).Error; err != nil {
			return false, 0, err
		}
		inbound_ids = append(inbound_ids, ids...)
	}
	// Dedupe so an inbound hosting N expired clients is fetched and saved once
	// per tick instead of N times across chunk boundaries.
	inbound_ids = uniqueInts(inbound_ids)
	// Chunked to stay under SQLite's bind-variable limit when many inbounds
	// are touched in a single tick.
	for _, batch := range chunkInts(inbound_ids, sqliteMaxVars) {
		var page []*model.Inbound
		if err = tx.Model(model.Inbound{}).Where("id IN ?", batch).Find(&page).Error; err != nil {
			return false, 0, err
		}
		inbounds = append(inbounds, page...)
	}
	for inbound_index := range inbounds {
		settings := map[string]any{}
		json.Unmarshal([]byte(inbounds[inbound_index].Settings), &settings)
		clients := settings["clients"].([]any)
		for client_index := range clients {
			c := clients[client_index].(map[string]any)
			for traffic_index, traffic := range traffics {
				if transferEmailKey(traffic.Email) == transferEmailKey(c["email"].(string)) {
					newExpiryTime := traffic.ExpiryTime
					for newExpiryTime < now {
						newExpiryTime += (int64(traffic.Reset) * 86400000)
					}
					c["expiryTime"] = newExpiryTime
					traffics[traffic_index].ExpiryTime = newExpiryTime
					traffics[traffic_index].Down = 0
					traffics[traffic_index].Up = 0
					if !traffic.Enable {
						traffics[traffic_index].Enable = true
						c["enable"] = true
						clientsToAdd = append(clientsToAdd,
							struct {
								protocol string
								tag      string
								client   map[string]any
							}{
								protocol: string(inbounds[inbound_index].Protocol),
								tag:      inbounds[inbound_index].Tag,
								client:   c,
							})
					}
					clients[client_index] = any(c)
					break
				}
			}
		}
		settings["clients"] = clients
		newSettings, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return false, 0, err
		}
		inbounds[inbound_index].Settings = string(newSettings)
	}
	err = tx.Save(inbounds).Error
	if err != nil {
		return false, 0, err
	}
	for _, ib := range inbounds {
		if ib == nil {
			continue
		}
		cs, gcErr := s.GetClients(ib)
		if gcErr != nil {
			logger.Warning("autoRenewClients sync clients: GetClients failed", gcErr)
			continue
		}
		if syncErr := s.clientService.SyncInbound(tx, ib.Id, cs); syncErr != nil {
			logger.Warning("autoRenewClients sync clients: SyncInbound failed", syncErr)
		}
	}
	err = tx.Save(traffics).Error
	if err != nil {
		return false, 0, err
	}
	// A renewed client starts a fresh quota window: drop the cross-panel rows
	// too, or the stale pushed totals would re-deplete it immediately.
	if err = clearGlobalTraffic(tx, renewEmails...); err != nil {
		return false, 0, err
	}
	if p != nil {
		err1 = s.xrayApi.Init(p.GetAPIPort())
		if err1 != nil {
			return true, int64(len(traffics)), nil
		}
		for _, clientToAdd := range clientsToAdd {
			err1 = s.xrayApi.AddUser(clientToAdd.protocol, clientToAdd.tag, clientToAdd.client)
			if err1 != nil {
				needRestart = true
			}
		}
		s.xrayApi.Close()
	}
	return needRestart, int64(len(traffics)), nil
}

// AddClientStat inserts a per-client accounting row, no-op on email
// conflict. Xray reports traffic per email, so the surviving row acts as
// the shared accumulator for inbounds that re-use the same identity.
func (s *InboundService) AddClientStat(tx *gorm.DB, inboundId int, client *model.Client) error {
	if client == nil {
		return nil
	}
	client.Email = strings.TrimSpace(client.Email)
	if client.Email == "" {
		return nil
	}
	var existing xray.ClientTraffic
	result := tx.Where("LOWER(TRIM(email)) = LOWER(?)", client.Email).First(&existing)
	if result.Error == nil {
		return nil
	}
	if !database.IsNotFound(result.Error) {
		return result.Error
	}
	clientTraffic := xray.ClientTraffic{
		InboundId:  inboundId,
		Email:      client.Email,
		Total:      client.TotalGB,
		ExpiryTime: client.ExpiryTime,
		Enable:     client.Enable,
		Reset:      client.Reset,
	}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "email"}}, DoNothing: true}).
		Create(&clientTraffic).Error
}

func (s *InboundService) UpdateClientStat(tx *gorm.DB, email string, client *model.Client) error {
	result := tx.Model(xray.ClientTraffic{}).
		Where("LOWER(TRIM(email)) = LOWER(?)", strings.TrimSpace(email)).
		Updates(map[string]any{
			"enable":      client.Enable,
			"email":       client.Email,
			"total":       client.TotalGB,
			"expiry_time": client.ExpiryTime,
			"reset":       client.Reset,
		})
	err := result.Error
	return err
}

func (s *InboundService) DelClientStat(tx *gorm.DB, email string) error {
	if err := tx.Where("LOWER(TRIM(email)) = LOWER(?)", strings.TrimSpace(email)).Delete(xray.ClientTraffic{}).Error; err != nil {
		return err
	}
	if err := clearGlobalTraffic(tx, email); err != nil {
		return err
	}
	return tx.Where("LOWER(TRIM(email)) = LOWER(?)", strings.TrimSpace(email)).Delete(&model.NodeClientTraffic{}).Error
}

func (s *InboundService) delClientStatsByEmails(tx *gorm.DB, emails []string) error {
	const chunk = 400
	keys := make([]string, 0, len(emails))
	for _, email := range emails {
		if key := strings.ToLower(strings.TrimSpace(email)); key != "" {
			keys = append(keys, key)
		}
	}
	for start := 0; start < len(keys); start += chunk {
		end := min(start+chunk, len(keys))
		batch := keys[start:end]
		if err := tx.Where("LOWER(TRIM(email)) IN ?", batch).Delete(xray.ClientTraffic{}).Error; err != nil {
			return err
		}
		if err := tx.Where("LOWER(TRIM(email)) IN ?", batch).Delete(&model.ClientGlobalTraffic{}).Error; err != nil {
			return err
		}
		if err := tx.Where("LOWER(TRIM(email)) IN ?", batch).Delete(&model.NodeClientTraffic{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *InboundService) ResetClientTrafficByEmail(clientEmail string) error {
	return submitTrafficWrite(func() error {
		db := database.GetDB()
		if err := clearGlobalTraffic(db, clientEmail); err != nil {
			return err
		}
		return db.Model(xray.ClientTraffic{}).
			Where("LOWER(TRIM(email)) = LOWER(?)", strings.TrimSpace(clientEmail)).
			Updates(map[string]any{"enable": true, "up": 0, "down": 0}).Error
	})
}

func (s *InboundService) ResetClientTraffic(id int, clientEmail string) (needRestart bool, err error) {
	err = submitTrafficWrite(func() error {
		var inner error
		needRestart, inner = s.resetClientTrafficLocked(id, clientEmail)
		return inner
	})
	return
}

func (s *InboundService) resetClientTrafficLocked(id int, clientEmail string) (bool, error) {
	needRestart := false

	traffic, err := s.GetClientTrafficByEmail(clientEmail)
	if err != nil {
		return false, err
	}
	if traffic == nil {
		// A client can have a valid association before its first accounting
		// sample creates the shared traffic row. Resetting that association is
		// already idempotently complete; do not dereference a nil row or force
		// an unnecessary restart.
		return false, nil
	}

	if !traffic.Enable {
		inbound, err := s.GetInbound(id)
		if err != nil {
			return false, err
		}
		clients, err := s.GetClients(inbound)
		if err != nil {
			return false, err
		}
		for _, client := range clients {
			if transferEmailKey(client.Email) == transferEmailKey(clientEmail) && client.Enable {
				rt, push, dirty, perr := s.nodePushPlan(inbound)
				if perr != nil {
					return false, perr
				}
				if !push {
					if inbound.NodeID != nil {
						if dirty {
							if dErr := (&NodeService{}).MarkNodeDirty(*inbound.NodeID); dErr != nil {
								logger.Warning("mark node dirty failed:", dErr)
							}
						}
					} else {
						needRestart = true
					}
					break
				}
				cipher := ""
				if string(inbound.Protocol) == "shadowsocks" {
					var oldSettings map[string]any
					err = json.Unmarshal([]byte(inbound.Settings), &oldSettings)
					if err != nil {
						return false, err
					}
					cipher = oldSettings["method"].(string)
				}
				err1 := rt.AddUser(context.Background(), inbound, map[string]any{
					"email":    client.Email,
					"id":       client.ID,
					"auth":     client.Auth,
					"security": client.Security,
					"flow":     client.Flow,
					"password": client.Password,
					"cipher":   cipher,
				})
				if err1 == nil {
					logger.Debug("Client enabled on", rt.Name(), "due to reset traffic:", clientEmail)
				} else if inbound.NodeID != nil {
					logger.Warning("Error in enabling client on", rt.Name(), ":", err1)
					if dErr := (&NodeService{}).MarkNodeDirty(*inbound.NodeID); dErr != nil {
						logger.Warning("mark node dirty failed:", dErr)
					}
				} else {
					logger.Debug("Error in enabling client on", rt.Name(), ":", err1)
					needRestart = true
				}
				break
			}
		}
	}

	traffic.Up = 0
	traffic.Down = 0
	traffic.Enable = true

	db := database.GetDB()
	err = db.Save(traffic).Error
	if err != nil {
		return false, err
	}
	if err := clearGlobalTraffic(db, clientEmail); err != nil {
		return false, err
	}

	now := time.Now().UnixMilli()
	_ = db.Model(model.Inbound{}).
		Where("id = ?", id).
		Update("last_traffic_reset_time", now).Error

	inbound, err := s.GetInbound(id)
	if err == nil && inbound != nil && inbound.NodeID != nil {
		if rt, rterr := s.runtimeFor(inbound); rterr == nil {
			if e := rt.ResetClientTraffic(context.Background(), inbound, clientEmail); e != nil {
				logger.Warning("ResetClientTraffic: remote propagation to", rt.Name(), "failed:", e)
			}
		} else {
			logger.Warning("ResetClientTraffic: runtime lookup failed:", rterr)
		}
	}

	return needRestart, nil
}

func (s *InboundService) ResetAllTraffics() error {
	return submitTrafficWrite(func() error {
		return s.resetAllTrafficsLocked()
	})
}

func (s *InboundService) resetAllTrafficsLocked() error {
	db := database.GetDB()
	now := time.Now().UnixMilli()

	if err := db.Model(model.Inbound{}).
		Where("user_id > ?", 0).
		Updates(map[string]any{
			"up":                      0,
			"down":                    0,
			"last_traffic_reset_time": now,
		}).Error; err != nil {
		return err
	}

	nodes, err := (&NodeService{}).GetAll()
	if err == nil {
		for _, node := range nodes {
			if rt, err := runtime.GetManager().RuntimeFor(&node.Id); err == nil {
				if e := rt.ResetAllTraffics(context.Background()); e != nil {
					logger.Warning("ResetAllTraffics: remote propagation to", rt.Name(), "failed:", e)
				}
			}
		}
	}

	return nil
}

func (s *InboundService) ResetInboundTraffic(id int) error {
	return submitTrafficWrite(func() error {
		db := database.GetDB()
		if err := db.Model(model.Inbound{}).
			Where("id = ?", id).
			Updates(map[string]any{"up": 0, "down": 0}).Error; err != nil {
			return err
		}

		inbound, err := s.GetInbound(id)
		if err == nil && inbound != nil && inbound.NodeID != nil {
			if rt, rterr := s.runtimeFor(inbound); rterr == nil {
				if e := rt.ResetInboundTraffic(context.Background(), inbound); e != nil {
					logger.Warning("ResetInboundTraffic: remote propagation to", rt.Name(), "failed:", e)
				}
			} else {
				logger.Warning("ResetInboundTraffic: runtime lookup failed:", rterr)
			}
		}

		return nil
	})
}

func (s *InboundService) DelDepletedClients(id int) (err error) {
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	// Collect and mutate exclusively through tx. Runtime calls are deliberately
	// deferred until after Commit: a daemon must never be told that a client was
	// deleted when the corresponding settings/link transaction later rolls back.
	now := time.Now().UnixMilli()
	depletedClause := "reset = 0 AND ((total > 0 AND up + down >= total) OR (expiry_time > 0 AND expiry_time <= ?))"
	var depletedRows []xray.ClientTraffic
	if err = tx.Model(xray.ClientTraffic{}).
		Where(depletedClause, now).
		Find(&depletedRows).Error; err != nil {
		return err
	}
	if len(depletedRows) == 0 {
		if err = tx.Commit().Error; err != nil {
			return err
		}
		committed = true
		return nil
	}

	depletedEmails := make(map[string]struct{}, len(depletedRows))
	for _, row := range depletedRows {
		if key := transferEmailKey(row.Email); key != "" {
			depletedEmails[key] = struct{}{}
		}
	}
	if len(depletedEmails) == 0 {
		if err = tx.Commit().Error; err != nil {
			return err
		}
		committed = true
		return nil
	}

	var inbounds []*model.Inbound
	inboundQuery := tx.Model(model.Inbound{})
	if id >= 0 {
		inboundQuery = inboundQuery.Where("id = ?", id)
	}
	if err = inboundQuery.Find(&inbounds).Error; err != nil {
		return err
	}

	type runtimeUpdate struct {
		old model.Inbound
		new model.Inbound
	}
	updates := make([]runtimeUpdate, 0, len(inbounds))
	for _, inbound := range inbounds {
		var settings map[string]any
		if err = json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return err
		}
		rawClients, ok := settings["clients"].([]any)
		if !ok {
			continue
		}

		newClients := make([]any, 0, len(rawClients))
		removed := false
		for _, rawClient := range rawClients {
			client, ok := rawClient.(map[string]any)
			if !ok {
				newClients = append(newClients, rawClient)
				continue
			}
			email, _ := client["email"].(string)
			if _, isDepleted := depletedEmails[transferEmailKey(email)]; isDepleted {
				removed = true
				continue
			}
			newClients = append(newClients, rawClient)
		}
		if !removed {
			continue
		}

		oldInbound := *inbound
		settings["clients"] = newClients
		newSettings, marshalErr := json.MarshalIndent(settings, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		if err = tx.Model(model.Inbound{}).Where("id = ?", inbound.Id).
			Update("settings", string(newSettings)).Error; err != nil {
			return err
		}
		inbound.Settings = string(newSettings)

		survivingClients, clientsErr := s.GetClients(inbound)
		if clientsErr != nil {
			return clientsErr
		}
		if err = s.clientService.SyncInbound(tx, inbound.Id, survivingClients); err != nil {
			return err
		}
		updates = append(updates, runtimeUpdate{old: oldInbound, new: *inbound})
	}

	// Keep a shared client's accounting while another inbound still owns the
	// canonical association. The link table, rather than a stale
	// client_traffics.inbound_id or settings substring, is authoritative.
	emails := make([]string, 0, len(depletedEmails))
	for email := range depletedEmails {
		emails = append(emails, email)
	}
	var toDelete []string
	if id < 0 {
		toDelete = emails
	} else {
		var stillReferenced []string
		if err = tx.Table("client_inbounds").
			Joins("JOIN clients ON clients.id = client_inbounds.client_id").
			Where("LOWER(TRIM(clients.email)) IN ?", emails).
			Distinct().
			Pluck("LOWER(TRIM(clients.email))", &stillReferenced).Error; err != nil {
			return err
		}
		stillSet := make(map[string]struct{}, len(stillReferenced))
		for _, email := range stillReferenced {
			stillSet[transferEmailKey(email)] = struct{}{}
		}
		for _, email := range emails {
			if _, kept := stillSet[email]; !kept {
				toDelete = append(toDelete, email)
			}
		}
	}
	if len(toDelete) > 0 {
		if err = s.delClientStatsByEmails(tx, toDelete); err != nil {
			return err
		}
	}

	if err = tx.Commit().Error; err != nil {
		return err
	}
	committed = true

	// The database is now authoritative. Reconcile each affected runtime only
	// after commit, and treat failures as retryable errors instead of pretending
	// the daemon changed. OpenVPN is host-managed; its next reconcile observes
	// the removed settings/link and must not be driven through the Xray API.
	var runtimeErr error
	for _, change := range updates {
		if change.old.Protocol == model.OpenVPN && change.old.NodeID == nil {
			continue
		}
		if change.old.Protocol == model.L2TP && change.old.NodeID == nil {
			if reconcileErr := reconcileL2TPRuntime(&change.new); reconcileErr != nil && runtimeErr == nil {
				runtimeErr = reconcileErr
			}
			continue
		}

		rt, push, dirty, pushErr := s.nodePushPlan(&change.new)
		if pushErr != nil {
			if runtimeErr == nil {
				runtimeErr = pushErr
			}
			continue
		}
		if push {
			if updateErr := rt.UpdateInbound(context.Background(), &change.old, &change.new); updateErr != nil && runtimeErr == nil {
				runtimeErr = updateErr
			}
		} else if dirty && change.new.NodeID != nil {
			if dirtyErr := (&NodeService{}).MarkNodeDirty(*change.new.NodeID); dirtyErr != nil && runtimeErr == nil {
				runtimeErr = dirtyErr
			}
		}
	}
	return runtimeErr
}

func (s *InboundService) GetClientTrafficTgBot(tgId int64) ([]*xray.ClientTraffic, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound

	// Retrieve inbounds where settings contain the given tgId
	err := db.Model(model.Inbound{}).Where("settings LIKE ?", fmt.Sprintf(`%%"tgId": %d%%`, tgId)).Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		logger.Errorf("Error retrieving inbounds with tgId %d: %v", tgId, err)
		return nil, err
	}

	var emails []string
	for _, inbound := range inbounds {
		clients, err := s.GetClients(inbound)
		if err != nil {
			logger.Errorf("Error retrieving clients for inbound %d: %v", inbound.Id, err)
			continue
		}
		for _, client := range clients {
			if client.TgID == tgId {
				emails = append(emails, client.Email)
			}
		}
	}

	// Chunked to stay under SQLite's bind-variable limit when a single Telegram
	// account owns thousands of clients across inbounds.
	uniqEmails := uniqueNonEmptyStrings(emails)
	traffics := make([]*xray.ClientTraffic, 0, len(uniqEmails))
	for _, batch := range chunkStrings(uniqEmails, sqliteMaxVars) {
		var page []*xray.ClientTraffic
		if err = db.Model(xray.ClientTraffic{}).Where("LOWER(TRIM(email)) IN ?", batch).Find(&page).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			logger.Errorf("Error retrieving ClientTraffic for emails %v: %v", batch, err)
			return nil, err
		}
		traffics = append(traffics, page...)
	}
	if len(traffics) == 0 {
		logger.Warning("No ClientTraffic records found for emails:", emails)
		return nil, nil
	}

	// Populate UUID and other client data for each traffic record
	for i := range traffics {
		if ct, client, e := s.GetClientByEmail(traffics[i].Email); e == nil && ct != nil && client != nil {
			traffics[i].Enable = client.Enable
			traffics[i].UUID = client.ID
			traffics[i].SubId = client.SubID
		}
	}

	return traffics, nil
}

// BumpClientsLastOnline sets client_traffics.last_online to now for the given
// emails. Used in online-API mode for clients that hold a live connection but
// moved no bytes this poll — the traffic path (addClientTraffic) only bumps
// last_online on a non-zero delta, so idle-but-connected clients would
// otherwise show a stale "last online" while being reported online.
func (s *InboundService) BumpClientsLastOnline(emails []string) error {
	uniq := uniqueNonEmptyStrings(emails)
	if len(uniq) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	return submitTrafficWrite(func() error {
		db := database.GetDB()
		for _, batch := range chunkStrings(uniq, sqliteMaxVars) {
			if err := db.Model(xray.ClientTraffic{}).Where("LOWER(TRIM(email)) IN ?", batch).Update("last_online", now).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *InboundService) GetActiveClientTraffics(emails []string) ([]*xray.ClientTraffic, error) {
	uniq := uniqueNonEmptyStrings(emails)
	if len(uniq) == 0 {
		return nil, nil
	}
	db := database.GetDB()
	traffics := make([]*xray.ClientTraffic, 0, len(uniq))
	for _, batch := range chunkStrings(uniq, sqliteMaxVars) {
		var page []*xray.ClientTraffic
		if err := db.Model(xray.ClientTraffic{}).Where("LOWER(TRIM(email)) IN ?", batch).Find(&page).Error; err != nil {
			return nil, err
		}
		traffics = append(traffics, page...)
	}
	return traffics, nil
}

// GetAllClientTraffics returns the full set of client_traffics rows so the
// websocket broadcasters can ship a complete snapshot every cycle. The old
// delta-only path (GetActiveClientTraffics on activeEmails) silently dropped
// the per-client section whenever no client moved bytes in the cycle or a
// node sync failed, leaving client rows in the UI stuck at stale numbers.
func (s *InboundService) GetAllClientTraffics() ([]*xray.ClientTraffic, error) {
	db := database.GetDB()
	var traffics []*xray.ClientTraffic
	if err := db.Model(xray.ClientTraffic{}).Find(&traffics).Error; err != nil {
		return nil, err
	}
	overlayGlobalTraffic(db, traffics)
	return traffics, nil
}

type InboundTrafficSummary struct {
	Id     int   `json:"id"`
	Up     int64 `json:"up"`
	Down   int64 `json:"down"`
	Total  int64 `json:"total"`
	Enable bool  `json:"enable"`
}

func (s *InboundService) GetInboundsTrafficSummary() ([]InboundTrafficSummary, error) {
	db := database.GetDB()
	var summaries []InboundTrafficSummary
	if err := db.Model(&model.Inbound{}).
		Select("id, up, down, total, enable").
		Find(&summaries).Error; err != nil {
		return nil, err
	}
	return summaries, nil
}

func (s *InboundService) GetClientTrafficByEmail(email string) (traffic *xray.ClientTraffic, err error) {
	db := database.GetDB()
	var traffics []*xray.ClientTraffic
	if err := db.Model(xray.ClientTraffic{}).Where("LOWER(TRIM(email)) = LOWER(?)", strings.TrimSpace(email)).Find(&traffics).Error; err != nil {
		logger.Warningf("Error retrieving ClientTraffic with email %s: %v", email, err)
		return nil, err
	}
	if len(traffics) == 0 {
		return nil, nil
	}
	overlayGlobalTraffic(db, traffics)
	t := traffics[0]

	if rec, rErr := s.clientService.GetRecordByEmail(db, email); rErr == nil && rec != nil {
		c := rec.ToClient()
		t.UUID = c.ID
		t.SubId = c.SubID
		return t, nil
	}

	t2, client, err := s.GetClientByEmail(email)
	if err != nil {
		logger.Warningf("Error retrieving ClientTraffic with email %s: %v", email, err)
		return nil, err
	}
	if t2 != nil && client != nil {
		t2.UUID = client.ID
		t2.SubId = client.SubID
		return t2, nil
	}
	return nil, nil
}

// emailAssociationsWithinScope proves that every current association for a
// canonical client is inside the allowed inbound set. Client traffic and IP
// history are keyed by email rather than by inbound, so this is the minimum
// proof needed before returning or mutating either aggregate.
func (s *InboundService) emailAssociationsWithinScope(email string, allowedInboundIDs map[int]struct{}) (bool, error) {
	if len(allowedInboundIDs) == 0 {
		return false, nil
	}
	ids, err := s.clientService.GetInboundIdsForEmail(database.GetDB(), email)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return false, nil
	}
	for _, id := range ids {
		if _, ok := allowedInboundIDs[id]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// emailHasAssociationInScope proves that the client is attached to at least
// one inbound in the caller's scope. ClientTraffic is an email-level aggregate
// and may also include bytes from another inbound, but the caller's explicit
// ResellerClient mapping is the authorization boundary. The scoped response
// below therefore preserves the permitted counters while redacting the stale
// InboundId metadata that could identify the other inbound.
func (s *InboundService) emailHasAssociationInScope(email string, allowedInboundIDs map[int]struct{}) (bool, error) {
	if len(allowedInboundIDs) == 0 {
		return false, nil
	}
	ids, err := s.clientService.GetInboundIdsForEmail(database.GetDB(), email)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if _, ok := allowedInboundIDs[id]; ok {
			return true, nil
		}
	}
	return false, nil
}

// GetClientTrafficByEmailForInbounds returns the email aggregate once the
// client has an association in the requested scope. Reseller controllers first
// enforce the explicit ResellerClient mapping; this method enforces the owned
// inbound intersection and removes the legacy row's unrelated InboundId.
func (s *InboundService) GetClientTrafficByEmailForInbounds(email string, allowedInboundIDs map[int]struct{}) (*xray.ClientTraffic, error) {
	ok, err := s.emailHasAssociationInScope(email, allowedInboundIDs)
	if err != nil || !ok {
		return nil, err
	}
	traffic, err := s.GetClientTrafficByEmail(email)
	if err != nil || traffic == nil {
		return traffic, err
	}

	// ClientTraffic is historically keyed by email and its InboundId points to
	// whichever inbound first created the shared row. That inbound can belong to
	// an administrator or another reseller. Never return that stale routing
	// identifier through a scoped endpoint.
	scoped := *traffic
	scoped.InboundId = 0
	return &scoped, nil
}

func (s *InboundService) updateClientTrafficByEmail(email string, upload int64, download int64) error {
	db := database.GetDB()
	err := db.Model(xray.ClientTraffic{}).
		Where("LOWER(TRIM(email)) = LOWER(?)", strings.TrimSpace(email)).
		Updates(map[string]any{
			"up":   upload,
			"down": download,
		}).Error
	if err != nil {
		logger.Warningf("Error updating ClientTraffic with email %s: %v", email, err)
	}
	return err
}

// UpdateClientTrafficByEmailForInbounds applies the legacy email-level traffic
// update only when the client's complete association set is inside the supplied
// scope. This prevents a reseller's manual traffic correction from changing a
// shared client row that also belongs to another tenant.
func (s *InboundService) UpdateClientTrafficByEmailForInbounds(email string, upload int64, download int64, allowedInboundIDs map[int]struct{}) error {
	ok, err := s.emailAssociationsWithinScope(email, allowedInboundIDs)
	if err != nil || !ok {
		return err
	}
	return submitTrafficWrite(func() error {
		return s.updateClientTrafficByEmail(email, upload, download)
	})
}

func (s *InboundService) UpdateClientTrafficByEmail(email string, upload int64, download int64) error {
	return submitTrafficWrite(func() error {
		return s.updateClientTrafficByEmail(email, upload, download)
	})
}

func (s *InboundService) SearchClientTraffic(query string) (traffic *xray.ClientTraffic, err error) {
	db := database.GetDB()
	inbound := &model.Inbound{}
	traffic = &xray.ClientTraffic{}

	// Search for inbound settings that contain the query
	err = db.Model(model.Inbound{}).Where("settings LIKE ?", "%\""+query+"\"%").First(inbound).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			logger.Warningf("Inbound settings containing query %s not found: %v", query, err)
			return nil, err
		}
		logger.Errorf("Error searching for inbound settings with query %s: %v", query, err)
		return nil, err
	}

	traffic.InboundId = inbound.Id

	// Unmarshal settings to get clients
	settings := map[string][]model.Client{}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		logger.Errorf("Error unmarshalling inbound settings for inbound ID %d: %v", inbound.Id, err)
		return nil, err
	}

	clients := settings["clients"]
	for _, client := range clients {
		if (client.ID == query || client.Password == query) && client.Email != "" {
			traffic.Email = client.Email
			break
		}
	}

	if traffic.Email == "" {
		logger.Warningf("No client found with query %s in inbound ID %d", query, inbound.Id)
		return nil, gorm.ErrRecordNotFound
	}

	// Retrieve ClientTraffic based on the found email
	err = db.Model(xray.ClientTraffic{}).Where("LOWER(TRIM(email)) = LOWER(?)", strings.TrimSpace(traffic.Email)).First(traffic).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			logger.Warningf("ClientTraffic for email %s not found: %v", traffic.Email, err)
			return nil, err
		}
		logger.Errorf("Error retrieving ClientTraffic for email %s: %v", traffic.Email, err)
		return nil, err
	}

	return traffic, nil
}
