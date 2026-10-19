package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/util/tproxy"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"go.uber.org/atomic"
)

// Daemon relay: routing OpenVPN and L2TP/IPsec traffic through the core.
//
// Those two protocols are served by host daemons, not by Xray. For every
// enabled local OpenVPN/L2TP inbound that opts into Xray routing (or is
// referenced by an inboundTag rule in RouterConfig), the generated config
// receives:
//  1. A loopback TCP dokodemo-door inbound in "redirect" mode (SO_ORIGINAL_DST)
//     tagged with the inbound's own tag, with sniffing routeOnly=true so both
//     domain and IP routing rules match.
//  2. A companion loopback UDP dokodemo-door inbound on the same port tagged
//     with "<tag>:dns" that forwards DNS (UDP/53) queries through Xray's
//     router to the inbound's configured DNS server (default 1.1.1.1:53).
//
// The daemons' network managers redirect TCP (1-65535) and UDP (53) traffic
// from the VPN subnet into 127.0.0.1:<port> via iptables nat PREROUTING DNAT,
// and drop QUIC (UDP/443) in FORWARD so browsers fall back to TCP/443.

const (
	// daemonRelayBasePort is the first loopback port used for relay
	// listeners: inbound id N gets daemonRelayBasePort+N, bumped past any
	// port another generated inbound already uses.
	daemonRelayBasePort = 63900

	// daemonRelayDokodemoSettings makes the TCP listener recover the kernel's
	// original pre-DNAT destination via SO_ORIGINAL_DST.
	daemonRelayDokodemoSettings = `{"network":"tcp","followRedirect":true}`
	// daemonRelayStreamSettings turns the TCP listener into a REDIRECT/DNAT
	// target (SO_ORIGINAL_DST). Also the signature used to recognise primary
	// relay inbounds in the running config.
	daemonRelayStreamSettings = `{"sockopt":{"tproxy":"redirect"}}`
	// daemonRelaySniffingSettings enables HTTP/TLS/QUIC/FakeDNS sniffing with
	// routeOnly=true so Xray's router matches both sniffed domains and the
	// original destination IP while dialing the real IP if no proxy re-resolves.
	daemonRelaySniffingSettings = `{"enabled":true,"destOverride":["http","tls","quic","fakedns"],"routeOnly":true}`

	daemonRelayDNSStreamSettings   = `{"sockopt":{}}`
	daemonRelayDNSSniffingSettings = `{"enabled":false}`
	daemonRelayDNSTagSuffix        = ":dns"

	// daemonRelayCrashWindow bounds how soon after start a core death counts
	// as "died on startup" for the crash-loop guard.
	daemonRelayCrashWindow = 30 * time.Second
)

var (
	// daemonRelaySuspended is the crash-loop guard's fail-open switch: while
	// set, no relay inbounds are generated. Lifted by a forced restart.
	daemonRelaySuspended atomic.Bool
	// daemonRelayStrikes counts consecutive startup crashes with relay
	// inbounds present. Guarded by the package-level lock.
	daemonRelayStrikes int
	// daemonRelayUnsupportedLogged de-duplicates the capability warning.
	daemonRelayUnsupportedLogged atomic.Bool
)

// daemonRelayListen is the JSON form of the relay's listen address.
var daemonRelayListen = json_util.RawMessage(`"` + tproxy.ListenIP + `"`)

// daemonRelaySupported is the capability probe; a variable so tests can run
// the injection logic on hosts without loopback bind privileges.
var daemonRelaySupported = tproxy.Supported

// isDaemonRelayInbound reports whether a generated inbound is one of the
// relay listeners produced by injectDaemonRelays (either the primary TCP
// listener or its companion :dns UDP listener).
func isDaemonRelayInbound(ib xray.InboundConfig) bool {
	if ib.Protocol != "dokodemo-door" || !bytes.Equal(ib.Listen, daemonRelayListen) {
		return false
	}
	if ib.Port < daemonRelayBasePort || ib.Port > 65535 {
		return false
	}
	if bytes.Equal(ib.StreamSettings, json_util.RawMessage(daemonRelayStreamSettings)) {
		return true
	}
	if strings.HasSuffix(ib.Tag, daemonRelayDNSTagSuffix) &&
		bytes.Equal(ib.StreamSettings, json_util.RawMessage(daemonRelayDNSStreamSettings)) {
		return true
	}
	return false
}

// configHasDaemonRelay reports whether cfg carries any relay inbound.
func configHasDaemonRelay(cfg *xray.Config) bool {
	if cfg == nil {
		return false
	}
	for i := range cfg.InboundConfigs {
		if isDaemonRelayInbound(cfg.InboundConfigs[i]) {
			return true
		}
	}
	return false
}

