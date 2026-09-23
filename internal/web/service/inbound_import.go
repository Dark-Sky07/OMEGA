package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
)

// PrepareInboundImport reconciles clients embedded in an inbound export with
// the destination's canonical client records without replacing existing
// credentials, limits, protocol fields, or timestamps. New clients remain in
// the payload and are created by AddInbound. Existing traffic rows are already
// protected by AddInbound's email-conflict insert, so importing an inbound does
// not reset destination usage.
func (s *InboundService) PrepareInboundImport(inbound *model.Inbound) error {
	if inbound == nil || inbound.Settings == "" {
		return nil
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return fmt.Errorf("invalid inbound settings: %w", err)
	}
	rawClients, ok := settings["clients"].([]any)
	if !ok {
		return nil
	}

	for index, raw := range rawClients {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("encode imported client %d: %w", index, err)
		}
		var imported model.Client
		if err := json.Unmarshal(encoded, &imported); err != nil {
			return fmt.Errorf("decode imported client %d: %w", index, err)
		}
		if imported.Email == "" {
			continue
		}
		existing, err := s.clientService.GetRecordByEmail(nil, imported.Email)
		if err != nil && (database.IsNotFound(err) || err == gorm.ErrRecordNotFound) {
			// Email identity is case-insensitive for import matching, while the
			// canonical spelling remains whatever the destination already stores.
			candidate := &model.ClientRecord{}
			lookupErr := database.GetDB().Where("LOWER(email) = LOWER(?)", strings.TrimSpace(imported.Email)).First(candidate).Error
			if lookupErr == nil {
				existing = candidate
				err = nil
			} else if database.IsNotFound(lookupErr) || lookupErr == gorm.ErrRecordNotFound {
				continue
			} else {
				return lookupErr
			}
		}
		if err != nil {
			return err
		}
		// The destination record remains authoritative even when the import
		// carries a different subId: importing an association must not rotate
		// a live identity or overwrite its credentials.
		canonical := existing.ToClient()
		// The inbound JSON carries the association's flow value. Preserve that
		// per-inbound override while taking every canonical client field from
		// the destination record; SyncInboundPreservingExisting will link the
		// association without writing the destination record back.
		canonical.Flow = imported.Flow
		// Keep the exact email spelling used by the destination record. All
		// other fields, including protocol credentials and per-client limits,
		// intentionally come from the existing record.
		rawClients[index] = canonical
	}
	settings["clients"] = rawClients
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode inbound settings: %w", err)
	}
	inbound.Settings = string(encoded)
	return nil
}
