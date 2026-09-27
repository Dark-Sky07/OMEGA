// Package tproxy holds the pieces shared by the OpenVPN and L2TP/IPsec
// network managers when they divert their clients' traffic into Xray instead
// of forwarding it straight to the internet: the iptables TPROXY rule shapes,
// the policy-routing entries those rules depend on, and a runtime capability
// probe.
//
// The data path is the stock Linux transparent-proxy setup. Xray exposes a
// loopback dokodemo-door inbound with "tproxy" sockopts (see
// internal/web/service/xray.go's daemon relay injection); the kernel hands
// new TCP connections and every UDP datagram leaving the VPN pool to that
// listener with the original destination intact, so Xray's router sees the
// same destinations — and the same inbound tag — it would for any native
// inbound. Nothing about the VPN daemons themselves changes: the tunnel,
// authentication and per-client accounting stay where they are.
//
// Everything here is fail-open by design. A host without xt_TPROXY/xt_socket
// (typically a container whose host kernel never loaded them) simply keeps
// the previous direct FORWARD/MASQUERADE path; callers log the reason and
// retry on their next reconcile round.
package tproxy

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const (
	// Mark is the fwmark stamped on diverted packets. RouteTable and
	// RulePriority are the policy-routing slot that delivers packets carrying
	// that mark to the local listener. The values are arbitrary but stable
	// and deliberately far from the marks/tables popular tooling uses
	// (wg-quick 51820, tailscale 0x80000, the v2ray docs' 1/100).
	Mark         = 0x2e01
	RouteTable   = 2601
	RulePriority = 2601
	// ReversePriority is the slot of the companion rule that keeps the
	// kernel's reverse-path check away from RouteTable (see
	// ensurePolicyRouting).
	ReversePriority = 2600

	// ListenIP is the loopback address the Xray relay listener binds.
	ListenIP = "127.0.0.1"
)

// Runner executes an external command and returns its combined output. Both
// network managers already inject one for tests; it is reused here so the
// policy-routing commands go through the same seam.
type Runner func(name string, args ...string) ([]byte, error)

// markArg is the fwmark argument shared by the MARK and TPROXY targets.
func markArg() string {
	return fmt.Sprintf("0x%x/0xffffffff", Mark)
}

// DivertRules returns the mangle-table PREROUTING rules that hand traffic
// from clients of pool arriving on iface to the transparent listener on
// ListenIP:port. Each entry starts with the chain name, matching the
// managers' existing rule slices.
//
//   - TCP: only SYNs are diverted. Connections that were established through
//     the direct FORWARD path before these rules existed keep flowing there
//     instead of being reset mid-stream, so enabling the relay on a live
//     inbound is not disruptive. The remaining packets of a diverted
//     connection already belong to a transparent socket; they only need the
//     mark so policy routing keeps delivering them locally (the classic
//     "-m socket" divert rule).
//   - UDP: every datagram is diverted. The TPROXY target itself looks the
//     matching listener up per packet, and UDP flows survive the switch.
//   - Traffic to the pool itself and to the host's own addresses is left
//     alone: client-to-client and gateway-bound packets (e.g. a resolver on
//     the tunnel IP) never enter Xray.
func DivertRules(iface, pool string, port int) [][]string {
	portStr := strconv.Itoa(port)
	mark := markArg()
	return [][]string{
		{"PREROUTING", "-i", iface, "-s", pool, "-p", "tcp", "!", "--syn", "-m", "socket", "--transparent", "-j", "MARK", "--set-xmark", mark},
		{"PREROUTING", "-i", iface, "-s", pool, "!", "-d", pool, "-m", "addrtype", "!", "--dst-type", "LOCAL", "-p", "tcp", "--syn", "-j", "TPROXY", "--on-ip", ListenIP, "--on-port", portStr, "--tproxy-mark", mark},
		{"PREROUTING", "-i", iface, "-s", pool, "!", "-d", pool, "-m", "addrtype", "!", "--dst-type", "LOCAL", "-p", "udp", "-j", "TPROXY", "--on-ip", ListenIP, "--on-port", portStr, "--tproxy-mark", mark},
	}
}

