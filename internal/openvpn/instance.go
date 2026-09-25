// Package openvpn manages openvpn daemon processes that serve OpenVPN
// inbounds. Xray-core has no OpenVPN protocol, so openvpn inbounds are run as
// standalone openvpn server processes — one process per inbound — entirely
// outside the Xray config and lifecycle. The pattern mirrors the mtproto
// (mtg) sidecar manager: a reconcile job keeps the running set in sync with
// the database and folds daemon-reported traffic into the normal accounting.
//
// Every inbound gets its own data directory holding a self-signed CA, the
// server keypair and one keypair per attached client (CN = client email).
// Certificates are generated in pure Go, so no openssl is required.
package openvpn

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Instance is the desired runtime configuration of one openvpn inbound.
type Instance struct {
	Id     int
	Tag    string
	Listen string
	Port   int

	// Proto is the transport the daemon listens on: "udp" or "tcp".
	Proto string

	// RedirectGw pushes "redirect-gateway def1 bypass-dns" so the client's
	// whole default route goes through the tunnel.
	RedirectGw bool

	// PushDNS pushes the configured DNS servers to the clients.
	PushDNS bool
	DNS1    string
	DNS2    string

	// Clients are the emails of the clients attached to this inbound; each
	// becomes one certificate (CN = email) and one management-interface
	// identity, which is also how per-client traffic is attributed.
	Clients []string
}

// fingerprint changes whenever any value that ends up in the generated
// openvpn.conf — or the set of client certificates — changes, so reconcile
// restarts the daemon when the operator edits a setting or (de)attaches a
// client.
func (inst Instance) fingerprint() string {
	clients := make([]string, len(inst.Clients))
	copy(clients, inst.Clients)
	sort.Strings(clients)
	return strings.Join([]string{
		strconv.Itoa(inst.Port),
		inst.listenFor(),
		inst.protoFor(),
		strconv.FormatBool(inst.RedirectGw),
		strconv.FormatBool(inst.PushDNS),
		inst.dns1For(),
		inst.dns2For(),
		strings.Join(clients, ","),
	}, "|")
}

func (inst Instance) listenFor() string {
	if inst.Listen == "" || inst.Listen == "0.0.0.0" {
		return ""
	}
	return inst.Listen
}

func (inst Instance) protoFor() string {
	if strings.HasPrefix(inst.Proto, "tcp") {
		return "tcp"
	}
	return "udp"
}

func (inst Instance) dns1For() string {
	if inst.DNS1 != "" {
		return inst.DNS1
	}
	return "1.1.1.1"
}

func (inst Instance) dns2For() string {
	if inst.DNS2 != "" {
		return inst.DNS2
	}
	return "8.8.8.8"
}

// serverSubnet returns the deterministic /24 client pool for this inbound so
// two inbounds on the same host never hand out overlapping addresses.
func (inst Instance) serverSubnet() (network string, mask string) {
	n := inst.Id % 250
	o := inst.Id / 250
	if o > 255 {
		o = 255
	}
	return "10." + strconv.Itoa(n) + "." + strconv.Itoa(o) + ".0", "255.255.255.0"
}

func (inst Instance) serverSubnetCIDR() string {
	network, _ := inst.serverSubnet()
	return network + "/24"
}

// InstanceFromInbound derives a desired Instance from an openvpn inbound.
// Returns false when the inbound is not a usable openvpn inbound.
func InstanceFromInbound(ib *model.Inbound, clients []string) (Instance, bool) {
	if ib == nil || ib.Protocol != model.OpenVPN {
		return Instance{}, false
	}
	var parsed struct {
		Proto      string `json:"proto"`
		RedirectGw *bool  `json:"redirectGateway"`
		PushDNS    *bool  `json:"pushDNS"`
		DNS1       string `json:"dns1"`
		DNS2       string `json:"dns2"`
	}
	if ib.Settings != "" {
		if err := json.Unmarshal([]byte(ib.Settings), &parsed); err != nil {
			return Instance{}, false
		}
	}
	// Both toggles default on (full tunnel + pushed DNS); the pointer form
	// distinguishes an explicit false from a missing key.
	redirectGw := true
	if parsed.RedirectGw != nil {
		redirectGw = *parsed.RedirectGw
	}
	pushDNS := true
	if parsed.PushDNS != nil {
		pushDNS = *parsed.PushDNS
	}
	return Instance{
		Id:         ib.Id,
		Tag:        ib.Tag,
		Listen:     ib.Listen,
		Port:       ib.Port,
		Proto:      parsed.Proto,
		RedirectGw: redirectGw,
		PushDNS:    pushDNS,
		DNS1:       parsed.DNS1,
		DNS2:       parsed.DNS2,
		Clients:    clients,
	}, true
}
