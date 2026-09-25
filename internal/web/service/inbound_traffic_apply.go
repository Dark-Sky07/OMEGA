package service

import (
	"context"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/mtproto"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

type trafficLocalApplyAction uint8

const (
	trafficAddUser trafficLocalApplyAction = iota + 1
	trafficRemoveUser
	trafficDisableInbound
)

type trafficLocalApplyPlan struct {
	action  trafficLocalApplyAction
	inbound model.Inbound
	client  map[string]any
	email   string
}

type trafficMutationBatch struct {
	localPlans  []trafficLocalApplyPlan
	remotePlans []trafficInboundUpdatePlan
	nodeIDs     map[int]struct{}
}

type trafficInboundUpdatePlan struct{ oldInbound, newInbound model.Inbound }

// applyLocalMtproto refreshes a single mtg sidecar after a quota or expiry
// mutation. The regular job remains the backstop, but immediate reconciliation
// keeps disable/reset behavior consistent with Xray and AmneziaWG clients.
func (s *InboundService) applyLocalMtproto(inboundID int) {
	inbound, err := s.GetInbound(inboundID)
	if err != nil || inbound == nil || inbound.Protocol != model.MTProto || inbound.NodeID != nil {
		return
	}
	inst, ok := mtproto.InstanceFromInbound(inbound)
	if !ok || !inbound.Enable {
		mtproto.GetManager().Remove(inboundID)
		return
	}
	if err := mtproto.GetManager().Ensure(inst); err != nil {
		logger.Debugf("mtproto: immediate apply failed for inbound %d: %v", inboundID, err)
	}
}

func newTrafficMutationBatch() *trafficMutationBatch {
	return &trafficMutationBatch{nodeIDs: make(map[int]struct{})}
}

func (b *trafficMutationBatch) addNode(nodeID int) {
	if nodeID > 0 {
		b.nodeIDs[nodeID] = struct{}{}
	}
}

func (b *trafficMutationBatch) markNodesTx(tx *gorm.DB) error {
	if b == nil {
		return nil
	}
	nodeSvc := NodeService{}
	for nodeID := range b.nodeIDs {
		if err := nodeSvc.MarkNodeDirtyTx(tx, nodeID); err != nil {
			return err
		}
	}
	return nil
}

func (s *InboundService) applyTrafficMutationBatch(b *trafficMutationBatch) bool {
	if b == nil {
		return false
	}
	needRestart := false
	for i := range b.remotePlans {
		plan := &b.remotePlans[i]
		rt, err := s.runtimeFor(&plan.newInbound)
		if err == nil {
			err = rt.UpdateInbound(context.Background(), &plan.oldInbound, &plan.newInbound)
		}
		if err != nil {
			logger.Debug("traffic post-commit remote apply failed:", err)
			needRestart = true
		}
	}
	for i := range b.localPlans {
		plan := &b.localPlans[i]
		if plan.inbound.Protocol == model.MTProto {
			s.applyLocalMtproto(plan.inbound.Id)
			continue
		}
		if plan.inbound.Protocol == model.AmneziaWG {
			s.applyLocalAmneziaWG(plan.inbound.Id)
			continue
		}
		rt, err := s.runtimeFor(&plan.inbound)
		if err == nil {
			switch plan.action {
			case trafficAddUser:
				err = rt.AddUser(context.Background(), &plan.inbound, plan.client)
			case trafficRemoveUser:
				err = rt.RemoveUser(context.Background(), &plan.inbound, plan.email)
				if err != nil && strings.Contains(err.Error(), "not found") {
					err = nil
				}
			case trafficDisableInbound:
				err = rt.DelInbound(context.Background(), &plan.inbound)
				if xray.IsMissingHandlerErr(err) {
					err = nil
				}
			}
		}
		if err != nil {
			logger.Debug("traffic post-commit runtime apply failed:", err)
			needRestart = true
		}
	}
	return needRestart
}
