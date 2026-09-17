package service

import (
	"net"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// openvpnProfileHost picks the address the client profile should dial,
// mirroring the subscription link host resolution: an explicit custom share
// address wins, then a routable listen address (for the "listen" strategy),
// then the panel's public host. OpenVPN inbounds are always local (no node),
// so the node-address branch of the link resolver does not apply.
func openvpnProfileHost(inbound *model.Inbound, fallbackHost string) string {
	var listenAddr string
	if listen := inbound.Listen; listen != "" && listen[0] != '@' && listen[0] != '/' {
		if ip := net.ParseIP(strings.Trim(listen, "[]")); ip != nil {
			if !ip.IsLoopback() && !ip.IsUnspecified() {
				listenAddr = listen
			}
		} else {
			listenAddr = listen
		}
	}
	switch inbound.ShareAddrStrategy {
	case "listen":
		if listenAddr != "" {
			return listenAddr
		}
	case "custom":
		if addr := strings.TrimSpace(inbound.ShareAddr); addr != "" {
			return addr
		}
	}
	if listenAddr != "" {
		return listenAddr
	}
	return fallbackHost
}

// GetOpenvpnProfile renders the .ovpn profile for a client's openvpn inbound.
// It returns the profile text plus the inbound it was generated from.
func (s *InboundService) GetOpenvpnProfile(fallbackHost, email string) (string, *model.Inbound, error) {
	if email == "" {
		return "", nil, common.NewError("openvpn: client email is required")
	}
	rec, err := s.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return "", nil, err
	}
	inboundIds, err := s.clientService.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return "", nil, err
	}
	var chosen *model.Inbound
	for _, id := range inboundIds {
		inbound, getErr := s.GetInbound(id)
		if getErr != nil {
			continue
		}
		if inbound.Protocol != model.OpenVPN {
			continue
		}
		if !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		chosen = inbound
		break
	}
	if chosen == nil {
		return "", nil, common.NewError("openvpn: client is not attached to an enabled local openvpn inbound")
	}
	inst, ok := openvpn.InstanceFromInbound(chosen, []string{email})
	if !ok {
		return "", nil, common.NewError("openvpn: inbound settings are invalid")
	}
	profile, err := openvpn.BuildProfile(inst, email, openvpnProfileHost(chosen, fallbackHost))
	if err != nil {
		return "", nil, err
	}
	return profile, chosen, nil
}

// GetOpenvpnInboundTags returns the tags of the enabled local openvpn inbounds
// attached to a client — used by the panel UI to decide whether to offer a
// config download for a client.
func (s *InboundService) GetOpenvpnInboundTagsForClient(email string) []string {
	rec, err := s.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return nil
	}
	inboundIds, err := s.clientService.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return nil
	}
	tags := make([]string, 0, len(inboundIds))
	for _, id := range inboundIds {
		inbound, getErr := s.GetInbound(id)
		if getErr != nil || inbound.Protocol != model.OpenVPN || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		tags = append(tags, inbound.Tag)
	}
	return tags
}
