package tproxy

import (
	"errors"
	"strings"
	"testing"
)

func TestDivertRulesShape(t *testing.T) {
	rules := DivertRules("tun7", "10.7.0.0/24", 63907)
	if len(rules) != 3 {
		t.Fatalf("expected three divert rules, got %d", len(rules))
	}
	want := []string{
		"PREROUTING -i tun7 -s 10.7.0.0/24 -p tcp ! --syn -m socket --transparent -j MARK --set-xmark 0x2e01/0xffffffff",
		"PREROUTING -i tun7 -s 10.7.0.0/24 ! -d 10.7.0.0/24 -m addrtype ! --dst-type LOCAL -p tcp --syn -j TPROXY --on-ip 127.0.0.1 --on-port 63907 --tproxy-mark 0x2e01/0xffffffff",
		"PREROUTING -i tun7 -s 10.7.0.0/24 ! -d 10.7.0.0/24 -m addrtype ! --dst-type LOCAL -p udp -j TPROXY --on-ip 127.0.0.1 --on-port 63907 --tproxy-mark 0x2e01/0xffffffff",
	}
	for i, w := range want {
		if got := strings.Join(rules[i], " "); got != w {
			t.Errorf("rule %d = %q, want %q", i, got, w)
		}
	}
}

func TestAcceptRulesShape(t *testing.T) {
	rules := AcceptRules("ppp+", "10.252.0.0/24")
	if len(rules) != 1 {
		t.Fatalf("expected one accept rule, got %d", len(rules))
	}
	want := "INPUT -i ppp+ -s 10.252.0.0/24 -m mark --mark 0x2e01/0xffffffff -j ACCEPT"
	if got := strings.Join(rules[0], " "); got != want {
		t.Fatalf("accept rule = %q, want %q", got, want)
	}
}

func TestFirewalldActive(t *testing.T) {
	active := func(name string, args ...string) ([]byte, error) {
		if name == "systemctl" && len(args) == 2 && args[0] == "is-active" && args[1] == "firewalld" {
			return []byte("active\n"), nil
		}
		return nil, errors.New("unexpected command")
	}
	if !FirewalldActive(active) {
		t.Fatal("active firewalld not detected")
	}
	inactive := func(string, ...string) ([]byte, error) { return []byte("inactive\n"), errors.New("exit status 3") }
	if FirewalldActive(inactive) {
		t.Fatal("inactive firewalld reported active")
	}
	missing := func(string, ...string) ([]byte, error) { return nil, errors.New("executable file not found") }
	if FirewalldActive(missing) || FirewalldActive(nil) {
		t.Fatal("hosts without systemctl must count as no firewalld")
	}
}

func TestHasMarkRule(t *testing.T) {
	cases := map[string]bool{
		"0:\tfrom all lookup local\n2601:\tfrom all fwmark 0x2e01 lookup 2601\n32766:\tfrom all lookup main\n": true,
		"2601:\tfrom all fwmark 0x2e01/0xffffffff lookup 2601\n":                                               true,
		"2601:\tfrom all fwmark 11777 lookup 2601\n":                                                           true,
		"2601:\tfrom all fwmark 0x2e01 lookup 100\n":                                                           false,
		"2601:\tfrom all fwmark 0x1 lookup 2601\n":                                                             false,
		"": false,
	}
	for in, want := range cases {
		if got := HasMarkRule(in); got != want {
			t.Errorf("HasMarkRule(%q) = %v, want %v", in, got, want)
		}
	}
}

type fakeRunner struct {
	calls       []string
	ruleSeen    bool // pref 2601 fwmark -> 2601
	reverseSeen bool // pref 2600 iif lo fwmark -> main
	failAdd     bool
	throws      map[string]bool
}

func (f *fakeRunner) run(name string, args ...string) ([]byte, error) {
	cmd := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	if f.throws == nil {
		f.throws = map[string]bool{}
	}
	switch {
	case cmd == "ip -4 rule show":
		out := "0:\tfrom all lookup local\n"
		if f.reverseSeen {
			out += "2600:\tfrom all iif lo fwmark 0x2e01 lookup main\n"
		}
		if f.ruleSeen {
			out += "2601:\tfrom all fwmark 0x2e01 lookup 2601\n"
		}
		return []byte(out + "32766:\tfrom all lookup main\n"), nil
	case strings.HasPrefix(cmd, "ip -4 rule add pref 2600 "):
		if f.failAdd {
			return []byte("RTNETLINK answers: Operation not permitted"), errors.New("exit status 2")
		}
		f.reverseSeen = true
		return nil, nil
	case strings.HasPrefix(cmd, "ip -4 rule add pref 2601 "):
		if f.failAdd {
			return []byte("RTNETLINK answers: Operation not permitted"), errors.New("exit status 2")
		}
		f.ruleSeen = true
		return nil, nil
	case strings.HasPrefix(cmd, "ip -4 rule del pref 2600 "):
		if !f.reverseSeen {
			return []byte("RTNETLINK answers: No such file or directory"), errors.New("exit status 2")
		}
		f.reverseSeen = false
		return nil, nil
	case strings.HasPrefix(cmd, "ip -4 rule del pref 2601 "):
		if !f.ruleSeen {
			return []byte("RTNETLINK answers: No such file or directory"), errors.New("exit status 2")
		}
		f.ruleSeen = false
		return nil, nil
	case strings.HasPrefix(cmd, "ip -4 route replace throw "):
		f.throws[args[4]] = true
		return nil, nil
	case strings.HasPrefix(cmd, "ip -4 route del throw "):
		if !f.throws[args[4]] {
			return []byte("RTNETLINK answers: No such process"), errors.New("exit status 2")
		}
		delete(f.throws, args[4])
		return nil, nil
	case cmd == "ip -4 route flush table 2601":
		f.throws = map[string]bool{}
		return nil, nil
	}
	return nil, nil
}

