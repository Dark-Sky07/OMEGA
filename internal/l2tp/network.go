package l2tp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/tproxy"
)

// NetworkManager owns the firewall rules required by the L2TP pool. Rules are
// inserted with iptables -C/-I/-D so repeated reconcile rounds are idempotent.
// IPv4 forwarding is enabled at runtime and is intentionally not disabled on
// teardown: another service may also depend on forwarding.
type NetworkManager struct {
	mu         sync.Mutex
	rules      map[int]networkState
	sysctlPath string
	runner     commandRunner
	lookPath   func(file string) (string, error)
	// relayWarned remembers the last relay failure logged per inbound so a
	// host without TPROXY support does not repeat the warning every tick.
	relayWarned map[int]string
}

// networkState is persisted as network.json below the inbound directory so a
// later panel process can remove exactly the rules an earlier one installed.
// Fields must stay exported for encoding/json to round-trip them.
type networkState struct {
	PoolCIDR        string `json:"poolCIDR"`
	InterfaceName   string `json:"interfaceName"`
	FixedInputPorts []int  `json:"fixedInputPorts"`
	// XrayRelayPort is the loopback TPROXY listener the pool's TCP/UDP
	// traffic is currently diverted to (see internal/util/tproxy); 0 means
	// the pool takes the direct FORWARD/MASQUERADE path.
	XrayRelayPort int `json:"xrayRelayPort,omitempty"`
}

// clientInterfaces matches every PPP session interface pppd creates for the
// L2TP clients; the pool source filter keeps unrelated PPP links out.
const clientInterfaces = "ppp+"

func networkStatePath(id int) string {
	return filepath.Join(dataDirForID(id), "network.json")
}

func loadNetworkState(id int) (networkState, bool) {
	data, err := os.ReadFile(networkStatePath(id))
	if err != nil {
		return networkState{}, false
	}
	var state networkState
	if err := json.Unmarshal(data, &state); err != nil || state.PoolCIDR == "" {
		return networkState{}, false
	}
	return state, true
}

func saveNetworkState(id int, state networkState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(networkStatePath(id), data, 0o600)
}

type commandRunner func(name string, args ...string) ([]byte, error)

var (
	networkOnce sync.Once
	networkMgr  *NetworkManager
)

func GetNetworkManager() *NetworkManager {
	networkOnce.Do(func() {
		networkMgr = &NetworkManager{
			rules:      make(map[int]networkState),
			sysctlPath: "/proc/sys/net/ipv4/ip_forward",
			runner: func(name string, args ...string) ([]byte, error) {
				return exec.Command(name, args...).CombinedOutput()
			},
			lookPath:    exec.LookPath,
			relayWarned: make(map[int]string),
		}
	})
	return networkMgr
}

func (m *NetworkManager) available() (string, error) {
	lookPath := m.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	for _, candidate := range []string{"iptables", "iptables-legacy", "iptables-nft"} {
		if path, err := lookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("iptables binary not found")
}

func (m *NetworkManager) run(name string, args ...string) error {
	output, err := m.runner(name, args...)
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s: %s: %w", name, strings.Join(args, " "), text, err)
	}
	return nil
}

func iptablesArgs(operation, table string, rule []string) []string {
	// Keep the capacity based only on the caller-provided slice. Adding the
	// fixed table/operation arguments to len(rule) can overflow an allocation
	// size before append gets a chance to grow it safely.
	args := make([]string, 0, len(rule))
	if table != "" {
		args = append(args, "-t", table)
	}
	args = append(args, operation)
	return append(args, rule...)
}

func (m *NetworkManager) ensureRule(iptables string, table string, rule []string) (bool, error) {
	if err := m.run(iptables, iptablesArgs("-C", table, rule)...); err == nil {
		return false, nil
	}
	if err := m.run(iptables, iptablesArgs("-I", table, rule)...); err != nil {
		return false, err
	}
	return true, nil
}

func (m *NetworkManager) removeRule(iptables string, table string, rule []string) {
	if err := m.run(iptables, iptablesArgs("-D", table, rule)...); err != nil {
		logger.Debug("l2tp: remove firewall rule skipped:", err)
	}
}

