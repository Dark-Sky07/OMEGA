package openvpn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// networkState records the firewall rules installed for one OpenVPN inbound.
// It is persisted below the inbound directory so a panel restart can remove
// rules that were installed by an earlier process.
type networkState struct {
	PoolCIDR      string `json:"poolCIDR"`
	InterfaceName string `json:"interfaceName"`
	Protocol      string `json:"protocol"`
	Port          int    `json:"port"`
}

type managedRule struct {
	table string
	rule  []string
}

type networkCommandRunner func(name string, args ...string) ([]byte, error)

type NetworkManager struct {
	mu     sync.Mutex
	rules  map[int]networkState
	runner networkCommandRunner
}

var (
	networkOnce sync.Once
	networkMgr  *NetworkManager
)

func networkStatePath(id int) string {
	return filepath.Join(dataDirForID(id), "network.json")
}

func loadNetworkState(id int) (networkState, bool) {
	data, err := os.ReadFile(networkStatePath(id))
	if err != nil {
		return networkState{}, false
	}
	var state networkState
	if err := json.Unmarshal(data, &state); err != nil || state.PoolCIDR == "" || state.InterfaceName == "" || state.Port <= 0 {
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

// GetNetworkManager returns the process-wide OpenVPN firewall manager.
func GetNetworkManager() *NetworkManager {
	networkOnce.Do(func() {
		networkMgr = &NetworkManager{
			rules: make(map[int]networkState),
			runner: func(name string, args ...string) ([]byte, error) {
				return exec.Command(name, args...).CombinedOutput()
			},
		}
	})
	return networkMgr
}

func (m *NetworkManager) available() (string, error) {
	for _, candidate := range []string{"iptables", "iptables-legacy", "iptables-nft"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("iptables binary not found")
}

func iptablesArgs(operation, table string, rule []string) []string {
	args := make([]string, 0, len(rule)+3)
	if table != "" {
		args = append(args, "-t", table)
	}
	args = append(args, operation)
	return append(args, rule...)
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

func (m *NetworkManager) ensureRule(iptables string, rule managedRule) (bool, error) {
	if err := m.run(iptables, iptablesArgs("-C", rule.table, rule.rule)...); err == nil {
		return false, nil
	}
	if err := m.run(iptables, iptablesArgs("-I", rule.table, rule.rule)...); err != nil {
		return false, err
	}
	return true, nil
}

func (m *NetworkManager) removeRule(iptables string, rule managedRule) {
	if err := m.run(iptables, iptablesArgs("-D", rule.table, rule.rule)...); err != nil {
		logger.Debug("openvpn: remove firewall rule skipped:", err)
	}
}

func (m *NetworkManager) enableForwarding() error {
	if err := m.run("sysctl", "-w", "net.ipv4.ip_forward=1"); err == nil {
		return nil
	}
	return m.run("sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward")
}

func protocolForState(inst Instance) string {
	if inst.protoFor() == "tcp" {
		return "tcp"
	}
	return "udp"
}

func stateForInstance(inst Instance) networkState {
	return networkState{
		PoolCIDR:      inst.serverSubnetCIDR(),
		InterfaceName: devNameForID(inst.Id),
		Protocol:      protocolForState(inst),
		Port:          inst.Port,
	}
}

func sameNetworkState(a, b networkState) bool {
	return a.PoolCIDR == b.PoolCIDR &&
		a.InterfaceName == b.InterfaceName &&
		a.Protocol == b.Protocol &&
		a.Port == b.Port
}

func rulesForState(state networkState) []managedRule {
	return []managedRule{
		{rule: []string{"INPUT", "-p", state.Protocol, "--dport", strconv.Itoa(state.Port), "-j", "ACCEPT"}},
		{rule: []string{"INPUT", "-i", state.InterfaceName, "-s", state.PoolCIDR, "-j", "ACCEPT"}},
		{rule: []string{"FORWARD", "-i", state.InterfaceName, "-s", state.PoolCIDR, "-j", "ACCEPT"}},
		{rule: []string{"FORWARD", "-o", state.InterfaceName, "-d", state.PoolCIDR, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{table: "nat", rule: []string{"POSTROUTING", "-s", state.PoolCIDR, "!", "-d", state.PoolCIDR, "-j", "MASQUERADE"}},
	}
}

func (m *NetworkManager) removeStateRules(iptables string, state networkState) {
	for _, rule := range rulesForState(state) {
		m.removeRule(iptables, rule)
	}
}

// Apply enables forwarding and installs the input, forwarding and
// MASQUERADE rules required for OpenVPN clients to reach the internet. The
// daemon can complete TLS without these rules, which otherwise leaves both
// Windows and Android connected with no working web traffic.
func (m *NetworkManager) Apply(inst Instance) error {
	if inst.Port < 1 || inst.Port > 65535 {
		return fmt.Errorf("openvpn: invalid port %d", inst.Port)
	}
	iptables, err := m.available()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.enableForwarding(); err != nil {
		return fmt.Errorf("openvpn: enable IPv4 forwarding: %w", err)
	}
	previous, hadPrevious := m.rules[inst.Id]
	if !hadPrevious {
		previous, hadPrevious = loadNetworkState(inst.Id)
	}
	state := stateForInstance(inst)
	rules := rulesForState(state)
	inserted := make([]managedRule, 0, len(rules))
	for _, rule := range rules {
		wasInserted, err := m.ensureRule(iptables, rule)
		if err != nil {
			for i := len(inserted) - 1; i >= 0; i-- {
				m.removeRule(iptables, inserted[i])
			}
			return fmt.Errorf("openvpn: install firewall rule: %w", err)
		}
		if wasInserted {
			inserted = append(inserted, rule)
		}
	}
	if err := saveNetworkState(inst.Id, state); err != nil {
		for i := len(inserted) - 1; i >= 0; i-- {
			m.removeRule(iptables, inserted[i])
		}
		return fmt.Errorf("openvpn: persist firewall state: %w", err)
	}
	if hadPrevious && !sameNetworkState(previous, state) {
		m.removeStateRules(iptables, previous)
	}
	m.rules[inst.Id] = state
	return nil
}

// Remove deletes the rules installed for one OpenVPN inbound.
func (m *NetworkManager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.rules[id]
	if !ok {
		state, ok = loadNetworkState(id)
	}
	if !ok {
		delete(m.rules, id)
		return
	}
	iptables, err := m.available()
	if err != nil {
		logger.Debug("openvpn: cannot remove firewall rules:", err)
		return
	}
	m.removeStateRules(iptables, state)
	delete(m.rules, id)
	_ = os.Remove(networkStatePath(id))
}

// RemoveExcept removes persisted firewall state for OpenVPN inbounds that are
// no longer desired. This also cleans rules left by a previous panel process
// when the in-memory manager starts empty after a restart.
func (m *NetworkManager) RemoveExcept(keep map[int]struct{}) {
	ids := make(map[int]struct{})
	m.mu.Lock()
	for id := range m.rules {
		ids[id] = struct{}{}
	}
	m.mu.Unlock()
	entries, err := os.ReadDir(openvpnDir())
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			id, parseErr := strconv.Atoi(entry.Name())
			if parseErr == nil && id > 0 {
				ids[id] = struct{}{}
			}
		}
	}
	for id := range ids {
		if _, wanted := keep[id]; !wanted {
			m.Remove(id)
		}
	}
}

func (m *NetworkManager) RemoveAll() {
	m.RemoveExcept(map[int]struct{}{})
}
