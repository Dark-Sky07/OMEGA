package l2tp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/util/tproxy"
)

type fakeFirewall struct {
	mu        sync.Mutex
	present   map[string]bool
	calls     []string
	failMatch string
	firewalld bool // systemctl is-active firewalld reports "active"
}

func newFakeFirewall() *fakeFirewall {
	return &fakeFirewall{present: map[string]bool{}}
}

func (f *fakeFirewall) run(name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	switch name {
	case "sysctl", "sh":
		return nil, nil
	case "systemctl":
		if f.firewalld {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), errors.New("exit status 3")
	}
	op, rest := "", ""
	for i, a := range args {
		if a == "-C" || a == "-I" || a == "-D" {
			op = a
			rest = strings.Join(append(append([]string{}, args[:i]...), args[i+1:]...), " ")
			break
		}
	}
	switch op {
	case "-C":
		if f.present[rest] {
			return nil, nil
		}
		return []byte("iptables: Bad rule (does a matching rule exist in that chain?)."), errors.New("exit status 1")
	case "-I":
		if f.failMatch != "" && strings.Contains(rest, f.failMatch) {
			return []byte("iptables: No chain/target/match by that name."), errors.New("exit status 1")
		}
		f.present[rest] = true
		return nil, nil
	case "-D":
		if f.present[rest] {
			delete(f.present, rest)
			return nil, nil
		}
		return []byte("iptables: Bad rule (does a matching rule exist in that chain?)."), errors.New("exit status 1")
	}
	return nil, nil
}

