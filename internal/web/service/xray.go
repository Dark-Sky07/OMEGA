package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/amneziawg"
	"github.com/mhsanaei/3x-ui/v3/internal/amneziawgnet"
	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"go.uber.org/atomic"
)

var (
	p                 *xray.Process
	lock              sync.Mutex
	isNeedXrayRestart atomic.Bool // Indicates that restart was requested for Xray
	isManuallyStopped atomic.Bool // Indicates that Xray was stopped manually from the panel
	result            string
)

// XrayService provides business logic for Xray process management.
// It handles starting, stopping, restarting Xray, and managing its configuration.
type XrayService struct {
	inboundService InboundService
	settingService SettingService
	xrayAPI        xray.XrayAPI
}

// IsXrayRunning checks if the Xray process is currently running.
func (s *XrayService) IsXrayRunning() bool {
	return p != nil && p.IsRunning()
}

// XrayProcess returns the current Xray process instance (may be nil when Xray
// is not running). It exposes the package-level process to callers outside this
// package (e.g. the tgbot subpackage) without changing access semantics.
func XrayProcess() *xray.Process {
	return p
}

// GetXrayErr returns the error from the Xray process, if any.
func (s *XrayService) GetXrayErr() error {
	if p == nil {
		return nil
	}

	err := p.GetErr()
	if err == nil {
		return nil
	}

	if runtime.GOOS == "windows" && err.Error() == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return nil
	}

	return err
}

// GetXrayResult returns the result string from the Xray process.
func (s *XrayService) GetXrayResult() string {
	if result != "" {
		return result
	}
	if s.IsXrayRunning() {
		return ""
	}
	if p == nil {
		return ""
	}

	result = p.GetResult()

	if runtime.GOOS == "windows" && result == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return ""
	}

	return result
}

// GetXrayVersion returns the version of the running Xray process.
func (s *XrayService) GetXrayVersion() string {
	if p == nil {
		return "Unknown"
	}
	return p.GetVersion()
}

// RemoveIndex removes an element at the specified index from a slice.
// Returns a new slice with the element removed.
func RemoveIndex(s []any, index int) []any {
	return append(s[:index], s[index+1:]...)
}

