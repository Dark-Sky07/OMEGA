package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// persistTunnelAllowedIPs applies an explicit per-inbound address override to
// the inbound settings after the legacy client update path has handled the
// shared client record and runtime identity. The override is intentionally
// keyed by email and inbound id: a single ClientRecord.AllowedIPs value cannot
// represent a client attached to both native WireGuard and AmneziaWG.
func (s *ClientService) persistTunnelAllowedIPs(inboundSvc *InboundService, inboundID int, email string, allowedIPs []string) error {
	defer lockInbound(inboundID).Unlock()

	inbound, err := inboundSvc.GetInbound(inboundID)
	if err != nil {
		return err
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return err
	}
	clients := clientsFromSettings(settings)
	found := false
	for _, raw := range clients {
		client, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		storedEmail, _ := client["email"].(string)
		if !strings.EqualFold(strings.TrimSpace(storedEmail), strings.TrimSpace(email)) {
			continue
		}
		client["allowedIPs"] = append([]string(nil), allowedIPs...)
		found = true
		break
	}
	if !found {
		return fmt.Errorf("client %q not found on inbound %d", email, inboundID)
	}

	settings["clients"] = clients
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	inbound.Settings = string(encoded)
	db := database.GetDB()
	if err := db.Save(inbound).Error; err != nil {
		return err
	}
	finalClients, err := inboundSvc.GetClients(inbound)
	if err != nil {
		return err
	}
	if err := s.SyncInbound(db, inboundID, finalClients); err != nil {
		return err
	}
	if inbound.Protocol == model.AmneziaWG && inbound.NodeID == nil {
		inboundSvc.applyLocalAmneziaWG(inboundID)
	}
	return nil
}
