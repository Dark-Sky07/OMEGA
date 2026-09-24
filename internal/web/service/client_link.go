package service

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
)

func (s *ClientService) SyncInbound(tx *gorm.DB, inboundId int, clients []model.Client) error {
	return s.syncInbound(tx, inboundId, clients, false)
}

// SyncInboundPreservingExisting rebuilds only the association rows for an
// imported inbound. Existing ClientRecord rows remain authoritative; clients
// absent from the canonical table are created from the imported payload.
func (s *ClientService) SyncInboundPreservingExisting(tx *gorm.DB, inboundId int, clients []model.Client) error {
	return s.syncInbound(tx, inboundId, clients, true)
}

func (s *ClientService) syncInbound(tx *gorm.DB, inboundId int, clients []model.Client, preserveExisting bool) error {
	if tx == nil {
		tx = database.GetDB()
	}

	if err := tx.Where("inbound_id = ?", inboundId).Delete(&model.ClientInbound{}).Error; err != nil {
		return err
	}

	emails := make([]string, 0, len(clients))
	seen := make(map[string]struct{}, len(clients))
	for i := range clients {
		email := strings.TrimSpace(clients[i].Email)
		if email == "" {
			continue
		}
		clients[i].Email = email
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		emails = append(emails, email)
	}

	existing := make(map[string]*model.ClientRecord, len(emails))
	const selectChunk = 400
	for start := 0; start < len(emails); start += selectChunk {
		end := min(start+selectChunk, len(emails))
		var rows []model.ClientRecord
		keys := make([]string, 0, end-start)
		for _, email := range emails[start:end] {
			keys = append(keys, strings.ToLower(email))
		}
		if err := tx.Where("LOWER(TRIM(email)) IN ?", keys).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			existing[strings.ToLower(strings.TrimSpace(rows[i].Email))] = &rows[i]
		}
	}

	idByEmail := make(map[string]int, len(emails))
	pending := make(map[string]*model.ClientRecord, len(emails))
	toCreate := make([]*model.ClientRecord, 0, len(emails))
	for i := range clients {
		email := strings.TrimSpace(clients[i].Email)
		if email == "" {
			continue
		}

		key := strings.ToLower(email)
		incoming := clients[i].ToRecord()
		row, ok := existing[key]
		if !ok {
			if _, dup := pending[key]; !dup {
				pending[key] = incoming
				toCreate = append(toCreate, incoming)
			}
			continue
		}

		idByEmail[key] = row.Id
		if preserveExisting {
			continue
		}

		before := *row
		if incoming.UUID != "" {
			row.UUID = incoming.UUID
		}
		if incoming.Password != "" {
			row.Password = incoming.Password
		}
		if incoming.Auth != "" {
			row.Auth = incoming.Auth
		}
		row.Flow = incoming.Flow
		if incoming.Security != "" {
			row.Security = incoming.Security
		}
		if incoming.Reverse != "" {
			row.Reverse = incoming.Reverse
		}
		row.SubID = incoming.SubID
		row.LimitIP = incoming.LimitIP
		row.TotalGB = incoming.TotalGB
		row.ExpiryTime = incoming.ExpiryTime
		row.Enable = incoming.Enable
		row.TgID = incoming.TgID
		if incoming.Group != "" {
			row.Group = incoming.Group
		}
		row.Comment = incoming.Comment
		row.Reset = incoming.Reset
		if incoming.CreatedAt > 0 && (row.CreatedAt == 0 || incoming.CreatedAt < row.CreatedAt) {
			row.CreatedAt = incoming.CreatedAt
		}
		preservedUpdatedAt := max(incoming.UpdatedAt, row.UpdatedAt)
		row.UpdatedAt = preservedUpdatedAt

		if *row == before {
			continue
		}
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ClientRecord{}).
			Where("id = ?", row.Id).
			UpdateColumn("updated_at", preservedUpdatedAt).Error; err != nil {
			return err
		}
	}

	if len(toCreate) > 0 {
		if err := tx.CreateInBatches(toCreate, 200).Error; err != nil {
			return err
		}
		for _, rec := range toCreate {
			idByEmail[strings.ToLower(strings.TrimSpace(rec.Email))] = rec.Id
		}
	}

	links := make([]model.ClientInbound, 0, len(clients))
	linked := make(map[int]struct{}, len(clients))
	for i := range clients {
		email := strings.TrimSpace(clients[i].Email)
		if email == "" {
			continue
		}
		id, ok := idByEmail[strings.ToLower(email)]
		if !ok {
			continue
		}
		if _, dup := linked[id]; dup {
			continue
		}
		linked[id] = struct{}{}
		links = append(links, model.ClientInbound{
			ClientId:     id,
			InboundId:    inboundId,
			FlowOverride: clients[i].Flow,
		})
	}
	if len(links) > 0 {
		if err := tx.CreateInBatches(links, 200).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *ClientService) DetachInbound(tx *gorm.DB, inboundId int) error {
	if tx == nil {
		tx = database.GetDB()
	}
	return tx.Where("inbound_id = ?", inboundId).Delete(&model.ClientInbound{}).Error
}

func (s *ClientService) ListForInbound(tx *gorm.DB, inboundId int) ([]model.Client, error) {
	if tx == nil {
		tx = database.GetDB()
	}
	type joinedRow struct {
		model.ClientRecord
		FlowOverride string
	}
	var rows []joinedRow
	err := tx.Table("clients").
		Select("clients.*, client_inbounds.flow_override AS flow_override").
		Joins("JOIN client_inbounds ON client_inbounds.client_id = clients.id").
		Where("client_inbounds.inbound_id = ?", inboundId).
		Order("clients.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]model.Client, 0, len(rows))
	for i := range rows {
		c := rows[i].ToClient()
		c.Flow = rows[i].FlowOverride
		out = append(out, *c)
	}
	return out, nil
}