// daemonRelayPortFor picks the loopback port for an inbound id, skipping
// ports already claimed by other inbounds in the generated config.
func daemonRelayPortFor(id int, used map[int]struct{}) int {
	if id <= 0 {
		return 0
	}
	port := daemonRelayBasePort + id
	if port > 65535 {
		port = daemonRelayBasePort
	}
	for ; port <= 65535; port++ {
		if _, taken := used[port]; !taken {
			return port
		}
	}
	return 0
}

// daemonRelayPortAvailable makes sure nothing else on the host owns the
// loopback port before Xray is asked to bind it.
func daemonRelayPortAvailable(port int) bool {
	if p != nil && p.IsRunning() {
		if cfg := p.GetConfig(); cfg != nil {
			for i := range cfg.InboundConfigs {
				if cfg.InboundConfigs[i].Port == port && isDaemonRelayInbound(cfg.InboundConfigs[i]) {
					return true
				}
			}
		}
	}
	addr := net.JoinHostPort(tproxy.ListenIP, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return false
	}
	_ = pc.Close()
	return true
}

func daemonInboundDNS1(inbound *model.Inbound) string {
	if inbound != nil && strings.TrimSpace(inbound.Settings) != "" {
		var parsed struct {
			DNS1 string `json:"dns1"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &parsed); err == nil {
			ip := net.ParseIP(strings.TrimSpace(parsed.DNS1))
			if ip != nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return "1.1.1.1"
}

func daemonRelayDNSDokodemoSettings(inbound *model.Inbound) string {
	return fmt.Sprintf(`{"address":%q,"port":53,"network":"udp","followRedirect":false}`, daemonInboundDNS1(inbound))
}

func daemonInboundOutboundTag(inbound *model.Inbound) string {
	if inbound == nil || strings.TrimSpace(inbound.Settings) == "" {
		return ""
	}
	var parsed struct {
		OutboundTag string `json:"outboundTag"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.OutboundTag)
}

func routingReferencesInboundTag(raw json_util.RawMessage, tag string) bool {
	if tag == "" || len(raw) == 0 {
		return false
	}
	var routing map[string]any
	if err := json.Unmarshal(raw, &routing); err != nil {
		return false
	}
	rules, _ := routing["rules"].([]any)
	for _, r := range rules {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if ruleContainsInboundTag(rm, tag) {
			return true
		}
	}
	return false
}

func ruleContainsInboundTag(rule map[string]any, tag string) bool {
	tags, ok := rule["inboundTag"].([]any)
	if !ok {
		return false
	}
	for _, item := range tags {
		if s, ok := item.(string); ok && s == tag {
			return true
		}
	}
	return false
}

func injectDaemonRelayRouting(cfg *xray.Config, tag string, outboundTag string) {
	if cfg == nil || tag == "" {
		return
	}
	dnsTag := tag + daemonRelayDNSTagSuffix
	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			logger.Warning("daemon relay: routing section is unparsable, skipping routing update:", err)
			return
		}
	}
	rules, _ := routing["rules"].([]any)
	changed := false
	lastMatchingIdx := -1
	for idx, r := range rules {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if !ruleContainsInboundTag(rm, tag) {
			continue
		}
		lastMatchingIdx = idx
		if !ruleContainsInboundTag(rm, dnsTag) {
			tags, _ := rm["inboundTag"].([]any)
			rm["inboundTag"] = append(tags, dnsTag)
			changed = true
		}
	}
	if outboundTag != "" {
		rule := map[string]any{
			"type":       "field",
			"inboundTag": []any{tag, dnsTag},
		}
		if routingTagIsBalancer(routing, outboundTag) {
			rule["balancerTag"] = outboundTag
		} else {
			rule["outboundTag"] = outboundTag
		}
		if lastMatchingIdx >= 0 {
			nextRules := make([]any, 0, len(rules)+1)
			nextRules = append(nextRules, rules[:lastMatchingIdx+1]...)
			nextRules = append(nextRules, rule)
			nextRules = append(nextRules, rules[lastMatchingIdx+1:]...)
			rules = nextRules
		} else {
			rules = append([]any{rule}, rules...)
		}
		routing["rules"] = rules
		changed = true
	}
	if changed {
		if newRouting, err := json.Marshal(routing); err == nil {
			cfg.RouterConfig = json_util.RawMessage(newRouting)
		} else {
			logger.Warning("daemon relay: failed to rebuild routing section:", err)
		}
	}
}

