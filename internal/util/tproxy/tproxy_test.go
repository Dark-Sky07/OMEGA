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
	calls    []string
	ruleSeen bool
	failAdd  bool
}

func (f *fakeRunner) run(name string, args ...string) ([]byte, error) {
	cmd := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	switch {
	case cmd == "ip -4 rule show":
		if f.ruleSeen {
			return []byte("2601:\tfrom all fwmark 0x2e01 lookup 2601\n"), nil
		}
		return []byte("0:\tfrom all lookup local\n32766:\tfrom all lookup main\n"), nil
	case strings.HasPrefix(cmd, "ip -4 rule add"):
		if f.failAdd {
			return []byte("RTNETLINK answers: Operation not permitted"), errors.New("exit status 2")
		}
		f.ruleSeen = true
		return nil, nil
	case strings.HasPrefix(cmd, "ip -4 rule del"):
		if !f.ruleSeen {
			return []byte("RTNETLINK answers: No such file or directory"), errors.New("exit status 2")
		}
		f.ruleSeen = false
		return nil, nil
	}
	return nil, nil
}

func resetOwners() {
	mu.Lock()
	owners = map[string]struct{}{}
	mu.Unlock()
}

func TestAcquireReleaseLifecycle(t *testing.T) {
	resetOwners()
	t.Cleanup(resetOwners)
	f := &fakeRunner{}

	if err := Acquire("openvpn:1", f.run); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if !f.ruleSeen {
		t.Fatal("first acquire must add the ip rule")
	}
	if !containsPrefix(f.calls, "ip -4 route replace local 0.0.0.0/0 dev lo table 2601") {
		t.Fatalf("local route was not installed: %v", f.calls)
	}
	adds := countPrefix(f.calls, "ip -4 rule add")
	if err := Acquire("l2tp:2", f.run); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if countPrefix(f.calls, "ip -4 rule add") != adds {
		t.Fatal("second acquire must not add a duplicate ip rule")
	}
	if ActiveOwners() != 2 {
		t.Fatalf("expected two owners, got %d", ActiveOwners())
	}

	Release("openvpn:1", f.run)
	if !f.ruleSeen {
		t.Fatal("rule must survive while another owner remains")
	}
	Release("l2tp:2", f.run)
	if f.ruleSeen {
		t.Fatal("rule must be removed once the last owner releases")
	}
	if !containsPrefix(f.calls, "ip -4 route flush table 2601") {
		t.Fatalf("table was not flushed: %v", f.calls)
	}
	if ActiveOwners() != 0 {
		t.Fatalf("expected no owners, got %d", ActiveOwners())
	}
}

func TestAcquireFailsWithoutRegistering(t *testing.T) {
	resetOwners()
	t.Cleanup(resetOwners)
	f := &fakeRunner{failAdd: true}
	if err := Acquire("openvpn:9", f.run); err == nil {
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
