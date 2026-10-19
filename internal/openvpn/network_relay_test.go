package openvpn

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/util/tproxy"
)

// fakeFirewall emulates iptables -C/-I/-D bookkeeping plus the sysctl/systemctl
// helpers so the network manager can be exercised without root or a kernel.
type fakeFirewall struct {
	mu        sync.Mutex
	present   map[string]bool
	calls     []string
	failMatch string // an -I whose rule contains this substring fails
	firewalld bool   // systemctl is-active firewalld reports "active"
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
		runner:      f.run,
		lookPath:    func(string) (string, error) { return "/sbin/iptables", nil },
		relayWarned: make(map[int]string),
	}
}

func readState(t *testing.T, id int) networkState {
	t.Helper()
	data, err := os.ReadFile(networkStatePath(id))
	if err != nil {
		t.Fatalf("read network state: %v", err)
	}
	var st networkState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("decode network state: %v", err)
	}
	return st
}

func TestNetworkManagerRelayLifecycle(t *testing.T) {
	tempBinFolder(t)
	if err := os.MkdirAll(dataDirForID(7), 0o750); err != nil {
		t.Fatal(err)
	}
	f := newFakeFirewall()
	m := newTestNetworkManager(f)
	inst := Instance{Id: 7, Port: 1194, Proto: "udp", XrayRelayPort: 63907}

	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply with relay: %v", err)
	}
	if got := len(f.divertRules()); got != 2 {
		t.Fatalf("expected two nat PREROUTING divert rules, got %d: %v", got, f.divertRules())
	}
	if !f.has("-t nat PREROUTING -i tun7 -s 10.7.0.0/24 ! -d 10.7.0.0/24 -p tcp -j DNAT --to-destination 127.0.0.1:63907") {
		t.Fatalf("TCP DNAT rule missing: %v", f.divertRules())
	}
	if !f.has("-t nat PREROUTING -i tun7 -s 10.7.0.0/24 -p udp --dport 53 -j DNAT --to-destination 127.0.0.1:63907") {
		t.Fatalf("UDP DNS DNAT rule missing: %v", f.divertRules())
	}
	if !f.has("-t filter INPUT -i tun7 -s 10.7.0.0/24 -d 127.0.0.1 -j ACCEPT") {
		t.Fatal("INPUT exception for diverted packets missing")
	}
	if !f.has("-t filter FORWARD -i tun7 -s 10.7.0.0/24 -p udp --dport 443 -j DROP") {
		t.Fatal("QUIC UDP/443 drop rule missing")
	}
	if st := readState(t, 7); st.XrayRelayPort != 63907 || st.PoolCIDR != "10.7.0.0/24" {
		t.Fatalf("persisted state = %+v", st)
	}
	if !f.has("POSTROUTING -s 10.7.0.0/24 ! -d 10.7.0.0/24 -j MASQUERADE") {
		t.Fatal("direct-path MASQUERADE rule must stay installed alongside the relay")
	}

	// Same port again: idempotent, nothing re-inserted.
	before := len(f.calls)
	if err := m.Apply(inst); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	for _, c := range f.calls[before:] {
		if strings.Contains(c, " -I ") {
			t.Fatalf("re-apply must not insert rules again: %s", c)
		}
	}

	// Port moved (Xray regenerated its relay on another port).
	inst.XrayRelayPort = 63999
	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply moved port: %v", err)
	}
	if f.has("--to-destination 127.0.0.1:63907") || !f.has("--to-destination 127.0.0.1:63999") {
		t.Fatalf("old divert rules must be swapped for the new port: %v", f.divertRules())
	}
	if got := len(f.divertRules()); got != 2 {
		t.Fatalf("expected two nat PREROUTING rules after port move, got %d", got)
	}

	// Relay gone (Xray stopped or toggle off): back to the direct path.
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
	if !f.has("INPUT -i tun7 -s 10.7.0.0/24 -j ACCEPT") {
		t.Fatal("base OpenVPN INPUT rule must survive relay removal")
	}
	if st := readState(t, 7); st.XrayRelayPort != 0 {
		t.Fatalf("persisted state still carries a relay port: %+v", st)
	}
	if !f.has("POSTROUTING -s 10.7.0.0/24 ! -d 10.7.0.0/24 -j MASQUERADE") {
		t.Fatal("direct-path rules must survive the relay teardown")
	}

	// Relay back, then a full Remove tears everything down.
	inst.XrayRelayPort = 63907
	if err := m.Apply(inst); err != nil {
		t.Fatalf("re-enable relay: %v", err)
	}
	m.Remove(7)
	if len(f.present) != 0 {
		t.Fatalf("Remove must delete every managed rule, left: %v", f.present)
	}
	if tproxy.ActiveOwners() != 0 {
		t.Fatalf("no tproxy owners expected after Remove, got %d", tproxy.ActiveOwners())
	}
}