func (f *fakeFirewall) divertRules() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for r := range f.present {
		if strings.HasPrefix(r, "-t nat PREROUTING ") {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeFirewall) has(substr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for r := range f.present {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

func newTestNetworkManager(f *fakeFirewall) *NetworkManager {
	return &NetworkManager{
		rules:       make(map[int]networkState),
		sysctlPath:  "/proc/sys/net/ipv4/ip_forward",
		runner:      f.run,
		lookPath:    func(string) (string, error) { return "/sbin/iptables", nil },
		relayWarned: make(map[int]string),
	}
}

func useTempL2TPRoot(t *testing.T) {
	t.Helper()
	l2tpRootOverride = filepath.Join(t.TempDir(), "l2tp")
	t.Cleanup(func() { l2tpRootOverride = "" })
}

func TestNetworkStateRoundTrip(t *testing.T) {
	useTempL2TPRoot(t)
	if err := os.MkdirAll(dataDirForID(3), 0o750); err != nil {
		t.Fatal(err)
	}
	want := networkState{PoolCIDR: DefaultPoolCIDR, InterfaceName: "eth0", FixedInputPorts: []int{500, 4500, 1701}, XrayRelayPort: 63903}
	if err := saveNetworkState(3, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(networkStatePath(3))
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	if _, ok := generic["poolCIDR"]; !ok {
		t.Fatalf("state must be serialised with exported fields, got %s", raw)
	}
	got, ok := loadNetworkState(3)
	if !ok {
		t.Fatal("persisted state must load back")
	}
	if got.PoolCIDR != want.PoolCIDR || got.InterfaceName != want.InterfaceName || got.XrayRelayPort != want.XrayRelayPort || len(got.FixedInputPorts) != 3 {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, want)
	}
}

func TestNetworkManagerRelayLifecycle(t *testing.T) {
	useTempL2TPRoot(t)
	inst := validInstance()
	if err := os.MkdirAll(dataDirForID(inst.Id), 0o750); err != nil {
		t.Fatal(err)
	}
	f := newFakeFirewall()
	m := newTestNetworkManager(f)
	inst.XrayRelayPort = 63907

	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply with relay: %v", err)
	}
	if got := len(f.divertRules()); got != 2 {
		t.Fatalf("expected two nat PREROUTING divert rules, got %d: %v", got, f.divertRules())
	}
	if !f.has("-t nat PREROUTING -i ppp+ -s 10.252.0.0/24 ! -d 10.252.0.0/24 -p tcp -j DNAT --to-destination 127.0.0.1:63907") {
		t.Fatalf("TCP DNAT rule missing: %v", f.divertRules())
	}
	if !f.has("-t nat PREROUTING -i ppp+ -s 10.252.0.0/24 -p udp --dport 53 -j DNAT --to-destination 127.0.0.1:63907") {
		t.Fatalf("UDP DNS DNAT rule missing: %v", f.divertRules())
	}
	if !f.has("-t filter INPUT -i ppp+ -s 10.252.0.0/24 -d 127.0.0.1 -j ACCEPT") {
		t.Fatal("INPUT exception for diverted packets missing")
	}
	if !f.has("-t filter FORWARD -i ppp+ -s 10.252.0.0/24 -p udp --dport 443 -j DROP") {
		t.Fatal("QUIC UDP/443 drop rule missing")
	}
	st, ok := loadNetworkState(inst.Id)
	if !ok || st.XrayRelayPort != 63907 || st.PoolCIDR != DefaultPoolCIDR {
		t.Fatalf("persisted state = %+v (ok=%v)", st, ok)
	}
	if !f.has("POSTROUTING -s 10.252.0.0/24 -j MASQUERADE") {
		t.Fatal("direct-path MASQUERADE rule must stay installed alongside the relay")
	}

	inst.XrayRelayPort = 0
	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply without relay: %v", err)
	}
	if got := len(f.divertRules()); got != 0 {
		t.Fatalf("divert rules must be removed on fallback, got %v", f.divertRules())
	}
	if f.has("-d 127.0.0.1 -j ACCEPT") {
		t.Fatal("INPUT exception must go away with the divert rules")
	}
	if st, _ := loadNetworkState(inst.Id); st.XrayRelayPort != 0 {
		t.Fatalf("state still carries a relay port: %+v", st)
	}

	inst.XrayRelayPort = 63907
	if err := m.Apply(inst); err != nil {
		t.Fatalf("re-enable relay: %v", err)
	}
	m.Remove(inst.Id)
	if len(f.present) != 0 {
		t.Fatalf("Remove must delete every managed rule, left: %v", f.present)
	}
	if tproxy.ActiveOwners() != 0 {
		t.Fatalf("no tproxy owners expected after Remove, got %d", tproxy.ActiveOwners())
	}
}

func TestNetworkManagerRelaySkipsUnderFirewalld(t *testing.T) {
	useTempL2TPRoot(t)
	inst := validInstance()
	if err := os.MkdirAll(dataDirForID(inst.Id), 0o750); err != nil {
		t.Fatal(err)
	}
	f := newFakeFirewall()
	f.firewalld = true
	m := newTestNetworkManager(f)
	inst.XrayRelayPort = 63907

	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply under firewalld: %v", err)
	}
	if got := len(f.divertRules()); got != 0 {
		t.Fatalf("no divert rules expected under firewalld, got %v", f.divertRules())
	}
	if f.has("-d 127.0.0.1 -j ACCEPT") {
		t.Fatal("no INPUT exception expected under firewalld")
	}
	if tproxy.ActiveOwners() != 0 {
		t.Fatal("relay owner must not be acquired under firewalld")
	}
	if m.relayWarned[inst.Id] != tproxy.FirewalldWarning {
		t.Fatalf("operator warning expected, got %q", m.relayWarned[inst.Id])
	}
	if !f.has("FORWARD -s 10.252.0.0/24 -j ACCEPT") {
		t.Fatal("direct-path rules must stay in place")
	}
	f.firewalld = false
	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply after firewalld stopped: %v", err)
	}
	if got := len(f.divertRules()); got != 2 {
		t.Fatalf("relay must resume once firewalld is gone, got %d nat divert rules", got)
	}
	m.Remove(inst.Id)
}

func TestNetworkManagerRelayFailsOpen(t *testing.T) {
	useTempL2TPRoot(t)
	inst := validInstance()
	if err := os.MkdirAll(dataDirForID(inst.Id), 0o750); err != nil {
		t.Fatal(err)
	}
	f := newFakeFirewall()
	f.failMatch = "-j DNAT"
	m := newTestNetworkManager(f)
	inst.XrayRelayPort = 63907

	if err := m.Apply(inst); err != nil {
		t.Fatalf("a failing DNAT target must not fail Apply: %v", err)
	}
	if got := len(f.divertRules()); got != 0 {
		t.Fatalf("no divert rules expected, got %v", f.divertRules())
	}
	if f.has("-d 127.0.0.1 -j ACCEPT") {
		t.Fatal("the INPUT exception installed before the failing rule must be rolled back too")
	}
	if st, _ := loadNetworkState(inst.Id); st.XrayRelayPort != 0 {
		t.Fatalf("state must record the direct path, got %+v", st)
	}
	if !f.has("FORWARD -s 10.252.0.0/24 -j ACCEPT") {
		t.Fatal("direct-path rules must be installed regardless of relay support")
	}
	m.Remove(inst.Id)
	if tproxy.ActiveOwners() != 0 {
		t.Fatalf("no tproxy owners expected, got %d", tproxy.ActiveOwners())
	}
}