// AcceptRules returns the filter-table INPUT rules that let diverted packets
// reach the listener on hosts with a default-deny INPUT policy (ufw and the
// like). After TPROXY the packets are delivered locally, so they traverse
// INPUT carrying their original — non-local — destination and would be
// dropped there by such a policy; matching on the divert mark keeps the
// exception exactly as narrow as the divert rules themselves. Callers install
// these before, and remove them after, the DivertRules.
func AcceptRules(iface, pool string) [][]string {
	return [][]string{
		{"INPUT", "-i", iface, "-s", pool, "-m", "mark", "--mark", markArg(), "-j", "ACCEPT"},
	}
}

// FirewalldActive reports whether firewalld manages this host's firewall.
// firewalld keeps its own nftables table whose INPUT chain rejects anything
// its zones do not allow — and it is evaluated after the iptables filter
// table, so the INPUT exception from AcceptRules cannot protect the diverted
// packets there. Callers treat an active firewalld as "relay unavailable"
// (fail-open) rather than risk black-holing the clients' traffic.
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
	mu sync.Mutex
	// owners maps each registered owner to the client pool it diverts.
	owners = map[string]string{}
)

// Acquire registers owner as a user of the shared policy-routing entries and
// makes sure they are installed, including the "throw" route for owner's
// client pool. It is idempotent and cheap, so managers call it on every
// reconcile round before (re)installing their divert rules — an
// administrator flushing "ip rule" is healed on the next tick.
func Acquire(owner, pool string, run Runner) error {
	if run == nil {
		return errors.New("tproxy: no command runner")
	}
	mu.Lock()
	defer mu.Unlock()
	if err := ensurePolicyRouting(pool, run); err != nil {
		return err
	}
	if prev, ok := owners[owner]; ok && prev != pool && prev != "" && !poolInUse(prev, owner) {
		removeThrow(prev, run)
	}
	owners[owner] = pool
	return nil
}

// Release drops owner's registration and removes the policy-routing entries
// once nobody in this process uses them anymore. Callers must remove their
// divert rules first: a TPROXY rule without the routing entries would forward
// diverted packets instead of delivering them.
func Release(owner string, run Runner) {
	if run == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	pool, had := owners[owner]
	delete(owners, owner)
	if len(owners) == 0 {
		removePolicyRouting(run)
		return
	}
	if had && pool != "" && !poolInUse(pool, owner) {
		removeThrow(pool, run)
	}
}

// poolInUse reports whether any owner other than except still diverts pool.
// Must be called with mu held.
func poolInUse(pool, except string) bool {
	for o, p := range owners {
		if o != except && p == pool {
			return true
		}
	}
	return false
}

// ActiveOwners reports how many owners currently hold the policy routing.
func ActiveOwners() int {
	mu.Lock()
	defer mu.Unlock()
	return len(owners)
}