// GetXrayConfig retrieves and builds the Xray configuration from settings and inbounds.
func (s *XrayService) GetXrayConfig() (*xray.Config, error) {
	templateConfig, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}

	xrayConfig := &xray.Config{}
	err = json.Unmarshal([]byte(templateConfig), xrayConfig)
	if err != nil {
		return nil, err
	}
	xrayConfig.LogConfig = resolveXrayLogPaths(xrayConfig.LogConfig)
	xrayConfig.API = ensureAPIServices(xrayConfig.API)
	xrayConfig.Policy = ensureStatsPolicy(xrayConfig.Policy)

	_, _, _ = s.inboundService.AddTraffic(nil, nil)

	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		if inbound.NodeID != nil {
			continue
		}
		if inbound.Protocol == model.MTProto || inbound.Protocol == model.AmneziaWG {
			continue
		}
		// OpenVPN and L2TP/IPsec inbounds are served by host daemons that bind
		// their own sockets; they must never appear in the Xray config.
		if inbound.Protocol == model.OpenVPN || inbound.Protocol == model.L2TP {
			// These are host daemons, not Xray protocols. Their listeners and
			// credentials are reconciled by their respective jobs.
			continue
		}
		settings := map[string]any{}
		json.Unmarshal([]byte(inbound.Settings), &settings)

		dbClients, listErr := s.inboundService.clientService.ListForInbound(nil, inbound.Id)
		if listErr != nil {
			return nil, listErr
		}

		clientStats := inbound.ClientStats
		enableMap := make(map[string]bool, len(clientStats))
		for _, clientTraffic := range clientStats {
			enableMap[transferEmailKey(clientTraffic.Email)] = clientTraffic.Enable
		}

		var finalClients []any
		for i := range dbClients {
			c := dbClients[i]
			if enable, exists := enableMap[transferEmailKey(c.Email)]; exists && !enable {
				logger.Infof("Remove Inbound User %s due to expiration or traffic limit", c.Email)
				continue
			}
			if !c.Enable {
				continue
			}
			flow := c.Flow
			if flow == "xtls-rprx-vision-udp443" {
				flow = "xtls-rprx-vision"
			}
			entry := map[string]any{"email": c.Email}
			switch inbound.Protocol {
			case model.VLESS:
				if c.ID != "" {
					entry["id"] = c.ID
				}
				if flow != "" {
					entry["flow"] = flow
				}
				if c.Reverse != nil {
					entry["reverse"] = c.Reverse
				}
			case model.VMESS:
				if c.ID != "" {
					entry["id"] = c.ID
				}
				if c.Security != "" {
					entry["security"] = c.Security
				}
			case model.Trojan:
				if c.Password != "" {
					entry["password"] = c.Password
				}
				if flow != "" {
					entry["flow"] = flow
				}
			case model.Shadowsocks:
				if c.Password != "" {
					entry["password"] = c.Password
				}
			case model.Hysteria:
				if c.Auth != "" {
					entry["auth"] = c.Auth
				}
			}
			finalClients = append(finalClients, entry)
		}

		_, hadClients := settings["clients"]
		mutated := hadClients || len(finalClients) > 0
		if mutated {
			settings["clients"] = finalClients
		}

		if inboundCanHostFallbacks(inbound) {
			fallbacks, fbErr := s.inboundService.fallbackService.BuildFallbacksJSON(nil, inbound.Id)
			if fbErr != nil {
				return nil, fbErr
			}
			if len(fallbacks) > 0 {
				generic := make([]any, 0, len(fallbacks))
				for _, f := range fallbacks {
					generic = append(generic, f)
				}
				settings["fallbacks"] = generic
				mutated = true
			}
		}

		if mutated {
			modifiedSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.Settings = string(modifiedSettings)
		}

		if len(inbound.StreamSettings) > 0 {
			// Unmarshal stream JSON
			var stream map[string]any
			json.Unmarshal([]byte(inbound.StreamSettings), &stream)

			// Remove the "settings" field under "tlsSettings" and "realitySettings"
			tlsSettings, ok1 := stream["tlsSettings"].(map[string]any)
			realitySettings, ok2 := stream["realitySettings"].(map[string]any)
			if ok1 || ok2 {
				if ok1 {
					delete(tlsSettings, "settings")
				} else if ok2 {
					delete(realitySettings, "settings")
				}
			}

			delete(stream, "externalProxy")

			newStream, err := json.MarshalIndent(stream, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.StreamSettings = string(newStream)
		}

		if inbound.Protocol == model.Shadowsocks {
			if healed, ok := model.HealShadowsocksClientMethods(inbound.Settings); ok {
				inbound.Settings = healed
			}
		}

		inboundConfig := inbound.GenXrayInboundConfig()
		xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *inboundConfig)
	}

	// Merge subscription-derived outbounds (if any) into the final outbounds array.
	// These are additive: each subscription is placed before or after the template
	// outbounds based on its Prepend flag, ordered by Priority. Tags assigned by the
	// subscription service are kept stable across refreshes so that balancers and
	// routing rules continue to work.
	subSvc := &OutboundSubscriptionService{}
	if prepend, appendList, err := subSvc.activeOutboundsSplit(); err == nil && (len(prepend) > 0 || len(appendList) > 0) {
		mergeSubscriptionOutbounds(xrayConfig, prepend, appendList)
	}

	// Route opted-in local mtproto inbounds through the core's router. Each one
	// gets a loopback SOCKS bridge — tagged with the inbound's own tag so it is
	// matchable in routing rules — that its mtg sidecar dials Telegram through.
	// Done after the subscription merge so a selected subscription outbound (or
	// balancer) is a valid rule target.
	for i := range inbounds {
		inbound := inbounds[i]
		if inbound.Protocol != model.MTProto || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		injectMtprotoEgress(xrayConfig, inbound)
	}

	// Every AmneziaWG inbound is embedded (internal/amneziawgnet: amneziawg-go
	// over a gVisor netstack, no kernel module) and relays every peer's
	// decapsulated traffic into its own loopback SOCKS5 inbound, always on —
	// unlike mtproto's bridge above, there's no opt-in gate here: once
	// traffic is decapsulated in gVisor, Xray's own freedom outbound is the
	// only way it reaches the real internet at all, not an optional extra
	// hop. Whether it goes anywhere beyond Xray's default routing is up to
	// whatever rules the admin adds through the stock Routing page, exactly
	// like routing any other protocol.
	injectAmneziawgnetSocks(xrayConfig, inbounds)

	// Restores each opted-in peer's own distinct public IPv6 source identity
	// for its outbound connections — a peer that has an IPv6 address in its
	// AllowedIPs, on an inbound with IPv6Enabled, gets its own freedom
	// outbound bound to that exact address via sendThrough.
	// internal/amneziawgnet's own Manager is responsible for actually
	// aliasing that address onto the host (see v6alias.go) so the kernel
	// lets Xray bind an egress socket to it at all; this call only builds
	// the Xray-side outbound/routing-rule half.
	injectAmneziawgV6Egress(xrayConfig, inbounds)

	// Wire the panel's own HTTP traffic through the configured outbound, after
	// the subscription merge so subscription outbound tags are valid targets.
	if egressTag, err := s.settingService.GetPanelOutbound(); err != nil {
		logger.Warning("read panelOutbound setting failed:", err)
	} else if egressTag != "" {
		injectPanelEgress(xrayConfig, egressTag)
	}

	return xrayConfig, nil
}

// PanelEgressInboundTag is the tag of the loopback SOCKS inbound injected into
// the generated config when a panel outbound is configured. The panel's own
// HTTP clients dial through it to egress via the chosen outbound.
const PanelEgressInboundTag = "panel-egress"

// panelEgressBasePort is the first port tried for the egress bridge; ports
// already taken by other inbounds in the generated config are skipped.
const panelEgressBasePort = 62790

