package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// L2TPJob reconciles the single local L2TP/IPsec daemon group and its runtime
// FORWARD/MASQUERADE rules. PPP interface counters are folded into the normal
// inbound/client traffic pipeline; no synthetic Xray counters are generated.
type L2TPJob struct {
	inboundService service.InboundService
	clientService  service.ClientService
}

func NewL2TPJob() *L2TPJob { return new(L2TPJob) }

func (j *L2TPJob) enabledClientsForInbound(ib *model.Inbound) ([]model.Client, bool) {
	clients, err := j.clientService.ListForInbound(nil, ib.Id)
	if err != nil {
		logger.Warning("l2tp job: list clients failed:", err)
		// A database error must not make the daemon fall back to credentials
		// copied into inbound.settings from an older reconcile round, nor
		// should it make Reconcile([]) tear down a healthy current daemon.
		return nil, false
	}
	enableMap := make(map[string]bool, len(ib.ClientStats))
	for _, stat := range ib.ClientStats {
		enableMap[stat.Email] = stat.Enable
	}
	out := make([]model.Client, 0, len(clients))
	for _, client := range clients {
		if !client.Enable {
			continue
		}
		if enabled, exists := enableMap[client.Email]; exists && !enabled {
			continue
		}
		out = append(out, client)
	}
	return out, true
}

func (j *L2TPJob) Run() {
	inbounds, err := j.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("l2tp job: get inbounds failed:", err)
		return
	}
	wanted := make([]l2tp.Instance, 0, 1)
	for _, ib := range inbounds {
		if ib.Protocol != model.L2TP || !ib.Enable || ib.NodeID != nil {
			continue
		}
		clients, clientsOK := j.enabledClientsForInbound(ib)
		if !clientsOK {
			return
		}
		inst, ok := l2tp.InstanceFromInbound(ib, clients)
		if !ok {
			logger.Warningf("l2tp job: inbound %d has invalid settings or credentials", ib.Id)
			continue
		}
		wanted = append(wanted, inst)
	}
	mgr := l2tp.GetManager()
	mgr.Reconcile(wanted)
	inboundDeltas, clientDeltas := mgr.CollectTraffic()
	if len(inboundDeltas) == 0 && len(clientDeltas) == 0 {
		return
	}
	traffics := make([]*xray.Traffic, 0, len(inboundDeltas))
	for _, delta := range inboundDeltas {
		traffics = append(traffics, &xray.Traffic{
			IsInbound: true,
			Tag:       delta.Tag,
			Up:        delta.Up,
			Down:      delta.Down,
		})
	}
	clientTraffics := make([]*xray.ClientTraffic, 0, len(clientDeltas))
	for _, delta := range clientDeltas {
		clientTraffics = append(clientTraffics, &xray.ClientTraffic{
			InboundId: delta.InboundId,
			Email:     delta.Email,
			Enable:    true,
			Up:        delta.Up,
			Down:      delta.Down,
		})
	}
	if _, _, err := j.inboundService.AddTraffic(traffics, clientTraffics); err != nil {
		logger.Warning("l2tp job: add traffic failed:", err)
	}
}
