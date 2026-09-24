package service

import (
	"encoding/json"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
	"gorm.io/gorm"
)

func (s *InboundService) annotateResellerFallbackParents(db *gorm.DB, inbounds []*model.Inbound, resellerId int) {
	s.annotateFallbackParents(db, inbounds)
	if len(inbounds) == 0 {
		return
	}
	var ownedMasters []int
	if err := db.Model(model.ResellerInbound{}).
		Where("reseller_id = ?", resellerId).
		Pluck("inbound_id", &ownedMasters).Error; err != nil {
		for _, inbound := range inbounds {
			inbound.FallbackParent = nil
		}
		return
	}
	allowed := make(map[int]struct{}, len(ownedMasters))
	for _, id := range ownedMasters {
		allowed[id] = struct{}{}
	}
	for _, inbound := range inbounds {
		if inbound.FallbackParent == nil {
			continue
		}
		if _, ok := allowed[inbound.FallbackParent.MasterId]; !ok {
			// A reseller may see the owned child inbound, but not an unrelated
			// admin-owned fallback parent or its link-rewrite metadata.
			inbound.FallbackParent = nil
		}
	}
}

// This file mirrors the admin inbound listings for reseller (نمایندگی)
// sessions: identical shape and enrichment, but restricted to the inbounds the
// reseller owns.

// resellerInboundQuery is the ownership-filtered inbound query.
func resellerInboundQuery(db *gorm.DB, resellerId int) *gorm.DB {
	return db.Model(model.Inbound{}).
		Joins("JOIN reseller_inbounds ON reseller_inbounds.inbound_id = inbounds.id").
		Where("reseller_inbounds.reseller_id = ?", resellerId)
}

// explicitResellerClientSet is intentionally separate from the inbound
// ownership query. Reseller inbound access is an inbound-level capability;
// client secrets, limits, and traffic rows require a ResellerClient mapping.
func explicitResellerClientSet(db *gorm.DB, resellerId int) (map[string]struct{}, error) {
	var emails []string
	if err := db.Model(model.ResellerClient{}).
		Where("reseller_id = ?", resellerId).
		Pluck("email", &emails).Error; err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		if key := strings.ToLower(strings.TrimSpace(email)); key != "" {
			allowed[key] = struct{}{}
		}
	}
	return allowed, nil
}

// filterResellerInboundClients removes every client representation that is
// not explicitly mapped to the reseller. Traffic/stat rows additionally need
// an isolated association because ClientTraffic is an email-level aggregate;
// a shared client cannot safely expose usage from an admin-owned inbound.
func filterResellerInboundClients(inbounds []*model.Inbound, settingsAllowed map[string]struct{}, statsAllowed ...map[string]struct{}) {
	stats := settingsAllowed
	if len(statsAllowed) > 0 && statsAllowed[0] != nil {
		stats = statsAllowed[0]
	}
	for _, inbound := range inbounds {
		inbound.ClientStats = filterResellerClientStats(inbound.ClientStats, stats)
		inbound.Settings = filterResellerSettingsClients(inbound.Settings, settingsAllowed)
	}
}

func filterResellerClientStats(stats []xray.ClientTraffic, allowed map[string]struct{}) []xray.ClientTraffic {
	out := make([]xray.ClientTraffic, 0, len(stats))
	for _, stat := range stats {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(stat.Email))]; ok {
			out = append(out, stat)
		}
	}
	return out
}

// filterResellerSettingsClients preserves the inbound's non-client settings
// while replacing clients with the explicitly visible subset. If a malformed
// settings blob cannot be safely parsed, return an empty client list rather
// than leaking the raw blob in a reseller response.
func filterResellerSettingsClients(settings string, allowed map[string]struct{}) string {
	if strings.TrimSpace(settings) == "" {
		return settings
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(settings), &raw); err != nil || raw == nil {
		return `{"clients":[]}`
	}
	clients, hasClients := raw["clients"]
	if !hasClients {
		return settings
	}
	entries, ok := clients.([]any)
	if !ok {
		raw["clients"] = []any{}
	} else {
		visible := make([]any, 0, len(entries))
		for _, entry := range entries {
			client, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			email, _ := client["email"].(string)
			if _, keep := allowed[strings.ToLower(strings.TrimSpace(email))]; keep {
				visible = append(visible, client)
			}
		}
		raw["clients"] = visible
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return `{"clients":[]}`
	}
	return string(encoded)
}