func fixedInputRule(port int) []string {
	return []string{"-p", "udp", "--dport", fmt.Sprint(port), "-j", "ACCEPT"}
}

// ESP (IP protocol 50) is used when the Windows client is not behind NAT.
// NAT-T moves the same traffic to UDP 4500, but allowing only UDP would make
// direct public-to-public L2TP/IPsec negotiation fail after IKE succeeds.
func espInputRule() []string {
	return []string{"-p", "esp", "-j", "ACCEPT"}
}

func (m *NetworkManager) enableFixedPorts(iptables string) ([]int, bool, error) {
	inserted := make([]int, 0, len(FixedPorts))
	for _, port := range FixedPorts {
		wasInserted, err := m.ensureRule(iptables, "", append([]string{"INPUT"}, fixedInputRule(port)...))
		if err != nil {
			return inserted, false, fmt.Errorf("open UDP %d: %w", port, err)
		}
		if wasInserted {
			inserted = append(inserted, port)
		}
	}
	espInserted, err := m.ensureRule(iptables, "", append([]string{"INPUT"}, espInputRule()...))
	if err != nil {
		m.removeFixedPorts(iptables, inserted)
		return inserted, false, fmt.Errorf("open ESP protocol 50: %w", err)
	}
	return inserted, espInserted, nil
}

func (m *NetworkManager) removeFixedPorts(iptables string, ports []int) {
	for _, port := range ports {
		m.removeRule(iptables, "", append([]string{"INPUT"}, fixedInputRule(port)...))
	}
}

func (m *NetworkManager) removeESP(iptables string) {
	m.removeRule(iptables, "", append([]string{"INPUT"}, espInputRule()...))
}

