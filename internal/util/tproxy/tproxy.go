// Package tproxy holds the shared helpers used by the OpenVPN and L2TP/IPsec
// network managers when they divert their clients' traffic into Xray instead
// of forwarding it straight to the internet: the iptables nat/filter rule
// shapes, the kernel sysctl configuration (route_localnet=1 / rp_filter=0)
// those rules depend on, and a runtime capability probe.
//
// Why nat PREROUTING DNAT ("redirect") instead of mangle TPROXY:
//   - Many production VPS kernels (cloud KVM minimal kernels, LXC/OpenVZ,
//     nftables-backed distros without xt_TPROXY/xt_socket loaded) do not
//     provide the TPROXY target or "-m socket --transparent", whereas every
//     host running OpenVPN/L2TP already has iptables nat (DNAT + MASQUERADE).
//   - With mangle TPROXY + policy routing table 2601 + Xray FakeUDP, UDP
//     return packets and src_valid_mark reverse-path checks frequently drop
//     traffic after the VPN tunnel connects.
//   - With nat PREROUTING DNAT to 127.0.0.1:<port> (plus route_localnet=1),
//     Xray's dokodemo-door TCP listener reads the original destination via
//     SO_ORIGINAL_DST ("tproxy": "redirect"), sniffs the domain with
//     routeOnly=true so both IP and domain routing rules match, and a
//     companion UDP dokodemo-door listener on the same loopback port forwards
//     DNS (UDP/53) queries through Xray's router while QUIC (UDP/443) is
//     dropped in FORWARD so browsers seamlessly fall back to TCP/443.
package tproxy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	// Mark, RouteTable, RulePriority and ReversePriority are retained for
	// backwards-compatible cleanup of legacy mangle/policy-routing rules left
	// behind by older releases.
	Mark            = 0x2e01
	RouteTable      = 2601
	RulePriority    = 2601
	ReversePriority = 2600

	// DivertTable is the iptables table where DivertRules are installed.
	DivertTable = "nat"
	// LegacyDivertTable is the old mangle table swept during upgrades/removals.
	LegacyDivertTable = "mangle"

	// ListenIP is the loopback address the Xray relay listener binds.
	ListenIP = "127.0.0.1"
)

// Runner executes an external command and returns its combined output.
type Runner func(name string, args ...string) ([]byte, error)

func markArg() string {
	return fmt.Sprintf("0x%x/0xffffffff", Mark)
}