// GetInboundsForReseller lists every inbound owned by a reseller.
func (s *InboundService) GetInboundsForReseller(resellerId int) ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := resellerInboundQuery(db, resellerId).Preload("ClientStats").Order("inbounds.id ASC").Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	allowed, err := explicitResellerClientSet(db, resellerId)
	if err != nil {
		return nil, err
	}
	statsAllowed, err := (&ResellerService{}).OwnedIsolatedAssociatedEmailSet(resellerId)
	if err != nil {
		return nil, err
	}
	s.enrichClientStats(db, inbounds)
	s.annotateResellerFallbackParents(db, inbounds, resellerId)
	s.annotateLocalOriginGuid(inbounds)
	filterResellerInboundClients(inbounds, allowed, statsAllowed)
	return inbounds, nil
}

// GetInboundsSlimForReseller is the list-page variant of GetInboundsForReseller.
func (s *InboundService) GetInboundsSlimForReseller(resellerId int) ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := resellerInboundQuery(db, resellerId).Preload("ClientStats").Order("inbounds.id ASC").Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	allowed, err := explicitResellerClientSet(db, resellerId)
	if err != nil {
		return nil, err
	}
	statsAllowed, err := (&ResellerService{}).OwnedIsolatedAssociatedEmailSet(resellerId)
	if err != nil {
		return nil, err
	}
	s.annotateResellerFallbackParents(db, inbounds, resellerId)
	s.annotateLocalOriginGuid(inbounds)
	s.backfillClientStats(db, inbounds)
	s.overlayInboundsClientStats(db, inbounds)
	filterResellerInboundClients(inbounds, allowed, statsAllowed)
	for _, ib := range inbounds {
		ib.Settings = slimSettingsClients(ib.Settings)
	}
	return inbounds, nil
}

// GetInboundDetailForReseller returns a full inbound configuration while
// applying the same explicit-only client filtering as list/detail payloads.
func (s *InboundService) GetInboundDetailForReseller(resellerId, inboundId int) (*model.Inbound, error) {
	db := database.GetDB()
	var owned model.Inbound
	if err := resellerInboundQuery(db, resellerId).
		Where("inbounds.id = ?", inboundId).
		First(&owned).Error; err != nil {
		return nil, err
	}
	inbound, err := s.GetInboundDetail(inboundId)
	if err != nil {
		return nil, err
	}
	allowed, err := explicitResellerClientSet(db, resellerId)
	if err != nil {
		return nil, err
	}
	statsAllowed, err := (&ResellerService{}).OwnedIsolatedAssociatedEmailSet(resellerId)
	if err != nil {
		return nil, err
	}
	s.annotateResellerFallbackParents(db, []*model.Inbound{inbound}, resellerId)
	s.annotateLocalOriginGuid([]*model.Inbound{inbound})
	filterResellerInboundClients([]*model.Inbound{inbound}, allowed, statsAllowed)
	return inbound, nil
}

// GetInboundOptionsForReseller is the picker projection for a reseller.
func (s *InboundService) GetInboundOptionsForReseller(resellerId int) ([]InboundOption, error) {
	db := database.GetDB()
	var rows []struct {
		Id             int    `gorm:"column:id"`
		Remark         string `gorm:"column:remark"`
		Tag            string `gorm:"column:tag"`
		Protocol       string `gorm:"column:protocol"`
		Port           int    `gorm:"column:port"`
		StreamSettings string `gorm:"column:stream_settings"`
		Settings       string `gorm:"column:settings"`
		NodeId         *int   `gorm:"column:node_id"`
	}
	err := resellerInboundQuery(db, resellerId).
		Select("inbounds.id, inbounds.remark, inbounds.tag, inbounds.protocol, inbounds.port, inbounds.stream_settings, inbounds.settings, inbounds.node_id").
		Order("inbounds.id ASC").
		Scan(&rows).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	out := make([]InboundOption, 0, len(rows))
	for _, r := range rows {
		option := InboundOption{
			Id:             r.Id,
			Remark:         r.Remark,
			Tag:            r.Tag,
			Protocol:       r.Protocol,
			Port:           r.Port,
			TlsFlowCapable: inboundCanEnableTlsFlow(r.Protocol, r.StreamSettings, r.Settings),
			SsMethod:       inboundShadowsocksMethod(r.Protocol, r.Settings),
			NodeId:         r.NodeId,
		}
		if r.Protocol == string(model.L2TP) {
			option.L2TP = l2tpInboundOption(r.Settings)
		}
		out = append(out, option)
	}
	return out, nil
}
