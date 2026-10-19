package tproxy

import (
	"errors"
	"strings"
	"testing"
)

func TestDivertRulesShape(t *testing.T) {
	rules := DivertRules("tun7", "10.7.0.0/24", 63907)
	if len(rules) != 2 {
		t.Fatalf("expected two divert rules, got %d", len(rules))
	}
	want := []string{
		"PREROUTING -i tun7 -s 10.7.0.0/24 ! -d 10.7.0.0/24 -p tcp -j DNAT --to-destination 127.0.0.1:63907",
		"PREROUTING -i tun7 -s 10.7.0.0/24 -p udp --dport 53 -j DNAT --to-destination 127.0.0.1:63907",
	}
	for i, w := range want {
		if got := strings.Join(rules[i], " "); got != w {
			t.Errorf("rule %d = %q, want %q", i, got, w)
		}
	}
}

func TestAcceptRulesShape(t *testing.T) {
	rules := AcceptRules("ppp+", "10.252.0.0/24")
	if len(rules) != 2 {
		t.Fatalf("expected two accept/filter rules, got %d", len(rules))
	}
	want := []string{
		"INPUT -i ppp+ -s 10.252.0.0/24 -d 127.0.0.1 -j ACCEPT",
		"FORWARD -i ppp+ -s 10.252.0.0/24 -p udp --dport 443 -j DROP",
	}
	for i, w := range want {
		if got := strings.Join(rules[i], " "); got != w {
			t.Fatalf("accept rule %d = %q, want %q", i, got, w)
		}
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
	calls      []string
	failSysctl bool
}

func (f *fakeRunner) run(name string, args ...string) ([]byte, error) {
	cmd := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	if name == "sysctl" && f.failSysctl {
		return []byte("permission denied"), errors.New("exit status 1")
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
	if !containsPrefix(f.calls, "sysctl -w net.ipv4.conf.all.route_localnet=1") {
		t.Fatalf("route_localnet sysctl was not applied: %v", f.calls)
	}
	if !containsPrefix(f.calls, "sysctl -w net.ipv4.conf.tun1.route_localnet=1") {
		t.Fatalf("tun1 route_localnet sysctl was not applied: %v", f.calls)
	}
	if err := Acquire("l2tp:2", "10.252.0.0/24", f.run); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if ActiveOwners() != 2 {
		t.Fatalf("expected two owners, got %d", ActiveOwners())
	}

	Release("openvpn:1", f.run)
	if ActiveOwners() != 1 {
		t.Fatalf("expected one owner remaining, got %d", ActiveOwners())
	}
	Release("l2tp:2", f.run)
	if ActiveOwners() != 0 {
		t.Fatalf("expected no owners, got %d", ActiveOwners())
	}
}

func TestHasReverseRule(t *testing.T) {
	cases := map[string]bool{
		"2600:\tfrom all iif lo fwmark 0x2e01 lookup main\n":                                            true,
		"2600:\tfrom all fwmark 0x2e01 iif lo lookup main\n":                                            true,
		"2600:\tfrom all iif lo fwmark 0x2e01/0xffffffff lookup 254\n":                                  true,
		"2601:\tfrom all fwmark 0x2e01 lookup 2601\n":                                                   false,
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
	if err := Acquire("openvpn:9", "10.9.0.0/24", nil); err == nil {
		t.Fatal("acquire must fail when runner is nil")
	}
	if ActiveOwners() != 0 {
		t.Fatal("a failed acquire must not register an owner")
	}
}

func containsPrefix(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