// injectPanelEgress appends a loopback SOCKS inbound to the generated config
// and prepends a routing rule sending it to outboundTag. Both live only in the
// generated config — the stored template is never modified — and both are
// hot-appliable, so changing the panel outbound never restarts the core.
func injectPanelEgress(cfg *xray.Config, outboundTag string) {
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Tag == PanelEgressInboundTag {
			logger.Warning("panel egress: inbound tag [", PanelEgressInboundTag, "] already exists, skipping injection")
			return
		}
	}

	// The rule must exist before the inbound takes traffic, otherwise the
	// bridge would silently egress through the default outbound instead.
	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			logger.Warning("panel egress: routing section is unparsable, skipping injection:", err)
			return
		}
	}
	rules, _ := routing["rules"].([]any)
	rule := map[string]any{
		"type":       "field",
		"inboundTag": []any{PanelEgressInboundTag},
	}
	// The configured tag may name a routing balancer instead of a concrete
	// outbound. A field rule can target either, so emit the matching key —
	// balancerTag load-balances the panel's own traffic across the balancer's
	// outbounds, while a plain outbound tag keeps the original behavior.
	if routingTagIsBalancer(routing, outboundTag) {
		rule["balancerTag"] = outboundTag
	} else {
		rule["outboundTag"] = outboundTag
	}
	routing["rules"] = append([]any{rule}, rules...)
	newRouting, err := json.Marshal(routing)
	if err != nil {
		logger.Warning("panel egress: failed to rebuild routing section, skipping injection:", err)
		return
	}
	cfg.RouterConfig = json_util.RawMessage(newRouting)

	used := make(map[int]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		used[cfg.InboundConfigs[i].Port] = struct{}{}
	}
	port := panelEgressBasePort
	for {
		if _, taken := used[port]; !taken {
			break
		}
		port++
	}

	cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
		Listen:   json_util.RawMessage(`"127.0.0.1"`),
		Port:     port,
		Protocol: "socks",
		Settings: json_util.RawMessage(`{"auth":"noauth","udp":false}`),
		Tag:      PanelEgressInboundTag,
	})
}

// routingTagIsBalancer reports whether tag names a balancer in the parsed
// routing section. The panel-egress rule targets a balancer via balancerTag and
// a concrete outbound via outboundTag, so the caller picks the key from this.
func routingTagIsBalancer(routing map[string]any, tag string) bool {
	if tag == "" {
		return false
	}
	balancers, ok := routing["balancers"].([]any)
	if !ok {
		return false
	}
	for _, b := range balancers {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := bm["tag"].(string); ok && t == tag {
			return true
		}
	}
	return false
}

// mtprotoEgressSocksSettings is the loopback SOCKS server a routed mtproto
// inbound exposes for its mtg sidecar to dial Telegram through. mtg makes plain
// TCP connections, so UDP is left off (matching the panel egress bridge).
const mtprotoEgressSocksSettings = `{"auth":"noauth","udp":false}`