// injectDaemonRelays appends a loopback TCP dokodemo-door relay inbound and
// companion :dns UDP dokodemo-door relay inbound per enabled local OpenVPN or
// L2TP inbound that has opted into Xray routing or is referenced in
// RouterConfig.
func injectDaemonRelays(cfg *xray.Config, inbounds []*model.Inbound) {
	var wanted []*model.Inbound
	for _, inbound := range inbounds {
		if inbound == nil || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		if inbound.Protocol != model.OpenVPN && inbound.Protocol != model.L2TP {
			continue
		}
		if !daemonRelayRoutesThroughXray(inbound) && !routingReferencesInboundTag(cfg.RouterConfig, inbound.Tag) {
			continue
		}
		wanted = append(wanted, inbound)
	}
	if len(wanted) == 0 {
		return
	}
	if daemonRelaySuspended.Load() {
		return
	}
	if !daemonRelaySupported() {
		if daemonRelayUnsupportedLogged.CompareAndSwap(false, true) {
			logger.Warning("daemon relay: this process cannot bind loopback relay sockets; OpenVPN/L2TP traffic stays on the direct path and routing rules will not apply to it")
		}
		return
	}
	daemonRelayUnsupportedLogged.Store(false)

	existingTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	usedPorts := make(map[int]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		existingTags[cfg.InboundConfigs[i].Tag] = struct{}{}
		usedPorts[cfg.InboundConfigs[i].Port] = struct{}{}
	}
	for _, inbound := range wanted {
		if _, taken := existingTags[inbound.Tag]; taken {
			logger.Warning("daemon relay: inbound tag [", inbound.Tag, "] already present in generated config, skipping its relay inbound")
			continue
		}
		dnsTag := inbound.Tag + daemonRelayDNSTagSuffix
		if _, taken := existingTags[dnsTag]; taken {
			logger.Warning("daemon relay: DNS inbound tag [", dnsTag, "] already present in generated config, skipping its relay inbound")
			continue
		}
		port := daemonRelayPortFor(inbound.Id, usedPorts)
		if port == 0 {
			logger.Warning("daemon relay: no free loopback port for inbound [", inbound.Tag, "], skipping its relay inbound")
			continue
		}
		if !daemonRelayPortAvailable(port) {
			logger.Warning("daemon relay: loopback port ", port, " for inbound [", inbound.Tag, "] is in use by another process, skipping its relay inbound")
			continue
		}
		existingTags[inbound.Tag] = struct{}{}
		existingTags[dnsTag] = struct{}{}
		usedPorts[port] = struct{}{}
		cfg.InboundConfigs = append(cfg.InboundConfigs,
			xray.InboundConfig{
				Listen:         daemonRelayListen,
				Port:           port,
				Protocol:       "dokodemo-door",
				Settings:       json_util.RawMessage(daemonRelayDokodemoSettings),
				StreamSettings: json_util.RawMessage(daemonRelayStreamSettings),
				Sniffing:       json_util.RawMessage(daemonRelaySniffingSettings),
				Tag:            inbound.Tag,
			},
			xray.InboundConfig{
				Listen:         daemonRelayListen,
				Port:           port,
				Protocol:       "dokodemo-door",
				Settings:       json_util.RawMessage(daemonRelayDNSDokodemoSettings(inbound)),
				StreamSettings: json_util.RawMessage(daemonRelayDNSStreamSettings),
				Sniffing:       json_util.RawMessage(daemonRelayDNSSniffingSettings),
				Tag:            dnsTag,
			},
		)
		injectDaemonRelayRouting(cfg, inbound.Tag, daemonInboundOutboundTag(inbound))
	}
}

// DaemonRelayPort returns the loopback port of the relay listener the
// running core currently exposes for the inbound with this tag, or 0 when
// there is none (core stopped or not yet listening, feature off or
// suspended, inbound opted out).
func (s *XrayService) DaemonRelayPort(tag string) int {
	if tag == "" || !s.IsXrayRunning() {
		return 0
	}
	cfg := p.GetConfig()
	if cfg == nil {
		return 0
	}
	for i := range cfg.InboundConfigs {
		ib := cfg.InboundConfigs[i]
		if ib.Tag != tag || !isDaemonRelayInbound(ib) {
			continue
		}
		if !tproxy.ListenerBound(ib.Port) {
			return 0
		}
		return ib.Port
	}
	return 0
}

