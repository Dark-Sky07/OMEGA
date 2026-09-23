// Package l2tp manages the strongSwan and xl2tpd daemons that implement
// L2TP/IPsec inbounds. L2TP/IPsec is deliberately kept outside Xray: the
// kernel owns the IPsec policies and pppd owns the per-client interfaces.
//
// There is one global L2TP listener per panel. The listener uses the fixed
// UDP ports 500, 4500 and 1701, so the service layer rejects a second L2TP
// inbound and prevents other inbounds from claiming those ports.
package l2tp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	DefaultPort       = 1701
	DefaultPoolCIDR   = "10.252.0.0/24"
	DefaultLocalIP    = "10.252.0.1"
	DefaultPoolStart  = "10.252.0.10"
	DefaultPoolEnd    = "10.252.0.250"
	DefaultDNS1       = "1.1.1.1"
	DefaultDNS2       = "8.8.8.8"
	DefaultListenAddr = "0.0.0.0"
)

// FixedPorts are claimed by a global L2TP/IPsec server. Port 1701 is the
// L2TP control/data port; 500 and 4500 are IKE and NAT-T respectively.
var FixedPorts = [...]int{500, 4500, 1701}

// Credential is one PPP/MS-CHAPv2 account. The username is the existing
// panel client's email and Password is the existing client's Password field.
type Credential struct {
	Email    string
	Password string
}

// Instance is the desired runtime configuration of the single global server.
type Instance struct {
	Id                int
	Tag               string
	Listen            string
	Port              int
	PSK               string
	PoolCIDR          string
	LocalIP           string
	PoolStart         string
	PoolEnd           string
	DNS1              string
	DNS2              string
	OutboundInterface string
	RedirectGateway   bool
	Credentials       []Credential
}

// fingerprint covers every value emitted into the daemon or firewall
// configuration. Passwords are intentionally included so a client password
// edit causes chap-secrets to be reconciled immediately.
func (inst Instance) fingerprint() string {
	creds := make([]string, 0, len(inst.Credentials))
	for _, c := range inst.Credentials {
		creds = append(creds, c.Email+"\x00"+c.Password)
	}
	sort.Strings(creds)
	return strings.Join([]string{
		strconv.Itoa(inst.Id),
		inst.Listen,
		strconv.Itoa(inst.Port),
		inst.PSK,
		inst.PoolCIDR,
		inst.LocalIP,
		inst.PoolStart,
		inst.PoolEnd,
		inst.DNS1,
		inst.DNS2,
		inst.OutboundInterface,
		strconv.FormatBool(inst.RedirectGateway),
		strings.Join(creds, "\x01"),
	}, "|")
}

func (inst Instance) listenFor() string {
	if strings.TrimSpace(inst.Listen) == "" {
		return DefaultListenAddr
	}
	return strings.TrimSpace(inst.Listen)
}

func (inst Instance) portFor() int {
	if inst.Port > 0 {
		return inst.Port
	}
	return DefaultPort
}

func (inst Instance) poolCIDRFor() string {
	if strings.TrimSpace(inst.PoolCIDR) != "" {
		return strings.TrimSpace(inst.PoolCIDR)
	}
	return DefaultPoolCIDR
}

func (inst Instance) localIPFor() string {
	if strings.TrimSpace(inst.LocalIP) != "" {
		return strings.TrimSpace(inst.LocalIP)
	}
	return DefaultLocalIP
}

func (inst Instance) poolStartFor() string {
	if strings.TrimSpace(inst.PoolStart) != "" {
		return strings.TrimSpace(inst.PoolStart)
	}
	return DefaultPoolStart
}

func (inst Instance) poolEndFor() string {
	if strings.TrimSpace(inst.PoolEnd) != "" {
		return strings.TrimSpace(inst.PoolEnd)
	}
	return DefaultPoolEnd
}

func (inst Instance) dns1For() string {
	if strings.TrimSpace(inst.DNS1) != "" {
		return strings.TrimSpace(inst.DNS1)
	}
	return DefaultDNS1
}

func (inst Instance) dns2For() string {
	if strings.TrimSpace(inst.DNS2) != "" {
		return strings.TrimSpace(inst.DNS2)
	}
	return DefaultDNS2
}