// injectMtprotoEgress wires one routed mtproto inbound into the generated
// config: it appends a loopback SOCKS inbound (tagged with the inbound's own tag,
// on the egress port persisted in settings) and, when an outbound is selected,
// prepends a routing rule sending that tag to it. Both live only in the generated
// config — the stored template is untouched — and both are hot-appliable, so
// toggling routing never forces a full Xray restart. Mirrors injectPanelEgress.
func injectMtprotoEgress(cfg *xray.Config, inbound *model.Inbound) {
	var parsed struct {
		RouteThroughXray bool   `json:"routeThroughXray"`
		RouteXrayPort    int    `json:"routeXrayPort"`
		OutboundTag      string `json:"outboundTag"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &parsed); err != nil {
		return
	}
	if !parsed.RouteThroughXray || parsed.RouteXrayPort <= 0 || inbound.Tag == "" {
		return
	}
	tag := inbound.Tag
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Tag == tag {
			logger.Warning("mtproto egress: inbound tag [", tag, "] already present in generated config, skipping bridge")
			return
		}
	}

	if parsed.OutboundTag != "" {
		routing := map[string]any{}
		parseOK := true
		if len(cfg.RouterConfig) > 0 {
			if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
				logger.Warning("mtproto egress: routing section is unparsable, skipping rule:", err)
				parseOK = false
			}
		}
		if parseOK {
			rules, _ := routing["rules"].([]any)
			rule := map[string]any{
				"type":       "field",
				"inboundTag": []any{tag},
			}
			if routingTagIsBalancer(routing, parsed.OutboundTag) {
				rule["balancerTag"] = parsed.OutboundTag
			} else {
				rule["outboundTag"] = parsed.OutboundTag
			}
			routing["rules"] = append([]any{rule}, rules...)
			if newRouting, err := json.Marshal(routing); err == nil {
				cfg.RouterConfig = json_util.RawMessage(newRouting)
			} else {
				logger.Warning("mtproto egress: failed to rebuild routing section, skipping rule:", err)
			}
		}
	}

	cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
		Listen:   json_util.RawMessage(`"127.0.0.1"`),
		Port:     parsed.RouteXrayPort,
		Protocol: "socks",
		Settings: json_util.RawMessage(mtprotoEgressSocksSettings),
		Tag:      tag,
	})
}

// amneziawgEgressSniffingSettings matches this fork's normal per-inbound
// default (see default.json's "mixed" inbound). Without this, domain-based
// Routing rules can never match this relay: the peer resolved DNS
// itself, through the tunnel, before ever sending a packet — by the time the
// embedded forwarder recovers the decapsulated traffic, the destination is
// already a bare IP, with no domain name attached at the network layer at
// all. Sniffing recovers it from the payload itself (TLS SNI / HTTP Host /
// QUIC) the same way it already does for every other inbound; without it,
// only tag/IP/network-based rules can ever match this traffic, and any
// domain rule above it in the list is silently unreachable.
const amneziawgEgressSniffingSettings = `{"enabled":true,"destOverride":["http","tls","quic","fakedns"]}`

// injectAmneziawgnetSocks gives every enabled AmneziaWG inbound with at
// least one qualifying peer its own loopback SOCKS5 inbound for the
// embedded (amneziawg-go) relay path (internal/amneziawgnet) -- always on,
// since there is no alternative datapath once traffic is decapsulated in
// gVisor: Xray's own freedom outbound is how it reaches the real internet at
// all (see internal/amneziawgnet/relay.go's doc comment, Finding 3 of the
// migration plan). Tagged with the inbound's own real tag: it's already
// selectable in the panel's stock Routing page (InboundService.GetInboundTags
// is protocol-blind), and per-inbound traffic totals
// (internal/web/service/inbound_traffic.go's addClientTraffic) match by
// exact tag -- reusing it isn't a style choice.
func injectAmneziawgnetSocks(cfg *xray.Config, inbounds []*model.Inbound) {
	existingTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		existingTags[cfg.InboundConfigs[i].Tag] = struct{}{}
	}

	for _, inbound := range inbounds {
		if inbound.Protocol != model.AmneziaWG || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		inst, ok := amneziawg.InstanceFromInbound(inbound)
		if !ok {
			continue
		}
		if _, taken := existingTags[inbound.Tag]; taken {
			logger.Warning("amneziawgnet socks: inbound tag [", inbound.Tag, "] already present in generated config, skipping its relay inbound")
			continue
		}

		emails := make([]string, 0, len(inst.Peers))
		for _, p := range inst.Peers {
			if p.Email != "" {
				emails = append(emails, p.Email)
			}
		}
		if len(emails) == 0 {
			continue
		}

		settings, err := amneziawgnet.SocksInboundSettings(emails, amneziawgnet.SocksPassword())
		if err != nil {
			logger.Warning("amneziawgnet socks: building settings for inbound [", inbound.Tag, "]: ", err)
			continue
		}

		existingTags[inbound.Tag] = struct{}{}
		cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
			Listen:   json_util.RawMessage(`"127.0.0.1"`),
			Port:     amneziawgnet.SOCKSPortForInbound(inbound.Id),
			Protocol: "socks",
			Settings: json_util.RawMessage(settings),
			Sniffing: json_util.RawMessage(amneziawgEgressSniffingSettings),
			Tag:      inbound.Tag,
		})
	}
}

// amneziawgV6EgressTag returns the stable, globally-unique freedom outbound
// tag for one peer's IPv6 source-identity egress. Stable across config
// regenerations (a pure function of two stable identifiers), so
// internal/xray/hot_diff.go's tag-keyed outbound/routing diffing recognizes
// "unchanged" rather than remove+recreate on every poll. The inbound.Id
// prefix is defense in depth, not load-bearing on its own: email is already
// enforced globally unique across the whole panel's client table
// (model.ClientRecord.Email has a gorm uniqueIndex) — kept anyway since it
// costs nothing and makes the tag self-describing, matching
// NodeEgressInboundTag's own style.
func amneziawgV6EgressTag(inboundID int, email string) string {
	return fmt.Sprintf("amneziawg-v6-%d-%s", inboundID, email)
}

// injectAmneziawgV6Egress gives every enabled, non-node-hosted AmneziaWG
// peer with an IPv6 AllowedIPs entry its own single-purpose freedom
// outbound, bound via sendThrough to that exact address, plus a routing
// rule sending only that peer's own traffic through it — restoring the
// per-client public IPv6 identity the hard cutover temporarily dropped
// (Phase 3.5 of the migration plan). Scoped to outbound source identity
// only: it depends on internal/amneziawgnet's own alias mechanism actually
// giving the host that address at the OS level (see v6alias.go's
// V6AliasesActive, the exact same gate this function uses below) — without
// that, sendThrough fails to bind and every connection through it errors
// outright (freedom.go's dial failure); there is no fallback outbound.
//
// The routing rule matches both inboundTag and user: SocksInboundSettings
// (used by injectAmneziawgnetSocks above) already authenticates each
// connection as the peer's own email via stock SOCKS5 auth, and a stock
// Xray SOCKS5 inbound sets that connection's stats/routing identity from
// the authenticated username — so "user" reliably isolates exactly one
// peer's traffic, the same building block Finding 3 of the migration plan
// already established for per-client stats.
//
// Modeled on injectNodeEgresses (the established N-per-slice inbound+rule
// precedent, not injectAmneziawgnetSocks itself, which only ever emits a
// single inbound and never touches outbounds/routing) and
// mergeSubscriptionOutbounds's unmarshal-append-remarshal pattern for
// cfg.OutboundConfigs. Synthetic rules are prepended ahead of whatever's
// already in the routing rules array, the same pattern injectNodeEgresses/
// injectMtprotoEgress already use for their own always-must-win infra
// rules — this never touches the admin's own saved Routing-page rule
// order.
func injectAmneziawgV6Egress(cfg *xray.Config, inbounds []*model.Inbound) {
	// Protocol is checked alongside Tag, not just Tag alone: a tag collision
	// with some unrelated (non-socks) inbound must not be mistaken for this
	// instance's own relay having been created.
	liveInboundTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Protocol == "socks" {
			liveInboundTags[cfg.InboundConfigs[i].Tag] = struct{}{}
		}
	}

	var existingOutbounds []any
	if len(cfg.OutboundConfigs) > 0 {
		if err := json.Unmarshal(cfg.OutboundConfigs, &existingOutbounds); err != nil {
			logger.Warning("amneziawg v6 egress: outbounds section is unparsable, skipping injection:", err)
			return
		}
	}
	usedOutboundTags := make(map[string]struct{}, len(existingOutbounds))
	for _, o := range existingOutbounds {
		if m, ok := o.(map[string]any); ok {
			if t, ok := m["tag"].(string); ok {
				usedOutboundTags[t] = struct{}{}
			}
		}
	}

	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			logger.Warning("amneziawg v6 egress: routing section is unparsable, skipping injection:", err)
			return
		}
	}
	rules, _ := routing["rules"].([]any)
	newRules := make([]any, 0)
	newOutbounds := make([]any, 0)

	for _, inbound := range inbounds {
		if inbound.Protocol != model.AmneziaWG || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		if _, live := liveInboundTags[inbound.Tag]; !live {
			// The relay inbound itself wasn't created this pass (e.g. a tag
			// collision inside injectAmneziawgnetSocks) -- no SOCKS5 inbound
			// exists for hot_diff.go's inboundTag match to ever fire against.
			continue
		}
		inst, ok := amneziawg.InstanceFromInbound(inbound)
		if !ok || !amneziawgnet.V6AliasesActive(inst) {
			continue
		}
		for _, p := range inst.Peers {
			if p.Email == "" {
				continue
			}
			v6 := amneziawg.FirstIPv6(p.AllowedIPs)
			if v6 == "" {
				continue
			}
			tag := amneziawgV6EgressTag(inbound.Id, p.Email)
			if _, taken := usedOutboundTags[tag]; taken {
				logger.Warning("amneziawg v6 egress: outbound tag [", tag, "] already exists, skipping peer [", p.Email, "]")
				continue
			}
			usedOutboundTags[tag] = struct{}{}
			newOutbounds = append(newOutbounds, map[string]any{
				"tag":         tag,
				"protocol":    "freedom",
				"sendThrough": v6,
				"settings":    map[string]any{},
			})
			newRules = append(newRules, map[string]any{
				"type":        "field",
				"inboundTag":  []any{inbound.Tag},
				"user":        []any{p.Email},
				"outboundTag": tag,
			})
		}
	}

	if len(newOutbounds) == 0 {
		return
	}

	merged := make([]any, 0, len(existingOutbounds))
	merged = append(merged, existingOutbounds...)
	merged = append(merged, newOutbounds...)
	combined, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		logger.Warning("amneziawg v6 egress: failed to rebuild outbounds section, skipping injection:", err)
		return
	}
	cfg.OutboundConfigs = json_util.RawMessage(combined)

	routing["rules"] = append(newRules, rules...)
	newRouting, err := json.Marshal(routing)
	if err != nil {
		logger.Warning("amneziawg v6 egress: failed to rebuild routing section, skipping injection:", err)
		return
	}
	cfg.RouterConfig = json_util.RawMessage(newRouting)
}

// mergeSubscriptionOutbounds appends the subscription outbounds to the
// OutboundConfigs array of the xray config. It works on the already-unmarshaled
// template so that manually configured outbounds are never overwritten.
//
// Safety: if we cannot parse the template's outbounds array, we leave
// OutboundConfigs exactly as it came from the template (we do not inject
// subscription outbounds). This prevents us from accidentally dropping the
// user's manually configured outbounds when the template is in a weird state.
func mergeSubscriptionOutbounds(cfg *xray.Config, prepend, appendList []any) {
	if len(prepend) == 0 && len(appendList) == 0 {
		return
	}
	var templateOutbounds []any
	if len(cfg.OutboundConfigs) > 0 {
		if err := json.Unmarshal(cfg.OutboundConfigs, &templateOutbounds); err != nil {
			// Corrupt template outbounds — do not touch the field at all.
			// The user will see problems on Xray start / next save.
			return
		}
	}
	merged := make([]any, 0, len(prepend)+len(templateOutbounds)+len(appendList))
	merged = append(merged, prepend...)
	merged = append(merged, templateOutbounds...)
	merged = append(merged, appendList...)
	combined, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return
	}
	cfg.OutboundConfigs = json_util.RawMessage(combined)
}

// ensureAPIServices guarantees the gRPC services the panel depends on are
// listed in the generated config's api block: HandlerService and StatsService
// have always been required for inbound/user management and traffic polling,
// and RoutingService enables hot routing reload on templates saved before it
// was added to the default template. The stored template itself is not
// modified — only the generated runtime config.
func ensureAPIServices(api json_util.RawMessage) json_util.RawMessage {
	if len(api) == 0 {
		// No api block means the panel's API integration is deliberately
		// disabled; don't resurrect it behind the user's back.
		return api
	}
	var parsed map[string]any
	if err := json.Unmarshal(api, &parsed); err != nil {
		return api
	}
	services, _ := parsed["services"].([]any)
	have := make(map[string]bool, len(services))
	for _, svc := range services {
		if name, ok := svc.(string); ok {
			have[name] = true
		}
	}
	added := false
	for _, name := range []string{"HandlerService", "StatsService", "RoutingService"} {
		if !have[name] {
			services = append(services, name)
			added = true
		}
	}
	if !added {
		return api
	}
	parsed["services"] = services
	out, err := json.Marshal(parsed)
	if err != nil {
		return api
	}
	return out
}

// ensureStatsPolicy guarantees every policy level in the generated config has
// statsUserOnline enabled, so the core tracks per-email online IPs for the
// panel's online view and access-log-free IP limiting. Generated clients carry
// no explicit level, so level "0" is created when absent. The flag is panel
// infrastructure and is forced on even over an explicit false in the template,
// same as the api services above. An entirely missing or unparsable policy
// block is left alone; the stored template itself is never modified — only the
// generated runtime config.
func ensureStatsPolicy(policy json_util.RawMessage) json_util.RawMessage {
	if len(policy) == 0 {
		return policy
	}
	var parsed map[string]any
	if err := json.Unmarshal(policy, &parsed); err != nil {
		return policy
	}
	levels, _ := parsed["levels"].(map[string]any)
	if levels == nil {
		levels = make(map[string]any)
	}
	if _, ok := levels["0"]; !ok {
		levels["0"] = map[string]any{}
	}
	changed := false
	for _, raw := range levels {
		level, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if enabled, ok := level["statsUserOnline"].(bool); !ok || !enabled {
			level["statsUserOnline"] = true
			changed = true
		}
	}
	if !changed {
		return policy
	}
	parsed["levels"] = levels
	out, err := json.Marshal(parsed)
	if err != nil {
		return policy
	}
	return out
}

func resolveXrayLogPaths(logCfg json_util.RawMessage) json_util.RawMessage {
	if len(logCfg) == 0 {
		return logCfg
	}
	var parsed map[string]any
	if err := json.Unmarshal(logCfg, &parsed); err != nil {
		return logCfg
	}
	changed := false
	for _, key := range []string{"access", "error"} {
		v, ok := parsed[key].(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || strings.EqualFold(trimmed, "none") {
			continue
		}
		base := path.Base(filepath.ToSlash(trimmed))
		if base == "" || base == "." || base == ".." || base == "/" {
			continue
		}
		confined := filepath.Join(config.GetLogFolder(), base)
		if confined == trimmed {
			continue
		}
		parsed[key] = confined
		changed = true
	}
	if !changed {
		return logCfg
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return logCfg
	}
	return out
}

// GetXrayTraffic fetches the current traffic statistics from the running Xray process.
func (s *XrayService) GetXrayTraffic() ([]*xray.Traffic, []*xray.ClientTraffic, error) {
	if !s.IsXrayRunning() {
		err := errors.New("xray is not running")
		logger.Debug("Attempted to fetch Xray traffic, but Xray is not running:", err)
		return nil, nil, err
	}
	apiPort := p.GetAPIPort()
	if err := s.xrayAPI.Init(apiPort); err != nil {
		logger.Debug("Failed to initialize Xray API:", err)
		return nil, nil, err
	}
	defer s.xrayAPI.Close()

	traffic, clientTraffic, err := s.xrayAPI.GetTraffic()
	if err != nil {
		logger.Debug("Failed to fetch Xray traffic:", err)
		return nil, nil, err
	}
	return traffic, clientTraffic, nil
}

// GetOnlineUsers returns connection-based online users (email + source IPs)
// from the running core's online-stats API. ok=false means the API is not
// available — xray isn't running or the core predates the online-stats RPCs —
// and callers must use the legacy traffic-delta / access-log paths. The
// capability is probed lazily per process: an Unimplemented answer pins this
// core as unsupported until the next restart, while transient errors leave the
// capability undecided so a flaky poll can't lock in legacy mode.
func (s *XrayService) GetOnlineUsers() ([]xray.OnlineUser, bool, error) {
	if !s.IsXrayRunning() {
		return nil, false, nil
	}
	if p.OnlineAPISupport() == xray.OnlineAPIUnsupported {
		return nil, false, nil
	}
	if err := s.xrayAPI.Init(p.GetAPIPort()); err != nil {
		logger.Debug("Failed to initialize Xray API:", err)
		return nil, false, err
	}
	defer s.xrayAPI.Close()

	users, err := s.xrayAPI.GetOnlineUsers()
	if err != nil {
		if xray.IsUnimplementedErr(err) {
			p.SetOnlineAPISupport(xray.OnlineAPIUnsupported)
			logger.Info("xray core does not support the online-stats API; falling back to traffic-delta onlines and access-log IP limit")
			return nil, false, nil
		}
		logger.Debug("Failed to fetch Xray online users:", err)
		return nil, false, err
	}
	if p.OnlineAPISupport() == xray.OnlineAPIUnknown {
		p.SetOnlineAPISupport(xray.OnlineAPISupported)
		logger.Info("xray core supports the online-stats API; using connection-based onlines and access-log-free IP limit")
	}
	return users, true, nil
}

// BalancerStatus is the live view of one balancer for the panel UI. Running
// is false when the balancer isn't present in the running core (e.g. xray is
// stopped or the balancer hasn't been saved/applied yet).
type BalancerStatus struct {
	Tag      string   `json:"tag"`
	Running  bool     `json:"running"`
	Override string   `json:"override"`
	Selected []string `json:"selected"`
}

// GetBalancersStatus queries the running core for the live state of the
// given balancer tags. Per-tag failures are reported as Running=false rather
// than failing the whole call, so the UI can render saved-but-not-applied
// balancers alongside live ones.
func (s *XrayService) GetBalancersStatus(tags []string) ([]BalancerStatus, error) {
	statuses := make([]BalancerStatus, 0, len(tags))
	if !s.IsXrayRunning() {
		for _, tag := range tags {
			statuses = append(statuses, BalancerStatus{Tag: tag})
		}
		return statuses, nil
	}
	if err := s.xrayAPI.Init(p.GetAPIPort()); err != nil {
		return nil, err
	}
	defer s.xrayAPI.Close()

	for _, tag := range tags {
		info, err := s.xrayAPI.GetBalancerInfo(tag)
		if err != nil {
			logger.Debug("get balancer info [", tag, "] failed:", err)
			statuses = append(statuses, BalancerStatus{Tag: tag})
			continue
		}
		statuses = append(statuses, BalancerStatus{
			Tag:      tag,
			Running:  true,
			Override: info.Override,
			Selected: info.Selected,
		})
	}
	return statuses, nil
}

// OverrideBalancer forces a balancer in the running core to use the given
// outbound tag; an empty target clears the override.
func (s *XrayService) OverrideBalancer(tag, target string) error {
	if !s.IsXrayRunning() {
		return errors.New("xray is not running")
	}
	if err := s.xrayAPI.Init(p.GetAPIPort()); err != nil {
		return err
	}
	defer s.xrayAPI.Close()
	return s.xrayAPI.SetBalancerTarget(tag, target)
}

// TestRoute asks the running core which outbound its router picks for the
// described connection.
func (s *XrayService) TestRoute(req xray.RouteTestRequest) (*xray.RouteTestResult, error) {
	if !s.IsXrayRunning() {
		return nil, errors.New("xray is not running")
	}
	if err := s.xrayAPI.Init(p.GetAPIPort()); err != nil {
		return nil, err
	}
	defer s.xrayAPI.Close()
	return s.xrayAPI.TestRoute(req)
}

// RestartXray reconciles the running Xray process with the current desired
// config. When isForce is false it first tries to apply the changes through
// the Xray gRPC API without restarting the process (inbounds, outbounds and
// routing rules/balancers are hot-reloadable); only changes the core cannot
// take at runtime — or a force request — stop and restart the process.
func (s *XrayService) RestartXray(isForce bool) error {
	lock.Lock()
	defer lock.Unlock()
	logger.Debug("restart Xray, force:", isForce)
	isManuallyStopped.Store(false)

	xrayConfig, err := s.GetXrayConfig()
	if err != nil {
		return err
	}

	if s.IsXrayRunning() {
		configUnchanged := p.GetConfig().Equals(xrayConfig)
		if !isForce && configUnchanged && !isNeedXrayRestart.Load() {
			logger.Debug("It does not need to restart Xray")
			return nil
		}
		if !isForce && !configUnchanged && s.tryHotApply(xrayConfig) {
			logger.Info("Xray config changes applied through the core API, no restart needed")
			return nil
		}
		p.Stop()
	}

	p = xray.NewProcess(xrayConfig)
	result = ""
	s.xrayAPI.StatsLastValues = nil
	err = p.Start()
	if err != nil {
		return err
	}

	return nil
}

// tryHotApply attempts to reconcile the running Xray instance with newCfg
// through the core gRPC API (HandlerService for inbounds/outbounds,
// RoutingService for rules/balancers). It returns true when the running
// instance now matches newCfg; on any failure it returns false and the
// caller falls back to a full process restart, which cleans up whatever was
// partially applied. Callers must hold the package-level lock.
func (s *XrayService) tryHotApply(newCfg *xray.Config) bool {
	oldCfg := p.GetConfig()
	diff, ok := xray.ComputeHotDiff(oldCfg, newCfg)
	if !ok {
		logger.Debug("hot apply: config change is not API-applicable, falling back to restart")
		return false
	}
	if diff.Empty() {
		p.SetConfig(newCfg)
		return true
	}

	apiPort := p.GetAPIPort()
	if apiPort <= 0 {
		return false
	}
	// A dedicated client: s.xrayAPI may be in use by traffic polling on other
	// service instances and is reset around restarts.
	hotAPI := xray.XrayAPI{}
	if err := hotAPI.Init(apiPort); err != nil {
		logger.Debug("hot apply: failed to init xray api:", err)
		return false
	}
	defer hotAPI.Close()

	// Removals first so changed handlers and port swaps never collide with
	// the additions that follow.
	for _, tag := range diff.RemovedInboundTags {
		if err := hotAPI.DelInbound(tag); err != nil && !xray.IsMissingHandlerErr(err) {
			logger.Info("hot apply: remove inbound [", tag, "] failed:", err)
			return false
		}
	}
	for _, tag := range diff.RemovedOutboundTags {
		if err := hotAPI.DelOutbound(tag); err != nil && !xray.IsMissingHandlerErr(err) {
			logger.Info("hot apply: remove outbound [", tag, "] failed:", err)
			return false
		}
	}
	for _, ob := range diff.AddedOutbounds {
		if err := addOutboundReconciling(&hotAPI, ob); err != nil {
			logger.Info("hot apply: add outbound failed:", err)
			return false
		}
	}
	for _, ib := range diff.AddedInbounds {
		if err := addInboundReconciling(&hotAPI, ib); err != nil {
			logger.Info("hot apply: add inbound failed:", err)
			return false
		}
	}
	if diff.RoutingConfig != nil {
		if err := hotAPI.ApplyRoutingConfig(diff.RoutingConfig); err != nil {
			logger.Info("hot apply: apply routing config failed:", err)
			return false
		}
	}

	p.SetConfig(newCfg)
	return true
}

// addInboundReconciling adds an inbound, and on a tag conflict (the handler
// was already created through the runtime API while the stored snapshot was
// stale) replaces the existing handler instead.
func addInboundReconciling(api *xray.XrayAPI, inbound []byte) error {
	err := api.AddInbound(inbound)
	if err == nil || !xray.IsExistingTagErr(err) {
		return err
	}
	var meta struct {
		Tag string `json:"tag"`
	}
	if jsonErr := json.Unmarshal(inbound, &meta); jsonErr != nil || meta.Tag == "" {
		return err
	}
	if delErr := api.DelInbound(meta.Tag); delErr != nil && !xray.IsMissingHandlerErr(delErr) {
		return delErr
	}
	return api.AddInbound(inbound)
}

// addOutboundReconciling mirrors addInboundReconciling for outbounds.
func addOutboundReconciling(api *xray.XrayAPI, outbound []byte) error {
	err := api.AddOutbound(outbound)
	if err == nil || !xray.IsExistingTagErr(err) {
		return err
	}
	var meta struct {
		Tag string `json:"tag"`
	}
	if jsonErr := json.Unmarshal(outbound, &meta); jsonErr != nil || meta.Tag == "" {
		return err
	}
	if delErr := api.DelOutbound(meta.Tag); delErr != nil && !xray.IsMissingHandlerErr(delErr) {
		return delErr
	}
	return api.AddOutbound(outbound)
}

// StopXray stops the running Xray process.
func (s *XrayService) StopXray() error {
	lock.Lock()
	defer lock.Unlock()
	isManuallyStopped.Store(true)
	logger.Debug("Attempting to stop Xray...")
	if s.IsXrayRunning() {
		return p.Stop()
	}
	return errors.New("xray is not running")
}

// SetToNeedRestart marks that Xray needs to be restarted.
func (s *XrayService) SetToNeedRestart() {
	isNeedXrayRestart.Store(true)
}

// GetXrayAPIPort returns the port the local xray process is listening on
// for its gRPC HandlerService, or 0 when xray isn't currently running.
// Exposed for the runtime package's LocalRuntime adapter — runtime can't
// reach into the package-level `p` directly without a service-package
// import cycle.
func (s *XrayService) GetXrayAPIPort() int {
	if p == nil || !p.IsRunning() {
		return 0
	}
	return p.GetAPIPort()
}

// IsNeedRestartAndSetFalse checks if restart is needed and resets the flag to false.
func (s *XrayService) IsNeedRestartAndSetFalse() bool {
	return isNeedXrayRestart.CompareAndSwap(true, false)
}

// DidXrayCrash checks if Xray crashed by verifying it's not running and wasn't manually stopped.
func (s *XrayService) DidXrayCrash() bool {
	return !s.IsXrayRunning() && !isManuallyStopped.Load()
}