// SyncDaemonInbounds immediately reconciles the running OpenVPN and L2TP
// daemons and their iptables relay rules against the current database and
// running Xray relay listeners, so toggling Route through Xray or editing
// Routing Rules takes effect immediately instead of waiting for the next
// periodic job tick.
func (s *XrayService) SyncDaemonInbounds() {
	var inboundSvc InboundService
	var clientSvc ClientService
	inbounds, err := inboundSvc.GetAllInbounds()
	if err != nil {
		return
	}
	var desiredOvpn []openvpn.Instance
	var desiredL2TP []l2tp.Instance
	for _, ib := range inbounds {
		if ib == nil || !ib.Enable || ib.NodeID != nil {
			continue
		}
		switch ib.Protocol {
		case model.OpenVPN:
			dbClients, err := clientSvc.ListForInbound(nil, ib.Id)
			if err != nil {
				continue
			}
			enableMap := make(map[string]bool, len(ib.ClientStats))
			for _, ct := range ib.ClientStats {
				enableMap[strings.ToLower(strings.TrimSpace(ct.Email))] = ct.Enable
			}
			emails := make([]string, 0, len(dbClients))
			for i := range dbClients {
				c := dbClients[i]
				if enable, exists := enableMap[strings.ToLower(strings.TrimSpace(c.Email))]; exists && !enable {
					continue
				}
				if !c.Enable {
					continue
				}
				emails = append(emails, c.Email)
			}
			if inst, ok := openvpn.InstanceFromInbound(ib, emails); ok {
				inst.XrayRelayPort = s.DaemonRelayPort(ib.Tag)
				desiredOvpn = append(desiredOvpn, inst)
			}
		case model.L2TP:
			dbClients, err := clientSvc.ListForInbound(nil, ib.Id)
			if err != nil {
				continue
			}
			enableMap := make(map[string]bool, len(ib.ClientStats))
			for _, ct := range ib.ClientStats {
				enableMap[strings.ToLower(strings.TrimSpace(ct.Email))] = ct.Enable
			}
			clients := make([]model.Client, 0, len(dbClients))
			for _, c := range dbClients {
				if !c.Enable {
					continue
				}
				if enabled, exists := enableMap[strings.ToLower(strings.TrimSpace(c.Email))]; exists && !enabled {
					continue
				}
				clients = append(clients, c)
			}
			if inst, ok := l2tp.InstanceFromInbound(ib, clients); ok {
				inst.XrayRelayPort = s.DaemonRelayPort(ib.Tag)
				desiredL2TP = append(desiredL2TP, inst)
			}
		}
	}
	openvpn.GetManager().Reconcile(desiredOvpn)
	l2tp.GetManager().Reconcile(desiredL2TP)
}

// dropDaemonRelayTraffic removes the core's inbound-level counters for relay
// listeners (both the primary TCP inbound and the companion :dns UDP inbound).
func dropDaemonRelayTraffic(cfg *xray.Config, traffics []*xray.Traffic) []*xray.Traffic {
	if cfg == nil || len(traffics) == 0 {
		return traffics
	}
	relayTags := make(map[string]struct{})
	for i := range cfg.InboundConfigs {
		if isDaemonRelayInbound(cfg.InboundConfigs[i]) {
			relayTags[cfg.InboundConfigs[i].Tag] = struct{}{}
		}
	}
	if len(relayTags) == 0 {
		return traffics
	}
	kept := traffics[:0]
	for _, tr := range traffics {
		if tr != nil && tr.IsInbound {
			if _, relay := relayTags[tr.Tag]; relay {
				continue
			}
		}
		kept = append(kept, tr)
	}
	return kept
}

// noteDaemonRelayCrash updates the crash-loop guard before a RestartXray
// attempt.
func noteDaemonRelayCrash(isForce, crashed bool) {
	if isForce {
		daemonRelayStrikes = 0
		if daemonRelaySuspended.CompareAndSwap(true, false) {
			logger.Info("daemon relay: suspension cleared by forced Xray restart")
		}
		return
	}
	if !crashed {
		daemonRelayStrikes = 0
		return
	}
	if p == nil || !configHasDaemonRelay(p.GetConfig()) || p.GetUptime() >= uint64(daemonRelayCrashWindow/time.Second) {
		daemonRelayStrikes = 0
		return
	}
	daemonRelayStrikes++
	if daemonRelayStrikes >= 2 && daemonRelaySuspended.CompareAndSwap(false, true) {
		logger.Warning("daemon relay: Xray stopped right after starting twice in a row with relay inbounds present; suspending the OpenVPN/L2TP relay so the core can start (a manual Xray restart from the dashboard re-enables it)")
	}
}