// ensurePolicyRouting installs the routing entries diverted packets need:
//
//	ip rule  pref 2600  iif lo fwmark 0x2e01 lookup main
//	ip rule  pref 2601         fwmark 0x2e01 lookup 2601
//	ip route table 2601: local default dev lo; throw <pool> (per owner)
//
// The second rule and the local route are the textbook TPROXY setup: a
// marked packet is delivered to the local listener whatever its destination.
// The other two entries exist because of the kernel's source validation.
// Before delivering locally it looks the packet's *source* up in reverse,
// and on hosts where net.ipv4.conf.all.src_valid_mark=1 (wg-quick sets that
// for any WireGuard/WARP tunnel with a default route and never clears it)
// that reverse lookup carries the packet's fwmark. Without protection it
// would land in table 2601, find "local default", and the kernel would drop
// the packet as a martian — every diverted connection dead while the tunnel
// itself looks fine. The reverse lookup is keyed on iif lo, so the 2600 rule
// sends it to the main table; the throw route makes table 2601 itself fall
// through for the pool as a second, kernel-version-independent guard. Both
// are no-ops for the forward lookup (iif = tun/ppp, destination outside the
// pool).
func ensurePolicyRouting(pool string, run Runner) error {
	table := strconv.Itoa(RouteTable)
	mark := fmt.Sprintf("0x%x", Mark)
	out, err := run("ip", "-4", "rule", "show")
	if err != nil {
		return fmt.Errorf("ip -4 rule show: %s: %w", strings.TrimSpace(string(out)), err)
	}
	rules := string(out)
	// Order matters on a live host: the guard rule must be in place before
	// the delivering rule, and both before any divert rule.
	if !HasReverseRule(rules) {
		if out, err := run("ip", "-4", "rule", "add", "pref", strconv.Itoa(ReversePriority), "iif", "lo", "fwmark", mark, "lookup", "main"); err != nil {
			return fmt.Errorf("ip -4 rule add (reverse-path guard): %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	// "replace" is idempotent, so no need to inspect the table first.
	if out, err := run("ip", "-4", "route", "replace", "local", "0.0.0.0/0", "dev", "lo", "table", table); err != nil {
		return fmt.Errorf("ip -4 route replace: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if pool != "" {
		if out, err := run("ip", "-4", "route", "replace", "throw", pool, "table", table); err != nil {
			return fmt.Errorf("ip -4 route replace throw %s: %s: %w", pool, strings.TrimSpace(string(out)), err)
		}
	}
	if !HasMarkRule(rules) {
		if out, err := run("ip", "-4", "rule", "add", "pref", strconv.Itoa(RulePriority), "fwmark", mark, "lookup", table); err != nil {
			return fmt.Errorf("ip -4 rule add: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

func removeThrow(pool string, run Runner) {
	_, _ = run("ip", "-4", "route", "del", "throw", pool, "table", strconv.Itoa(RouteTable))
}

func removePolicyRouting(run Runner) {
	table := strconv.Itoa(RouteTable)
	mark := fmt.Sprintf("0x%x", Mark)
	// Duplicate rules are possible when an older iproute2 allowed them; loop
	// until the kernel reports there is nothing left to delete. The
	// delivering rule goes first so no packet is ever routed to table 2601
	// without the guard.
	for i := 0; i < 8; i++ {
		if _, err := run("ip", "-4", "rule", "del", "pref", strconv.Itoa(RulePriority), "fwmark", mark, "lookup", table); err != nil {
			break
		}
	}
	for i := 0; i < 8; i++ {
		if _, err := run("ip", "-4", "rule", "del", "pref", strconv.Itoa(ReversePriority), "iif", "lo", "fwmark", mark, "lookup", "main"); err != nil {
			break
		}
	}
	_, _ = run("ip", "-4", "route", "flush", "table", table)
}

// HasReverseRule reports whether the "ip -4 rule show" output contains the
// reverse-path guard rule (iif lo + fwmark Mark -> main).
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

// HasMarkRule reports whether the "ip -4 rule show" output already contains
// the fwmark -> RouteTable rule. iproute2 prints the mark in hex ("fwmark
// 0x2e01", optionally with a "/mask" suffix) and the table by number unless
// /etc/iproute2/rt_tables names it, so both tokens are parsed leniently.
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
	tok = strings.ToLower(tok)
	if idx := strings.IndexByte(tok, '/'); idx >= 0 {
		tok = tok[:idx]
	}
	var v uint64
	var err error
	if strings.HasPrefix(tok, "0x") {
		v, err = strconv.ParseUint(tok[2:], 16, 32)
	} else {
		v, err = strconv.ParseUint(tok, 10, 32)
	}
	return err == nil && v == Mark
}
