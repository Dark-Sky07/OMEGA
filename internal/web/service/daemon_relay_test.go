package service

import (
	"net"
	"strconv"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/util/tproxy"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func relayTestConfig() *xray.Config {
	return &xray.Config{
		InboundConfigs: []xray.InboundConfig{{
			Listen:   json_util.RawMessage(`"0.0.0.0"`),
			Port:     443,
			Protocol: "vless",
			Tag:      "vless-443",
		}},
		RouterConfig: json_util.RawMessage(`{"domainStrategy":"AsIs","rules":[]}`),
	}
}

func forceRelaySupported(t *testing.T, supported bool) {
	t.Helper()
	prev := daemonRelaySupported
	daemonRelaySupported = func() bool { return supported }
	t.Cleanup(func() { daemonRelaySupported = prev })
}

func TestDaemonRelayRoutesThroughXray(t *testing.T) {
	node := 3
	cases := []struct {
		name string
		ib   *model.Inbound
		want bool
	}{
		{"nil", nil, false},
		{"openvpn default on", &model.Inbound{Protocol: model.OpenVPN, Settings: `{"proto":"udp"}`}, true},
		{"openvpn empty settings", &model.Inbound{Protocol: model.OpenVPN}, true},
		{"openvpn explicit true", &model.Inbound{Protocol: model.OpenVPN, Settings: `{"routeThroughXray":true}`}, true},
		{"openvpn explicit false", &model.Inbound{Protocol: model.OpenVPN, Settings: `{"routeThroughXray":false}`}, false},
		{"l2tp default on", &model.Inbound{Protocol: model.L2TP, Settings: `{"psk":"x"}`}, true},
		{"l2tp explicit false", &model.Inbound{Protocol: model.L2TP, Settings: `{"psk":"x","routeThroughXray":false}`}, false},
		{"remote node never", &model.Inbound{Protocol: model.OpenVPN, NodeID: &node}, false},
		{"other protocol never", &model.Inbound{Protocol: model.VLESS, Settings: `{"routeThroughXray":true}`}, false},
		{"mtproto keeps its own opt-in", &model.Inbound{Protocol: model.MTProto, Settings: `{"routeThroughXray":true}`}, false},
	}
	for _, tc := range cases {
		if got := daemonRelayRoutesThroughXray(tc.ib); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestInjectDaemonRelays(t *testing.T) {
	forceRelaySupported(t, true)
	daemonRelaySuspended.Store(false)
	cfg := relayTestConfig()
	node := 9
	inbounds := []*model.Inbound{
		{Id: 7, Tag: "ovpn-7", Protocol: model.OpenVPN, Enable: true, Settings: `{"proto":"udp"}`},
		{Id: 8, Tag: "l2tp-8", Protocol: model.L2TP, Enable: true, Settings: `{"psk":"secret"}`},
		{Id: 9, Tag: "ovpn-off", Protocol: model.OpenVPN, Enable: true, Settings: `{"routeThroughXray":false}`},
		{Id: 10, Tag: "ovpn-disabled", Protocol: model.OpenVPN, Enable: false},
		{Id: 11, Tag: "ovpn-remote", Protocol: model.OpenVPN, Enable: true, NodeID: &node},
		{Id: 12, Tag: "vless-443", Protocol: model.VLESS, Enable: true},
	}
	injectDaemonRelays(cfg, inbounds)

	if len(cfg.InboundConfigs) != 3 {
		t.Fatalf("expected exactly two relay inbounds appended, got %d: %+v", len(cfg.InboundConfigs)-1, cfg.InboundConfigs)
	}
	byTag := map[string]xray.InboundConfig{}
	for _, ib := range cfg.InboundConfigs[1:] {
		byTag[ib.Tag] = ib
	}
	for _, want := range []struct {
		tag  string
		port int
	}{{"ovpn-7", daemonRelayBasePort + 7}, {"l2tp-8", daemonRelayBasePort + 8}} {
		ib, ok := byTag[want.tag]
		if !ok {
			t.Fatalf("relay for %s missing", want.tag)
		}
		if ib.Port != want.port {
			t.Fatalf("%s: port %d, want %d", want.tag, ib.Port, want.port)
		}
		if ib.Protocol != "dokodemo-door" || string(ib.Listen) != `"127.0.0.1"` {
			t.Fatalf("%s: unexpected relay shape %+v", want.tag, ib)
		}
		if string(ib.Settings) != daemonRelayDokodemoSettings || string(ib.StreamSettings) != daemonRelayStreamSettings {
			t.Fatalf("%s: relay must be a tproxy dokodemo listener, got %s / %s", want.tag, ib.Settings, ib.StreamSettings)
		}
		if string(ib.Sniffing) != amneziawgEgressSniffingSettings {
			t.Fatalf("%s: relay must sniff like the other generated inbounds, got %s", want.tag, ib.Sniffing)
		}
		if !isDaemonRelayInbound(ib) {
			t.Fatalf("%s: generated relay must be recognised by isDaemonRelayInbound", want.tag)
		}
	}
	for _, tag := range []string{"ovpn-off", "ovpn-disabled", "ovpn-remote"} {
		if _, present := byTag[tag]; present {
			t.Fatalf("%s must not get a relay", tag)
		}
	}
	// Regenerating from the same inputs yields byte-identical inbounds, which
	// is what keeps hot diffs and the iptables rules stable.
	again := relayTestConfig()
	injectDaemonRelays(again, inbounds)
	if len(again.InboundConfigs) != len(cfg.InboundConfigs) {
		t.Fatal("regeneration changed the inbound count")
	}
	for i := range cfg.InboundConfigs {
		if !cfg.InboundConfigs[i].Equals(&again.InboundConfigs[i]) {
			t.Fatalf("regeneration produced a different inbound at %d", i)
		}
	}
}

func TestInjectDaemonRelays_SkipsWhenUnsupportedOrSuspended(t *testing.T) {
	inbounds := []*model.Inbound{{Id: 7, Tag: "ovpn-7", Protocol: model.OpenVPN, Enable: true}}

	forceRelaySupported(t, false)
	cfg := relayTestConfig()
	injectDaemonRelays(cfg, inbounds)
	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("no relay expected without transparent-socket support, got %+v", cfg.InboundConfigs)
	}

	forceRelaySupported(t, true)
	daemonRelaySuspended.Store(true)
	t.Cleanup(func() { daemonRelaySuspended.Store(false) })
	cfg = relayTestConfig()
	injectDaemonRelays(cfg, inbounds)
	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("no relay expected while suspended, got %+v", cfg.InboundConfigs)
	}
}

