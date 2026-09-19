package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

// This file mirrors the admin inbound listings for reseller (نمایندگی)
// sessions: identical shape and enrichment, but restricted to the inbounds the
// reseller owns.

// resellerInboundQuery is the ownership-filtered inbound query.
func resellerInboundQuery(db *gorm.DB, resellerId int) *gorm.DB {
	return db.Model(model.Inbound{}).
		Joins("JOIN reseller_inbounds ON reseller_inbounds.inbound_id = inbounds.id").
		Where("reseller_inbounds.reseller_id = ?", resellerId)
}

// GetInboundsForReseller lists every inbound owned by a reseller.
func (s *InboundService) GetInboundsForReseller(resellerId int) ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := resellerInboundQuery(db, resellerId).Preload("ClientStats").Order("inbounds.id ASC").Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	s.enrichClientStats(db, inbounds)
	s.annotateFallbackParents(db, inbounds)
	s.annotateLocalOriginGuid(inbounds)
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
	s.annotateFallbackParents(db, inbounds)
	s.annotateLocalOriginGuid(inbounds)
	s.backfillClientStats(db, inbounds)
	s.overlayInboundsClientStats(db, inbounds)
	for _, ib := range inbounds {
		ib.Settings = slimSettingsClients(ib.Settings)
	}
	return inbounds, nil
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
