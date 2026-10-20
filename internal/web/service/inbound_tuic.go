package service

import (
	"context"

	"gorm.io/gorm"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/logger"
	"github.com/Dark-Sky07/phoenix/internal/tuic"
	"github.com/Dark-Sky07/phoenix/internal/xray"
)

func trafficDisabledEmails(db *gorm.DB, emails []string) (map[string]struct{}, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	var rows []xray.ClientTraffic
	if err := db.Model(xray.ClientTraffic{}).
		Where("email IN ? AND enable = ?", emails, false).
		Select("email").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		out[r.Email] = struct{}{}
	}
	return out, nil
}

func (s *InboundService) DesiredTuicInstances() ([]tuic.Instance, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).
		Where("protocol = ? AND enable = ? AND node_id IS NULL", model.TUIC, true).
		Find(&inbounds).Error
	if err != nil {
		return nil, err
	}
	if len(inbounds) == 0 {
		return nil, nil
	}

	instances := make([]tuic.Instance, 0, len(inbounds))
	for _, ib := range inbounds {
		built, err := s.buildInboundForLocalRuntime(db, ib)
		if err != nil {
			return nil, err
		}
		inst, ok := tuic.InstanceFromInbound(built)
		if !ok {
			continue
		}
		instances = append(instances, inst)
	}
	emails := make([]string, 0)
	for _, inst := range instances {
		for _, e := range inst.Clients {
			emails = append(emails, e.Email)
		}
	}
	disabled, err := trafficDisabledEmails(db, emails)
	if err != nil {
		return nil, err
	}
	served := instances[:0]
	for _, inst := range instances {
		kept := make([]tuic.TuicClientSettings, 0, len(inst.Clients))
		for _, e := range inst.Clients {
			if _, off := disabled[e.Email]; !off {
				kept = append(kept, e)
			}
		}
		inst.Clients = kept
		if len(kept) > 0 {
			served = append(served, inst)
		}
	}
	return served, nil
}

func (s *InboundService) applyLocalTuic(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.TUIC || inbound.NodeID != nil {
		return
	}
	rt, err := s.runtimeFor(inbound)
	if err != nil {
		return
	}
	payload := inbound
	if inbound.Enable {
		if built, bErr := s.buildInboundForLocalRuntime(database.GetDB(), inbound); bErr == nil {
			payload = built
		}
	}
	if err := rt.UpdateInbound(context.Background(), inbound, payload); err != nil {
		logger.Debug("tuic: immediate client apply failed for inbound", inboundId, ":", err)
	}
}