func TestInjectDaemonRelays_TagAndPortCollisions(t *testing.T) {
	forceRelaySupported(t, true)
	daemonRelaySuspended.Store(false)

	// Tag already used by a native inbound: skipped rather than duplicated.
	cfg := relayTestConfig()
	injectDaemonRelays(cfg, []*model.Inbound{{Id: 7, Tag: "vless-443", Protocol: model.OpenVPN, Enable: true}})
	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("tag collision must skip the relay, got %+v", cfg.InboundConfigs)
	}

	// Port already used by another generated inbound: bumped to the next one.
	cfg = relayTestConfig()
	cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{Port: daemonRelayBasePort + 7, Protocol: "socks", Tag: "other"})
	injectDaemonRelays(cfg, []*model.Inbound{{Id: 7, Tag: "ovpn-7", Protocol: model.OpenVPN, Enable: true}})
	if got := cfg.InboundConfigs[len(cfg.InboundConfigs)-1]; got.Tag != "ovpn-7" || got.Port != daemonRelayBasePort+8 {
		t.Fatalf("port collision must pick the next free port, got %+v", got)
	}

	// Port held by another process on the host: skipped so xray can start.
	ln, err := net.Listen("tcp", net.JoinHostPort(tproxy.ListenIP, strconv.Itoa(daemonRelayBasePort+21)))
	if err != nil {
		t.Skipf("cannot bind probe port: %v", err)
	}
	defer ln.Close()
	cfg = relayTestConfig()
	injectDaemonRelays(cfg, []*model.Inbound{{Id: 21, Tag: "ovpn-21", Protocol: model.OpenVPN, Enable: true}})
	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("a busy loopback port must skip the relay, got %+v", cfg.InboundConfigs)
	}
}

func TestDaemonRelayPortFor(t *testing.T) {
	if got := daemonRelayPortFor(0, nil); got != 0 {
		t.Fatalf("id 0 must not get a port, got %d", got)
	}
	if got := daemonRelayPortFor(5, map[int]struct{}{}); got != daemonRelayBasePort+5 {
		t.Fatalf("got %d", got)
	}
	used := map[int]struct{}{daemonRelayBasePort + 5: {}, daemonRelayBasePort + 6: {}}
	if got := daemonRelayPortFor(5, used); got != daemonRelayBasePort+7 {
		t.Fatalf("collisions must be skipped, got %d", got)
	}
	if got := daemonRelayPortFor(65535-daemonRelayBasePort+1, nil); got != daemonRelayBasePort {
		t.Fatalf("ids past the port range must fall back to the first free port above the base, got %d", got)
	}
	full := map[int]struct{}{}
	for port := daemonRelayBasePort; port <= 65535; port++ {
		full[port] = struct{}{}
	}
	if got := daemonRelayPortFor(3, full); got != 0 {
		t.Fatalf("an exhausted range must yield 0, got %d", got)
	}
}