// DivertRules returns the nat-table PREROUTING rules that redirect client
// traffic arriving on iface from pool to the Xray relay on ListenIP:port:
//   - All TCP traffic leaving the VPN subnet (! -d pool) is DNATed to
//     127.0.0.1:port, where Xray's dokodemo-door ("tproxy": "redirect")
//     recovers the original destination via SO_ORIGINAL_DST.
//   - All UDP port 53 (DNS) traffic from the VPN subnet is DNATed to
//     127.0.0.1:port, where the companion UDP dokodemo-door listener forwards
//     queries through Xray's router to the inbound's configured DNS server.
func DivertRules(iface, pool string, port int) [][]string {
	dest := fmt.Sprintf("%s:%d", ListenIP, port)
	return [][]string{
		{"PREROUTING", "-i", iface, "-s", pool, "!", "-d", pool, "-p", "tcp", "-j", "DNAT", "--to-destination", dest},
		{"PREROUTING", "-i", iface, "-s", pool, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", dest},
	}
}

// LegacyDivertRules returns the old mangle-table TPROXY rules from earlier
// releases so callers can sweep them when upgrading a live server.
func LegacyDivertRules(iface, pool string, port int) [][]string {
	portStr := strconv.Itoa(port)
	mark := markArg()
	return [][]string{
		{"PREROUTING", "-i", iface, "-s", pool, "-p", "tcp", "!", "--syn", "-m", "socket", "--transparent", "-j", "MARK", "--set-xmark", mark},
		{"PREROUTING", "-i", iface, "-s", pool, "!", "-d", pool, "-m", "addrtype", "!", "--dst-type", "LOCAL", "-p", "tcp", "--syn", "-j", "TPROXY", "--on-ip", ListenIP, "--on-port", portStr, "--tproxy-mark", mark},
		{"PREROUTING", "-i", iface, "-s", pool, "!", "-d", pool, "-m", "addrtype", "!", "--dst-type", "LOCAL", "-p", "udp", "-j", "TPROXY", "--on-ip", ListenIP, "--on-port", portStr, "--tproxy-mark", mark},
	}
}

// AcceptRules returns the filter-table rules paired with DivertRules:
//   - INPUT exception scoped to -d 127.0.0.1 so packets DNATed to loopback
//     are accepted on hosts with a restrictive INPUT policy, without colliding
//     with OpenVPN's base `INPUT -i <iface> -s <pool> -j ACCEPT` rule when the
//     relay is later toggled off.
//   - FORWARD drop for UDP/443 (QUIC) from the VPN pool so browsers and apps
//     immediately fall back to TCP/443 and traverse the Xray relay instead of
//     bypassing Xray over direct UDP.
func AcceptRules(iface, pool string) [][]string {
	return [][]string{
		{"INPUT", "-i", iface, "-s", pool, "-d", ListenIP, "-j", "ACCEPT"},
		{"FORWARD", "-i", iface, "-s", pool, "-p", "udp", "--dport", "443", "-j", "DROP"},
	}
}

// LegacyAcceptRules returns the old mark-based filter INPUT rule from earlier
// releases so callers can remove it if left behind across an upgrade.
func LegacyAcceptRules(iface, pool string) [][]string {
	return [][]string{
		{"INPUT", "-i", iface, "-s", pool, "-m", "mark", "--mark", markArg(), "-j", "ACCEPT"},
	}
}

// FirewalldActive reports whether firewalld manages this host's firewall.
func FirewalldActive(run Runner) bool {
	if run == nil {
		return false
	}
	out, err := run("systemctl", "is-active", "firewalld")
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

// FirewalldWarning is the operator-facing explanation logged (once) when the
// relay stays off because of FirewalldActive.
const FirewalldWarning = "firewalld is active on this host; the Xray relay is not installed because firewalld's INPUT policy would drop the diverted packets, so this inbound stays on the direct path and routing rules do not apply to it (stop firewalld to enable the relay, or turn the inbound's \"Route through Xray\" switch off to silence this warning)"

var (
	mu     sync.Mutex
	owners = map[string]string{}
)

// Acquire enables the kernel sysctls required to DNAT tunnel traffic to
// 127.0.0.1 (route_localnet=1 and rp_filter=0 on all/default/lo and the tunnel
// interface), cleans up any leftover policy-routing rules from older releases,
// and records owner as an active relay user.
func Acquire(owner, pool string, run Runner) error {
	if run == nil {
		return errors.New("tproxy: no command runner")
	}
	mu.Lock()
	defer mu.Unlock()
	if err := ensureLocalnetSysctls(owner, run); err != nil {
		return err
	}
	owners[owner] = pool
	return nil
}

// Release drops owner's registration.
func Release(owner string, run Runner) {
	if run == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	delete(owners, owner)
}

// ActiveOwners reports how many owners currently hold the relay sysctls.
func ActiveOwners() int {
	mu.Lock()
	defer mu.Unlock()
	return len(owners)
}

func setSysctl(run Runner, key, value string) error {
	if out, err := run("sysctl", "-w", key+"="+value); err != nil {
		procPath := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
		if wErr := os.WriteFile(procPath, []byte(value+"\n"), 0o644); wErr != nil {
			return fmt.Errorf("sysctl -w %s=%s: %s: %w", key, value, strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

func ensureLocalnetSysctls(owner string, run Runner) error {
	// Global + default route_localnet=1 are mandatory so the kernel allows
	// PREROUTING DNAT to 127.0.0.1 on both existing and future interfaces.
	for _, key := range []string{
		"net.ipv4.conf.all.route_localnet",
		"net.ipv4.conf.default.route_localnet",
	} {
		if err := setSysctl(run, key, "1"); err != nil {
			return err
		}
	}
	// Best-effort rp_filter relaxation so return packets from 127.0.0.1 are
	// never dropped as martians on strict-RPF hosts.
	for _, key := range []string{
		"net.ipv4.conf.all.rp_filter",
		"net.ipv4.conf.default.rp_filter",
		"net.ipv4.conf.lo.rp_filter",
	} {
		_ = setSysctl(run, key, "0")
	}
	if strings.HasPrefix(owner, "openvpn:") {
		id := strings.TrimPrefix(owner, "openvpn:")
		if id != "" {
			iface := "tun" + id
			_ = setSysctl(run, "net.ipv4.conf."+iface+".route_localnet", "1")
			_ = setSysctl(run, "net.ipv4.conf."+iface+".rp_filter", "0")
		}
	} else if strings.HasPrefix(owner, "l2tp:") {
		if matches, err := filepath.Glob("/proc/sys/net/ipv4/conf/ppp*"); err == nil {
			for _, m := range matches {
				iface := filepath.Base(m)
				_ = setSysctl(run, "net.ipv4.conf."+iface+".route_localnet", "1")
				_ = setSysctl(run, "net.ipv4.conf."+iface+".rp_filter", "0")
			}
		}
	}
	return nil
}

// HasReverseRule reports whether the "ip -4 rule show" output contains the
// legacy reverse-path guard rule (iif lo + fwmark Mark -> main).
func HasReverseRule(rulesOutput string) bool {
	for _, line := range strings.Split(rulesOutput, "\n") {
		fields := strings.Fields(line)
		var markOK, iifOK, mainOK bool
		for i := 0; i+1 < len(fields); i++ {
			switch fields[i] {
			case "fwmark":
				markOK = markTokenMatches(fields[i+1])
			case "iif":
				iifOK = fields[i+1] == "lo"
			case "lookup", "table":
				mainOK = fields[i+1] == "main" || fields[i+1] == "254"
			}
		}
		if markOK && iifOK && mainOK {
			return true
		}
	}
	return false
}

// HasMarkRule reports whether the "ip -4 rule show" output contains the
// legacy fwmark -> RouteTable rule.
func HasMarkRule(rulesOutput string) bool {
	for _, line := range strings.Split(rulesOutput, "\n") {
		fields := strings.Fields(line)
		var markOK, tableOK bool
		for i := 0; i+1 < len(fields); i++ {
			switch fields[i] {
			case "fwmark":
				markOK = markTokenMatches(fields[i+1])
			case "lookup", "table":
				tableOK = fields[i+1] == strconv.Itoa(RouteTable)
			}
		}
		if markOK && tableOK {
			return true
		}
	}
	return false
}

func markTokenMatches(tok string) bool {
	if slash := strings.IndexByte(tok, '/'); slash >= 0 {
		tok = tok[:slash]
	}
	v, err := strconv.ParseUint(tok, 0, 32)
	return err == nil && v == Mark
}