func mergePorts(existing, inserted []int) []int {
	seen := make(map[int]struct{}, len(existing)+len(inserted))
	out := make([]int, 0, len(existing)+len(inserted))
	for _, port := range append(existing, inserted...) {
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	return out
}

func (m *NetworkManager) enableForwarding() error {
	if err := m.run("sysctl", "-w", "net.ipv4.ip_forward=1"); err == nil {
		return nil
	}
	// Minimal containers may not ship sysctl. Writing procfs directly keeps
	// Linux-host and Docker behavior equivalent when the proc mount is writable.
	return m.run("sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
}

// Apply enables forwarding and installs FORWARD/MASQUERADE rules for the pool.
// It is safe to call once per reconcile tick.
func (m *NetworkManager) Apply(inst Instance) error {
	if err := ValidateSettings(inst); err != nil {
		return err
	}
	iptables, err := m.available()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.enableForwarding(); err != nil {
		return fmt.Errorf("l2tp: enable IPv4 forwarding: %w", err)
	}
	previous, hadPrevious := m.rules[inst.Id]
	if !hadPrevious {
		previous, hadPrevious = loadNetworkState(inst.Id)
	}
	previousFixedPorts := []int(nil)
	if hadPrevious {
		previousFixedPorts = append(previousFixedPorts, previous.FixedInputPorts...)
	}
	newFixedPorts, espInserted, err := m.enableFixedPorts(iptables)
	if err != nil {
		return fmt.Errorf("l2tp: open fixed L2TP/IPsec listeners: %w", err)
	}
	fixedPorts := mergePorts(previousFixedPorts, newFixedPorts)
	pool := inst.poolCIDRFor()
	iface := strings.TrimSpace(inst.OutboundInterface)
	forwardOut := []string{"FORWARD", "-s", pool}
	if iface != "" {
		forwardOut = append(forwardOut, "-o", iface)
	}
	forwardOut = append(forwardOut, "-j", "ACCEPT")
	outInserted, err := m.ensureRule(iptables, "", forwardOut)
	if err != nil {
		m.removeFixedPorts(iptables, newFixedPorts)
		if espInserted {
			m.removeESP(iptables)
		}
		return fmt.Errorf("l2tp: install outbound FORWARD rule: %w", err)
	}

	forwardBack := []string{"FORWARD", "-d", pool}
	if iface != "" {
		forwardBack = append(forwardBack, "-i", iface)
	}
	forwardBack = append(forwardBack, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	backInserted, err := m.ensureRule(iptables, "", forwardBack)
	if err != nil {
		if outInserted {
			m.removeRule(iptables, "", forwardOut)
		}
		m.removeFixedPorts(iptables, newFixedPorts)
		if espInserted {
			m.removeESP(iptables)
		}
		return fmt.Errorf("l2tp: install return FORWARD rule: %w", err)
	}

	masquerade := []string{"POSTROUTING", "-s", pool}
	if iface != "" {
		masquerade = append(masquerade, "-o", iface)
	}
	masquerade = append(masquerade, "-j", "MASQUERADE")
	masqInserted, err := m.ensureRule(iptables, "nat", masquerade)
	if err != nil {
		if backInserted {
			m.removeRule(iptables, "", forwardBack)
		}
		if outInserted {
			m.removeRule(iptables, "", forwardOut)
		}
		m.removeFixedPorts(iptables, newFixedPorts)
		if espInserted {
			m.removeESP(iptables)
		}
		return fmt.Errorf("l2tp: install MASQUERADE rule: %w", err)
	}
	state := networkState{PoolCIDR: pool, InterfaceName: iface, FixedInputPorts: fixedPorts}
	if inst.XrayRelayPort > 0 && inst.XrayRelayPort <= 65535 {
		state.XrayRelayPort = inst.XrayRelayPort
	}
	// The relay is layered on top of the direct rules and is best effort:
	// whatever it reports as active is what gets persisted for cleanup.
	state.XrayRelayPort = m.applyRelay(iptables, inst.Id, previous, hadPrevious, state)
	if err := saveNetworkState(inst.Id, state); err != nil {
		// The firewall is only considered managed once its rollback metadata is
		// durable; otherwise a panel restart could leave unremovable rules.
		if state.XrayRelayPort > 0 {
			m.removeRelayRules(iptables, state)
			tproxy.Release(relayOwner(inst.Id), tproxy.Runner(m.runner))
		}
		if masqInserted {
			m.removeRule(iptables, "nat", masquerade)
		}
		if backInserted {
			m.removeRule(iptables, "", forwardBack)
		}
		if outInserted {
			m.removeRule(iptables, "", forwardOut)
		}
		m.removeFixedPorts(iptables, newFixedPorts)
		if espInserted {
			m.removeESP(iptables)
		}
		return fmt.Errorf("l2tp: persist firewall state: %w", err)
	}
	m.rules[inst.Id] = state
	return nil
}

// Remove deletes rules installed for an inbound. It does not turn forwarding
// off because forwarding is a host-wide setting and may be used by Xray,
// OpenVPN, WireGuard, containers, or another administrator-managed service.
func (m *NetworkManager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.rules[id]
	if !ok {
		state, ok = loadNetworkState(id)
	}
	iptables, err := m.available()
	if err != nil {
		delete(m.rules, id)
		_ = os.Remove(networkStatePath(id))
		return
	}
	ports := state.FixedInputPorts
	if len(ports) == 0 {
		// UDP 500/4500/1701 are reserved by the global L2TP service. Remove
		// exact managed rules even when this process has no durable state from
		// an older release or an unclean panel restart.
		ports = append([]int(nil), FixedPorts[:]...)
	}
	m.removeFixedPorts(iptables, ports)
	m.removeESP(iptables)
	if ok && state.XrayRelayPort > 0 {
		m.removeRelayRules(iptables, state)
		tproxy.Release(relayOwner(id), tproxy.Runner(m.runner))
	}
	if ok && state.PoolCIDR != "" {
		pool := state.PoolCIDR
		iface := state.InterfaceName
		forwardOut := []string{"FORWARD", "-s", pool}
		if iface != "" {
			forwardOut = append(forwardOut, "-o", iface)
		}
		forwardOut = append(forwardOut, "-j", "ACCEPT")
		m.removeRule(iptables, "", forwardOut)

		forwardBack := []string{"FORWARD", "-d", pool}
		if iface != "" {
			forwardBack = append(forwardBack, "-i", iface)
		}
		forwardBack = append(forwardBack, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT")
		m.removeRule(iptables, "", forwardBack)

		masquerade := []string{"POSTROUTING", "-s", pool}
		if iface != "" {
			masquerade = append(masquerade, "-o", iface)
		}
		masquerade = append(masquerade, "-j", "MASQUERADE")
		m.removeRule(iptables, "nat", masquerade)
	}
	delete(m.rules, id)
	delete(m.relayWarned, id)
	_ = os.Remove(networkStatePath(id))
}

// relayRulesForState returns the TPROXY divert rules for a state whose
// XrayRelayPort is set, or nil when the pool is on the direct path. Sessions
// arrive on pppd's per-client interfaces, so the interface match is the ppp+
// wildcard and the pool filter does the actual selection.
func relayRulesForState(state networkState) [][]string {
	if state.XrayRelayPort <= 0 {
		return nil
	}
	return tproxy.DivertRules(clientInterfaces, state.PoolCIDR, state.XrayRelayPort)
}

func relayOwner(id int) string {
	return "l2tp:" + strconv.Itoa(id)
}

func (m *NetworkManager) removeRelayRules(iptables string, state networkState) {
	for _, rule := range relayRulesForState(state) {
		m.removeRule(iptables, "mangle", rule)
	}
}

func (m *NetworkManager) warnRelay(id int, msg string) {
	if m.relayWarned == nil {
		m.relayWarned = make(map[int]string)
	}
	if m.relayWarned[id] == msg {
		return
	}
	m.relayWarned[id] = msg
	logger.Warningf("l2tp: inbound %d: cannot divert traffic through Xray, keeping the direct path: %s", id, msg)
}

// applyRelay converges the TPROXY divert rules of the pool to the relay port
// carried by wanted and returns the port that is actually active afterwards
// (0 = direct path). It never fails the caller: a kernel without TPROXY
// support, or policy routing that cannot be installed, just leaves the pool
// on the direct path with a warning, and the next reconcile round tries
// again. Must be called with m.mu held.
func (m *NetworkManager) applyRelay(iptables string, id int, previous networkState, hadPrevious bool, wanted networkState) int {
	prevPort := 0
	if hadPrevious {
		prevPort = previous.XrayRelayPort
	}
	changed := prevPort != wanted.XrayRelayPort || previous.PoolCIDR != wanted.PoolCIDR
	if prevPort > 0 && changed {
		m.removeRelayRules(iptables, previous)
	}
	if wanted.XrayRelayPort <= 0 {
		if prevPort > 0 {
			tproxy.Release(relayOwner(id), tproxy.Runner(m.runner))
			logger.Infof("l2tp: inbound %d: pool %s traffic is back on the direct path", id, wanted.PoolCIDR)
		}
		return 0
	}

	// The routing entries must exist before the first divert rule does,
	// otherwise a diverted packet would be forwarded instead of delivered.
	if err := tproxy.Acquire(relayOwner(id), tproxy.Runner(m.runner)); err != nil {
		m.warnRelay(id, err.Error())
		return 0
	}
	rules := relayRulesForState(wanted)
	inserted := make([][]string, 0, len(rules))
	for _, rule := range rules {
		wasInserted, err := m.ensureRule(iptables, "mangle", rule)
		if err != nil {
			for i := len(inserted) - 1; i >= 0; i-- {
				m.removeRule(iptables, "mangle", inserted[i])
			}
			tproxy.Release(relayOwner(id), tproxy.Runner(m.runner))
			m.warnRelay(id, err.Error())
			return 0
		}
		if wasInserted {
			inserted = append(inserted, rule)
		}
	}
	delete(m.relayWarned, id)
	if prevPort != wanted.XrayRelayPort {
		logger.Infof("l2tp: inbound %d: diverting pool %s TCP/UDP traffic through Xray (%s:%d)", id, wanted.PoolCIDR, tproxy.ListenIP, wanted.XrayRelayPort)
	}
	return wanted.XrayRelayPort
}

func (m *NetworkManager) RemoveAll() {
	m.mu.Lock()
	ids := make([]int, 0, len(m.rules))
	for id := range m.rules {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Remove(id)
	}
}

// ValidatePool is kept public for API/service tests and callers that need to
// validate a pool before creating an inbound.
func ValidatePool(cidr string) error {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil || ip.To4() == nil || network.IP.To4() == nil {
		return fmt.Errorf("poolCIDR must be an IPv4 CIDR")
	}
	return nil
}
