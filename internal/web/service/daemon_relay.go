package service

import (
	"bytes"
	"net"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/util/tproxy"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"go.uber.org/atomic"
)

// Daemon relay: routing OpenVPN and L2TP/IPsec traffic through the core.
//
// Those two protocols are served by host daemons, not by Xray, and until now
// their clients' packets were simply forwarded and NATed by the kernel — so
// nothing configured on the Routing page ever applied to them. The relay
// closes that gap without touching the daemons: for every enabled local
// OpenVPN/L2TP inbound the generated config gets a loopback dokodemo-door
// inbound in "tproxy" mode, tagged with the inbound's own tag, and the
// daemons' network managers divert the pool's new TCP connections and UDP
// datagrams into it with iptables TPROXY (internal/util/tproxy). Xray then
// sees the original destination, sniffs the domain exactly like it does for
// its native inbounds, and runs the same router — basic and custom rules,
// inboundTag matches, balancers, the lot.
//
// Safety properties, in order of importance for a production host:
//   - Nothing here can restart a daemon: the relay port is not part of the
//     daemon fingerprints and only affects iptables rules.
//   - Fail-open: when the relay listener is missing (Xray stopped, feature
//     suspended, host without TPROXY support) the network managers fall back
//     to the previous direct path within one reconcile round.
//   - The relay inbound is only generated when this process can open
//     transparent sockets and the loopback port is verifiably free, so the
//     core is never handed an inbound it cannot start; and should it still
//     die right after starting twice in a row, the relay is suspended.
//   - Traffic is accounted once: the daemons keep reporting per-inbound and
//     per-client bytes, and the core's own counters for the relay tag are
//     discarded (see dropDaemonRelayTraffic).

const (
	// daemonRelayBasePort is the first loopback port used for relay
	// listeners: inbound id N gets daemonRelayBasePort+N, bumped past any
	// port another generated inbound already uses. Deterministic ports keep
	// the iptables rules stable across config regenerations.
	daemonRelayBasePort = 63900

	// daemonRelayDokodemoSettings makes the listener accept both transports
	// and use the kernel-preserved original destination.
	daemonRelayDokodemoSettings = `{"network":"tcp,udp","followRedirect":true}`
	// daemonRelayStreamSettings turns the listener into a TPROXY target
	// (IP_TRANSPARENT + IP_RECVORIGDSTADDR). Also the signature used to
	// recognise relay inbounds in the running config.
	daemonRelayStreamSettings = `{"sockopt":{"tproxy":"tproxy"}}`

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
// the injection logic on hosts without CAP_NET_ADMIN.
var daemonRelaySupported = tproxy.Supported

// isDaemonRelayInbound reports whether a generated inbound is one of the
// relay listeners produced by injectDaemonRelays.
func isDaemonRelayInbound(ib xray.InboundConfig) bool {
	return ib.Protocol == "dokodemo-door" &&
		bytes.Equal(ib.Listen, daemonRelayListen) &&
		bytes.Equal(ib.StreamSettings, json_util.RawMessage(daemonRelayStreamSettings)) &&
		ib.Port >= daemonRelayBasePort && ib.Port <= 65535
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
// ports already claimed by other inbounds in the generated config. Ids too
// large for the id-offset scheme fall back to the first free port above the
// base. Returns 0 when no port is available in the valid range.
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
// loopback port before Xray is asked to bind it. A listener that belongs to
// this panel's own running relay (same port, relay signature) is fine — that
// is exactly what a regeneration keeps.
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

// injectDaemonRelays appends one loopback TPROXY dokodemo-door inbound per
// enabled, local OpenVPN/L2TP inbound that has not opted out. Each relay is
// tagged with the inbound's own tag so inboundTag rules keep working, and it
// carries the same sniffing settings as every other generated inbound so
// domain rules can match. Everything lives only in the generated config and
// is hot-appliable, so toggling the feature never restarts the core.
func injectDaemonRelays(cfg *xray.Config, inbounds []*model.Inbound) {
	var wanted []*model.Inbound
	for _, inbound := range inbounds {
		if inbound == nil || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		if inbound.Protocol != model.OpenVPN && inbound.Protocol != model.L2TP {
			continue
		}
		if !daemonRelayRoutesThroughXray(inbound) {
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
			logger.Warning("daemon relay: this process cannot open transparent sockets (needs Linux and CAP_NET_ADMIN); OpenVPN/L2TP traffic stays on the direct path and routing rules will not apply to it")
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
		usedPorts[port] = struct{}{}
		cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
			Listen:         daemonRelayListen,
			Port:           port,
			Protocol:       "dokodemo-door",
			Settings:       json_util.RawMessage(daemonRelayDokodemoSettings),
			StreamSettings: json_util.RawMessage(daemonRelayStreamSettings),
			Sniffing:       json_util.RawMessage(amneziawgEgressSniffingSettings),
			Tag:            inbound.Tag,
		})
	}
}

// DaemonRelayPort returns the loopback port of the relay listener the
// running core currently exposes for the inbound with this tag, or 0 when
// there is none (core stopped or not yet listening, feature off or
// suspended, inbound opted out). The OpenVPN/L2TP reconcile jobs feed this
// into their network managers every round, which is what makes the whole
// path fail-open.
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
		// The config is the intent; the socket is the fact. Never hand out a
		// port whose listener is not really there (core still starting, or
		// the bind failed) — a TPROXY rule aimed at a dead port would drop
		// the clients' traffic instead of relaying it.
		if !tproxy.ListenerBound(ib.Port) {
			return 0
		}
		return ib.Port
	}
	return 0
}

// dropDaemonRelayTraffic removes the core's inbound-level counters for relay
// listeners. The daemons already account that traffic (per inbound and per
// client, including what never enters Xray such as ICMP), so keeping both
// would double-count every byte. Relay inbounds have no users, so there are
// no per-client counters to filter.
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

// noteDaemonRelayCrash is the crash-loop guard. RestartXray calls it (with
// the package-level lock held) before regenerating the config: when the core
// died shortly after starting, twice in a row, while relay inbounds were part
// of its config, the relays are suspended so this feature can never keep a
// host in a crash loop — traffic falls back to the direct path and the reason
// is logged. A forced restart (panel start, the UI button) lifts the
// suspension so an operator can retry after fixing the host.
func noteDaemonRelayCrash(isForce, crashed bool) {
	if isForce {
		daemonRelayStrikes = 0
		if daemonRelaySuspended.Swap(false) {
			logger.Info("daemon relay: suspension lifted by a forced restart; OpenVPN/L2TP relay inbounds are generated again")
		}
		return
	}
	if !crashed || p == nil {
		daemonRelayStrikes = 0
		return
	}
	cfg := p.GetConfig()
	if !configHasDaemonRelay(cfg) || time.Duration(p.GetUptime())*time.Second > daemonRelayCrashWindow {
		daemonRelayStrikes = 0
		return
	}
	daemonRelayStrikes++
	if daemonRelayStrikes < 2 || daemonRelaySuspended.Load() {
		return
	}
	daemonRelaySuspended.Store(true)
	daemonRelayStrikes = 0
	logger.Error("daemon relay: xray died shortly after starting twice in a row while OpenVPN/L2TP relay inbounds were configured; suspending them (their traffic takes the direct path, routing rules will not apply) until the next forced restart. Last xray output: ", p.GetResult())
}