// InstanceFromInbound derives a desired daemon instance from a stored L2TP
// inbound and the enabled, attached clients. Invalid daemon settings return
// false so the reconcile job never writes a partially trusted configuration.
func InstanceFromInbound(ib *model.Inbound, clients []model.Client) (Instance, bool) {
	if ib == nil || ib.Protocol != model.L2TP {
		return Instance{}, false
	}
	var parsed struct {
		PSK               string `json:"psk"`
		PoolCIDR          string `json:"poolCIDR"`
		LocalIP           string `json:"localIP"`
		PoolStart         string `json:"poolStart"`
		PoolEnd           string `json:"poolEnd"`
		DNS1              string `json:"dns1"`
		DNS2              string `json:"dns2"`
		OutboundInterface string `json:"outboundInterface"`
		RedirectGateway   *bool  `json:"redirectGateway"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &parsed); err != nil {
		return Instance{}, false
	}
	if clients == nil {
		var stored struct {
			Clients []model.Client `json:"clients"`
		}
		if err := json.Unmarshal([]byte(ib.Settings), &stored); err == nil {
			clients = stored.Clients
		}
	}
	if parsed.PSK == "" {
		return Instance{}, false
	}
	redirectGateway := true
	if parsed.RedirectGateway != nil {
		redirectGateway = *parsed.RedirectGateway
	}

	creds := make([]Credential, 0, len(clients))
	for _, client := range clients {
		if !client.Enable || strings.TrimSpace(client.Email) == "" || client.Password == "" {
			continue
		}
		if strings.ContainsAny(client.Email, "\r\n") || strings.ContainsAny(client.Password, "\r\n") {
			return Instance{}, false
		}
		creds = append(creds, Credential{Email: client.Email, Password: client.Password})
	}
	return Instance{
		Id:                ib.Id,
		Tag:               ib.Tag,
		Listen:            ib.Listen,
		Port:              ib.Port,
		PSK:               parsed.PSK,
		PoolCIDR:          parsed.PoolCIDR,
		LocalIP:           parsed.LocalIP,
		PoolStart:         parsed.PoolStart,
		PoolEnd:           parsed.PoolEnd,
		DNS1:              parsed.DNS1,
		DNS2:              parsed.DNS2,
		OutboundInterface: parsed.OutboundInterface,
		RedirectGateway:   redirectGateway,
		Credentials:       creds,
	}, true
}

// GeneratePSK returns a printable 32-byte hexadecimal pre-shared key. The
// value is stored in inbound settings and never regenerated during reconcile.
func GeneratePSK() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Errorf("l2tp: crypto/rand read failed: %w", err))
	}
	return hex.EncodeToString(buf)
}

// ValidateSettings verifies the values used by strongSwan, xl2tpd and the
// firewall before a configuration is rendered. It is intentionally strict
// about IPv4 because this implementation only enables IPv4 forwarding/NAT.
func ValidateSettings(inst Instance) error {
	if inst.portFor() != DefaultPort {
		return fmt.Errorf("l2tp: port must be %d", DefaultPort)
	}
	for name, raw := range map[string]string{
		"poolCIDR":  inst.poolCIDRFor(),
		"localIP":   inst.localIPFor(),
		"poolStart": inst.poolStartFor(),
		"poolEnd":   inst.poolEndFor(),
	} {
		if ip := net.ParseIP(strings.TrimSpace(raw)); name != "poolCIDR" && (ip == nil || ip.To4() == nil) {
			return fmt.Errorf("l2tp: %s must be an IPv4 address", name)
		}
		if name == "poolCIDR" {
			ip, network, err := net.ParseCIDR(raw)
			if err != nil || ip.To4() == nil || network.IP.To4() == nil {
				return fmt.Errorf("l2tp: poolCIDR must be an IPv4 CIDR")
			}
		}
	}
	if inst.PSK == "" || strings.ContainsAny(inst.PSK, "\r\n\"") {
		return fmt.Errorf("l2tp: invalid pre-shared key")
	}
	for name, raw := range map[string]string{"dns1": inst.dns1For(), "dns2": inst.dns2For()} {
		ip := net.ParseIP(raw)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("l2tp: %s must be an IPv4 address", name)
		}
	}
	iface := strings.TrimSpace(inst.OutboundInterface)
	if len(iface) > 15 || strings.ContainsAny(iface, "\r\n \t'\"") {
		return fmt.Errorf("l2tp: invalid outbound interface")
	}
	for _, r := range iface {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("._+-", r) {
			continue
		}
		return fmt.Errorf("l2tp: invalid outbound interface")
	}
	listen := strings.TrimSpace(inst.listenFor())
	if ip := net.ParseIP(listen); ip == nil || ip.To4() == nil {
		return fmt.Errorf("l2tp: listen must be an IPv4 address")
	}
	start := net.ParseIP(inst.poolStartFor()).To4()
	end := net.ParseIP(inst.poolEndFor()).To4()
	if start == nil || end == nil || compareIPv4(start, end) > 0 {
		return fmt.Errorf("l2tp: poolStart must not be after poolEnd")
	}
	_, network, _ := net.ParseCIDR(inst.poolCIDRFor())
	local := net.ParseIP(inst.localIPFor()).To4()
	if network != nil && (!network.Contains(start) || !network.Contains(end) || local == nil || !network.Contains(local)) {
		return fmt.Errorf("l2tp: localIP and pool range must be inside poolCIDR")
	}
	if local.Equal(start) || local.Equal(end) {
		return fmt.Errorf("l2tp: localIP must not be a pool address")
	}
	for _, c := range inst.Credentials {
		if strings.TrimSpace(c.Email) == "" || c.Password == "" ||
			strings.ContainsAny(c.Email, "\r\n*\t") || strings.ContainsAny(c.Password, "\r\n") {
			return fmt.Errorf("l2tp: invalid PPP credential")
		}
	}
	return nil
}

func compareIPv4(a, b net.IP) int {
	for i := 0; i < net.IPv4len; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