func TestNetworkManagerRelayFailsOpen(t *testing.T) {
	tempBinFolder(t)
	if err := os.MkdirAll(dataDirForID(8), 0o750); err != nil {
		t.Fatal(err)
	}
	f := newFakeFirewall()
	f.failMatch = "-j DNAT"
	m := newTestNetworkManager(f)
	inst := Instance{Id: 8, Port: 1194, Proto: "udp", XrayRelayPort: 63908}

	if err := m.Apply(inst); err != nil {
		t.Fatalf("a failing DNAT target must not fail Apply: %v", err)
	}
	if got := len(f.divertRules()); got != 0 {
		t.Fatalf("partial divert rules must be rolled back, got %v", f.divertRules())
	}
	if f.has("-d 127.0.0.1 -j ACCEPT") {
		t.Fatal("the INPUT exception installed before the failing rule must be rolled back too")
	}
	if st := readState(t, 8); st.XrayRelayPort != 0 {
		t.Fatalf("state must record the direct path, got %+v", st)
	}
	if !f.has("FORWARD -i tun8 -s 10.8.0.0/24 -j ACCEPT") {
		t.Fatal("direct-path rules must be installed regardless of relay support")
	}
	if m.relayWarned[8] == "" {
		t.Fatal("relay failure should be remembered for warning de-duplication")
	}
	warned := m.relayWarned[8]
	if err := m.Apply(inst); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if m.relayWarned[8] != warned {
		t.Fatal("identical failure must not be re-logged")
	}
	m.Remove(8)
	if tproxy.ActiveOwners() != 0 {
		t.Fatalf("no tproxy owners expected, got %d", tproxy.ActiveOwners())
	}
}

func TestNetworkManagerRelaySkipsUnderFirewalld(t *testing.T) {
	tempBinFolder(t)
	if err := os.MkdirAll(dataDirForID(9), 0o750); err != nil {
		t.Fatal(err)
	}
	f := newFakeFirewall()
	m := newTestNetworkManager(f)
	inst := Instance{Id: 9, Port: 1194, Proto: "udp", XrayRelayPort: 63909}

	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := len(f.divertRules()); got != 2 {
		t.Fatalf("expected the relay to be active first, got %d nat divert rules", got)
	}
	f.firewalld = true
	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply under firewalld: %v", err)
	}
	if got := len(f.divertRules()); got != 0 {
		t.Fatalf("divert rules must be withdrawn under firewalld, got %v", f.divertRules())
	}
	if f.has("-d 127.0.0.1 -j ACCEPT") {
		t.Fatal("INPUT exception must be withdrawn under firewalld")
	}
	if tproxy.ActiveOwners() != 0 {
		t.Fatal("relay owner must be released under firewalld")
	}
	if st := readState(t, 9); st.XrayRelayPort != 0 {
		t.Fatalf("state must record the direct path, got %+v", st)
	}
	if m.relayWarned[9] != tproxy.FirewalldWarning {
		t.Fatalf("operator warning expected, got %q", m.relayWarned[9])
	}
	if !f.has("FORWARD -i tun9 -s 10.9.0.0/24 -j ACCEPT") {
		t.Fatal("direct-path rules must stay in place")
	}

	// firewalld gone again: the relay comes back on the next round.
	f.firewalld = false
	if err := m.Apply(inst); err != nil {
		t.Fatalf("apply after firewalld stopped: %v", err)
	}
	if got := len(f.divertRules()); got != 2 {
		t.Fatalf("relay must resume once firewalld is gone, got %d nat divert rules", got)
	}
	m.Remove(9)
}
