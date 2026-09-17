package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// OpenvpnJob reconciles the running openvpn daemon processes against the
// enabled openvpn inbounds in the database, restarts any that crashed or
// changed, and folds the per-client traffic read from each daemon's
// management interface into the usual inbound/client traffic accounting.
// It mirrors MtprotoJob: the daemons live entirely outside the Xray config.
type OpenvpnJob struct {
	inboundService service.InboundService
	clientService  service.ClientService
}

// NewOpenvpnJob creates a new openvpn reconcile/traffic job instance.
// Zero-value services are fine: like the other jobs, this one resolves
// state through the global database handle inside the service methods.
func NewOpenvpnJob() *OpenvpnJob {
	return new(OpenvpnJob)
}

// enabledClientsForInbound returns the emails of the clients attached to the
// inbound that are actually allowed to connect — mirroring the exact
// enable-logic of the Xray config renderer (client record Enable plus the
// shared client-traffic row, where the auto-disable job records quota and
// expiry violations). A disabled client's certificate is removed, so it
// cannot connect even if it still holds an old profile.
func (j *OpenvpnJob) enabledClientsForInbound(ib *model.Inbound) []string {
	dbClients, err := j.clientService.ListForInbound(nil, ib.Id)
	if err != nil {
		logger.Warning("openvpn job: list clients for inbound", ib.Id, "failed:", err)
		return nil
	}
	enableMap := make(map[string]bool, len(ib.ClientStats))
	for _, ct := range ib.ClientStats {
		enableMap[ct.Email] = ct.Enable
	}
	emails := make([]string, 0, len(dbClients))
	for i := range dbClients {
		c := dbClients[i]
		if enable, exists := enableMap[c.Email]; exists && !enable {
			continue
		}
		if !c.Enable {
			continue
		}
		emails = append(emails, c.Email)
	}
	return emails
}

// Run reconciles desired openvpn inbounds with running daemons and records
// traffic deltas.
func (j *OpenvpnJob) Run() {
	inbounds, err := j.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("openvpn job: get inbounds failed:", err)
		return
	}

	var desired []openvpn.Instance
	for _, ib := range inbounds {
		if ib.Protocol != model.OpenVPN || !ib.Enable || ib.NodeID != nil {
			continue
		}
		inst, ok := openvpn.InstanceFromInbound(ib, j.enabledClientsForInbound(ib))
		if ok {
			desired = append(desired, inst)
		}
	}

	mgr := openvpn.Manager()
	mgr.Reconcile(desired)

	inboundDeltas, clientDeltas := mgr.CollectTraffic()
	if len(inboundDeltas) == 0 && len(clientDeltas) == 0 {
		return
	}
	traffics := make([]*xray.Traffic, 0, len(inboundDeltas))
	for _, d := range inboundDeltas {
		traffics = append(traffics, &xray.Traffic{
			IsInbound: true,
			Tag:       d.Tag,
			Up:        d.Up,
			Down:      d.Down,
		})
	}
	clientTraffics := make([]*xray.ClientTraffic, 0, len(clientDeltas))
	for _, d := range clientDeltas {
		clientTraffics = append(clientTraffics, &xray.ClientTraffic{
			InboundId: d.InboundId,
			Email:     d.Email,
			Enable:    true,
			Up:        d.Up,
			Down:      d.Down,
		})
	}
	_, _, err = j.inboundService.AddTraffic(traffics, clientTraffics)
	if err != nil {
		logger.Warning("openvpn job: add traffic failed:", err)
	}
}