func resetOwners() {
	mu.Lock()
	owners = map[string]string{}
	mu.Unlock()
}

func TestAcquireReleaseLifecycle(t *testing.T) {
	resetOwners()
	t.Cleanup(resetOwners)
	f := &fakeRunner{}

	if err := Acquire("openvpn:1", "10.1.0.0/24", f.run); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if !f.ruleSeen || !f.reverseSeen {
		t.Fatalf("first acquire must add both ip rules (deliver=%v guard=%v)", f.ruleSeen, f.reverseSeen)
	}
	if !containsPrefix(f.calls, "ip -4 route replace local 0.0.0.0/0 dev lo table 2601") {
		t.Fatalf("local route was not installed: %v", f.calls)
	}
	if !f.throws["10.1.0.0/24"] {
		t.Fatalf("throw route for the pool was not installed: %v", f.calls)
	}
	// Guard rule and throw route before the delivering rule: on a
	// src_valid_mark host no packet may ever be routed to table 2601
	// without them.
	guard := indexPrefix(f.calls, "ip -4 rule add pref 2600 iif lo fwmark 0x2e01 lookup main")
	throwAt := indexPrefix(f.calls, "ip -4 route replace throw 10.1.0.0/24 table 2601")
	deliver := indexPrefix(f.calls, "ip -4 rule add pref 2601 fwmark 0x2e01 lookup 2601")
	if guard < 0 || throwAt < 0 || deliver < 0 || guard > deliver || throwAt > deliver {
		t.Fatalf("bad install order guard=%d throw=%d deliver=%d: %v", guard, throwAt, deliver, f.calls)
	}
	adds := countPrefix(f.calls, "ip -4 rule add")
	if err := Acquire("l2tp:2", "10.252.0.0/24", f.run); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if countPrefix(f.calls, "ip -4 rule add") != adds {
		t.Fatal("second acquire must not add duplicate ip rules")
	}
	if !f.throws["10.252.0.0/24"] || !f.throws["10.1.0.0/24"] {
		t.Fatalf("both pools need their throw route: %v", f.throws)
	}
	if ActiveOwners() != 2 {
		t.Fatalf("expected two owners, got %d", ActiveOwners())
	}

	// A pool change on a live owner swaps its throw route.
	if err := Acquire("openvpn:1", "10.9.0.0/24", f.run); err != nil {
		t.Fatalf("re-acquire with new pool: %v", err)
	}
	if f.throws["10.1.0.0/24"] || !f.throws["10.9.0.0/24"] {
		t.Fatalf("old pool's throw must go, new one must exist: %v", f.throws)
	}
	// Two owners on one pool share the throw route.
	if err := Acquire("openvpn:3", "10.252.0.0/24", f.run); err != nil {
		t.Fatalf("shared-pool acquire: %v", err)
	}
	Release("openvpn:3", f.run)
	if !f.throws["10.252.0.0/24"] {
		t.Fatal("throw route must survive while another owner still uses the pool")
	}

	Release("openvpn:1", f.run)
	if !f.ruleSeen || !f.reverseSeen {
		t.Fatal("rules must survive while another owner remains")
	}
	if f.throws["10.9.0.0/24"] {
		t.Fatal("released owner's throw route must be removed")
	}
	Release("l2tp:2", f.run)
	if f.ruleSeen || f.reverseSeen {
		t.Fatal("rules must be removed once the last owner releases")
	}
	if !containsPrefix(f.calls, "ip -4 route flush table 2601") {
		t.Fatalf("table was not flushed: %v", f.calls)
	}
	if len(f.throws) != 0 {
		t.Fatalf("flush must clear the throw routes: %v", f.throws)
	}
	if ActiveOwners() != 0 {
		t.Fatalf("expected no owners, got %d", ActiveOwners())
	}
}

func indexPrefix(calls []string, prefix string) int {
	for i, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

func TestHasReverseRule(t *testing.T) {
	cases := map[string]bool{
		"2600:\tfrom all iif lo fwmark 0x2e01 lookup main\n":                                            true,
		"2600:\tfrom all fwmark 0x2e01 iif lo lookup main\n":                                            true, // real iproute2 order
		"2600:\tfrom all iif lo fwmark 0x2e01/0xffffffff lookup 254\n":                                  true,
		"2601:\tfrom all fwmark 0x2e01 lookup 2601\n":                                                   false, // the delivering rule
		"2600:\tfrom all iif eth0 fwmark 0x2e01 lookup main\n":                                          false,
		"32765:\tnot from all fwmark 0xca6c lookup 51820\n":                                             false,
		"0:\tfrom all lookup local\n32766:\tfrom all lookup main\n":                                     false,
		"2600:\tfrom all iif lo fwmark 0x2e01 lookup main\n2601:\tfrom all fwmark 0x2e01 lookup 2601\n": true,
	}
	for in, want := range cases {
		if got := HasReverseRule(in); got != want {
			t.Errorf("HasReverseRule(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestAcquireFailsWithoutRegistering(t *testing.T) {
	resetOwners()
	t.Cleanup(resetOwners)
	f := &fakeRunner{failAdd: true}
	if err := Acquire("openvpn:9", "10.9.0.0/24", f.run); err == nil {
		t.Fatal("acquire must surface the ip rule failure")
	}
	if ActiveOwners() != 0 {
		t.Fatal("a failed acquire must not register an owner")
	}
}

func containsPrefix(calls []string, prefix string) bool {
	return countPrefix(calls, prefix) > 0
}

func countPrefix(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}
