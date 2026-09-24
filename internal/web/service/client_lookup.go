package service

import (
	"encoding/json"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

func (s *ClientService) GetRecordByEmail(tx *gorm.DB, email string) (*model.ClientRecord, error) {
	if tx == nil {
		tx = database.GetDB()
	}
	email = strings.TrimSpace(email)
	row := &model.ClientRecord{}
	err := tx.Where("email = ?", email).First(row).Error
	if err == nil {
		return row, nil
	}
	if !database.IsNotFound(err) {
		return nil, err
	}
	// Client email identity is case-insensitive. The fallback keeps older
	// panels that stored mixed-case addresses from creating a second record
	// during import or reseller-scoped writes.
	if lowerErr := tx.Where("LOWER(TRIM(email)) = LOWER(?)", email).First(row).Error; lowerErr != nil {
		return nil, lowerErr
	}
	return row, nil
}

// EffectiveFlow returns the client's flow from the first flow-capable inbound
// it is attached to (lowest inbound_id with a non-empty flow_override). The
// canonical clients.Flow column is unreliable for multi-inbound clients: a
// non-flow inbound (Hysteria, WS, gRPC, …) carries an empty flow and, when its
// SyncInbound runs last, overwrites the column to "" even though a VLESS Reality
// inbound stored a real flow. The per-inbound flow_override is always correct,
// so derive the display flow from it (order-independent). See issue #4792.
func (s *ClientService) EffectiveFlow(tx *gorm.DB, recordId int) (string, error) {
	return s.effectiveFlow(tx, recordId, nil)
}

// EffectiveFlowForInbounds limits the derived flow to associations visible to
// the caller. Reseller client records are explicit, but a client can also be
// attached to an admin-owned inbound whose flow must not influence a scoped
// response.
func (s *ClientService) EffectiveFlowForInbounds(tx *gorm.DB, recordId int, allowedInboundIDs map[int]struct{}) (string, error) {
	return s.effectiveFlow(tx, recordId, allowedInboundIDs)
}

func (s *ClientService) effectiveFlow(tx *gorm.DB, recordId int, allowedInboundIDs map[int]struct{}) (string, error) {
	if tx == nil {
		tx = database.GetDB()
	}
	query := tx.Model(&model.ClientInbound{}).
		Where("client_id = ? AND flow_override <> ?", recordId, "")
	if allowedInboundIDs != nil {
		ids := make([]int, 0, len(allowedInboundIDs))
		for id := range allowedInboundIDs {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return "", nil
		}
		query = query.Where("inbound_id IN ?", ids)
	}
	var flows []string
	if err := query.Order("inbound_id ASC").Limit(1).Pluck("flow_override", &flows).Error; err != nil {
		return "", err
	}
	if len(flows) == 0 {
		return "", nil
	}
	return flows[0], nil
}

func (s *ClientService) GetInboundIdsForEmail(tx *gorm.DB, email string) ([]int, error) {
	if tx == nil {
		tx = database.GetDB()
	}
	var ids []int
	err := tx.Table("client_inbounds").
		Select("client_inbounds.inbound_id").
		Joins("JOIN clients ON clients.id = client_inbounds.client_id").
		Where("LOWER(TRIM(clients.email)) = LOWER(?)", strings.TrimSpace(email)).
		Scan(&ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *ClientService) GetByID(id int) (*model.ClientRecord, error) {
	row := &model.ClientRecord{}
	if err := database.GetDB().Where("id = ?", id).First(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *ClientService) GetInboundIdsForRecord(id int) ([]int, error) {
	var ids []int
	err := database.GetDB().Table("client_inbounds").
		Where("client_id = ?", id).
		Order("inbound_id ASC").
		Pluck("inbound_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *ClientService) List() ([]ClientWithAttachments, error) {
	db := database.GetDB()
	var rows []model.ClientRecord
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ClientWithAttachments{}, nil
	}

	clientIds := make([]int, 0, len(rows))
	emails := make([]string, 0, len(rows))
	for i := range rows {
		clientIds = append(clientIds, rows[i].Id)
		if key := transferEmailKey(rows[i].Email); key != "" {
			emails = append(emails, key)
		}
	}

	attachments := make(map[int][]int, len(rows))
	for _, batch := range chunkInts(clientIds, sqlInChunk) {
		var links []model.ClientInbound
		if err := db.Where("client_id IN ?", batch).Find(&links).Error; err != nil {
			return nil, err
		}
		for _, l := range links {
			attachments[l.ClientId] = append(attachments[l.ClientId], l.InboundId)
		}
	}

	trafficByEmail := make(map[string]*xray.ClientTraffic, len(emails))
	if len(emails) > 0 {
		var stats []xray.ClientTraffic
		for _, batch := range chunkStrings(emails, sqlInChunk) {
			var batchStats []xray.ClientTraffic
			if err := db.Where("LOWER(TRIM(email)) IN ?", batch).Find(&batchStats).Error; err != nil {
				return nil, err
			}
			stats = append(stats, batchStats...)
		}
		overlayGlobalTrafficValues(db, stats)
		for i := range stats {
			trafficByEmail[strings.ToLower(strings.TrimSpace(stats[i].Email))] = &stats[i]
		}
	}

	out := make([]ClientWithAttachments, 0, len(rows))
	for i := range rows {
		out = append(out, ClientWithAttachments{
			ClientRecord: rows[i],
			InboundIds:   attachments[rows[i].Id],
			Traffic:      trafficByEmail[strings.ToLower(strings.TrimSpace(rows[i].Email))],
		})
	}
	return out, nil
}

func (s *ClientService) HasPendingNode(inboundSvc *InboundService, email string) bool {
	if strings.TrimSpace(email) == "" {
		return false
	}
	ids, err := s.GetInboundIdsForEmail(nil, email)
	if err != nil {
		return false
	}
	return inboundSvc.AnyNodePending(ids)
}

// findInboundIdsByClientEmail returns every inbound whose settings.clients[]
// JSON contains an entry with the given email. Driver-portable (no JSON
// operators) by parsing in Go — fine for the rare fallback path.
func (s *ClientService) findInboundIdsByClientEmail(email string) ([]int, error) {
	var inbounds []model.Inbound
	if err := database.GetDB().
		Select("id, settings").
		Where("LOWER(settings) LIKE LOWER(?)", "%"+strings.TrimSpace(email)+"%").
		Find(&inbounds).Error; err != nil {
		return nil, err
	}
	out := make([]int, 0, len(inbounds))
	for _, ib := range inbounds {
		var settings map[string]any
		if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
			continue
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			continue
		}
		for _, c := range clients {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if cEmail, _ := cm["email"].(string); transferEmailKey(cEmail) == transferEmailKey(email) {
				out = append(out, ib.Id)
				break
			}
		}
	}
	return out, nil
}