func TestIsDaemonRelayInbound(t *testing.T) {
	relay := xray.InboundConfig{
		Listen:         daemonRelayListen,
		Port:           daemonRelayBasePort + 1,
		Protocol:       "dokodemo-door",
		Settings:       json_util.RawMessage(daemonRelayDokodemoSettings),
		StreamSettings: json_util.RawMessage(daemonRelayStreamSettings),
		Tag:            "ovpn-1",
	}
	if !isDaemonRelayInbound(relay) {
		t.Fatal("relay inbound not recognised")
	}
	userTproxy := relay
	userTproxy.Listen = json_util.RawMessage(`"0.0.0.0"`)
	if isDaemonRelayInbound(userTproxy) {
		t.Fatal("an admin's own tproxy inbound on a public address must not be mistaken for a relay")
	}
	lowPort := relay
	lowPort.Port = 12345
	if isDaemonRelayInbound(lowPort) {
		t.Fatal("ports outside the relay range must not match")
	}
	socks := relay
	socks.Protocol = "socks"
	if isDaemonRelayInbound(socks) {
		t.Fatal("only dokodemo-door listeners are relays")
	}
	cfg := &xray.Config{InboundConfigs: []xray.InboundConfig{socks}}
	if configHasDaemonRelay(cfg) || configHasDaemonRelay(nil) {
		t.Fatal("configHasDaemonRelay false positive")
	}
	cfg.InboundConfigs = append(cfg.InboundConfigs, relay)
	if !configHasDaemonRelay(cfg) {
		t.Fatal("configHasDaemonRelay must see the relay")
	}
}

func TestDropDaemonRelayTraffic(t *testing.T) {
	relay := xray.InboundConfig{
		Listen:         daemonRelayListen,
		Port:           daemonRelayBasePort + 7,
		Protocol:       "dokodemo-door",
		Settings:       json_util.RawMessage(daemonRelayDokodemoSettings),
		StreamSettings: json_util.RawMessage(daemonRelayStreamSettings),
		Tag:            "ovpn-7",
	}
	cfg := &xray.Config{InboundConfigs: []xray.InboundConfig{
		{Port: 443, Protocol: "vless", Tag: "vless-443"},
		relay,
	}}
	in := []*xray.Traffic{
		{IsInbound: true, Tag: "vless-443", Up: 1, Down: 2},
		{IsInbound: true, Tag: "ovpn-7", Up: 10, Down: 20},
		{IsInbound: false, IsOutbound: true, Tag: "direct", Up: 100, Down: 200},
		nil,
	}
	out := dropDaemonRelayTraffic(cfg, in)
	if len(out) != 3 {
		t.Fatalf("expected the relay counter to be dropped, got %d entries", len(out))
	}
	for _, tr := range out {
		if tr != nil && tr.IsInbound && tr.Tag == "ovpn-7" {
			t.Fatal("relay inbound traffic must be discarded")
		}
	}
	// Outbound counters keep the relay's bytes (they really did egress there).
	if out[1] == nil || !out[1].IsOutbound || out[1].Up != 100 {
		t.Fatalf("outbound counters must be untouched, got %+v", out[1])
	}
	// Without relays the slice passes through untouched.
	plain := &xray.Config{InboundConfigs: []xray.InboundConfig{{Port: 443, Protocol: "vless", Tag: "vless-443"}}}
	if got := dropDaemonRelayTraffic(plain, in[:2]); len(got) != 2 {
		t.Fatalf("no filtering expected without relays, got %d", len(got))
	}
	if got := dropDaemonRelayTraffic(nil, in[:2]); len(got) != 2 {
		t.Fatalf("nil config must pass traffic through, got %d", len(got))
	}
}

func TestNoteDaemonRelayCrashGuard(t *testing.T) {
	prev := p
	t.Cleanup(func() {
		p = prev
		daemonRelayStrikes = 0
		daemonRelaySuspended.Store(false)
	})
	relayCfg := &xray.Config{InboundConfigs: []xray.InboundConfig{{
		Listen:         daemonRelayListen,
		Port:           daemonRelayBasePort + 7,
		Protocol:       "dokodemo-door",
		Settings:       json_util.RawMessage(daemonRelayDokodemoSettings),
		StreamSettings: json_util.RawMessage(daemonRelayStreamSettings),
		Tag:            "ovpn-7",
	}}}
	// A never-started process reports IsRunning false and uptime ~0: the
	// same shape as a core that died right after starting.
	p = xray.NewProcess(relayCfg)
	daemonRelayStrikes = 0
	daemonRelaySuspended.Store(false)

	noteDaemonRelayCrash(false, true)
	if daemonRelaySuspended.Load() {
		t.Fatal("a single crash must not suspend the relay")
	}
	noteDaemonRelayCrash(false, true)
	if !daemonRelaySuspended.Load() {
		t.Fatal("two consecutive startup crashes with relays present must suspend them")
	}
	noteDaemonRelayCrash(true, false)
	if daemonRelaySuspended.Load() {
		t.Fatal("a forced restart must lift the suspension")
	}

	// A crash while running normally (no relay in the config) never counts.
	p = xray.NewProcess(&xray.Config{})
	noteDaemonRelayCrash(false, true)
	noteDaemonRelayCrash(false, true)
	if daemonRelaySuspended.Load() {
		t.Fatal("crashes without relay inbounds must not suspend anything")
	}

	// Strikes reset when a restart happens for any reason other than a crash.
	p = xray.NewProcess(relayCfg)
	noteDaemonRelayCrash(false, true)
	noteDaemonRelayCrash(false, false)
	noteDaemonRelayCrash(false, true)
	if daemonRelaySuspended.Load() {
		t.Fatal("a non-crash restart in between must reset the strike counter")
	}
}
