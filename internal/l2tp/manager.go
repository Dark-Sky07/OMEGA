package l2tp

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

type managed struct {
	proc        *Process
	tag         string
	fingerprint string
	lastErrAt   time.Time
	last        map[string]interfaceCounter
	online      map[string]bool
}

// Manager owns the one global L2TP/IPsec process group. The service layer also
// enforces the singleton in the database; keeping the guard here prevents a
// malformed reconcile input from starting two listeners during a race.
type Manager struct {
	mu    sync.Mutex
	proc  *managed
	id    int
	ready bool

	// manualStop prevents the periodic reconcile job from immediately starting
	// the daemon after an explicit dashboard stop. It is intentionally reset by
	// a full panel process restart.
	manualStop bool
	lastError  string
}

// RuntimeStatus is the secret-free L2TP/IPsec snapshot exposed by the panel.
type RuntimeStatus struct {
	Running       bool
	InboundCount  int
	OnlineClients int
	ManualStop    bool
	Error         string
}

var (
	managerOnce sync.Once
	manager     *Manager
)

func GetManager() *Manager {
	managerOnce.Do(func() { manager = &Manager{} })
	return manager
}

func cleanupOrphanData(exceptID int) {
	entries, err := os.ReadDir(l2tpRoot())
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := strconv.Atoi(entry.Name())
		if err != nil || id <= 0 || id == exceptID {
			continue
		}
		stopOrphan(id)
		GetNetworkManager().Remove(id)
		removeStrongSwanRuntime(id)
		_ = os.RemoveAll(dataDirForID(id))
	}
}

// Ensure brings the daemon and firewall toward one desired instance.
func (m *Manager) Ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(inst)
}

func (m *Manager) ensureLocked(inst Instance) error {
	if err := ValidateSettings(inst); err != nil {
		return err
	}
	fp := inst.fingerprint()
	if m.proc != nil && m.id == inst.Id && m.proc.fingerprint == fp && m.proc.proc.IsRunning() {
		if err := reconcileSystemChapSecrets(inst); err != nil {
			return err
		}
		m.proc.tag = inst.Tag
		if m.proc.last == nil {
			m.proc.last = make(map[string]interfaceCounter)
		}
		if m.proc.online == nil {
			m.proc.online = make(map[string]bool)
		}
		if err := GetNetworkManager().Apply(inst); err != nil {
			return err
		}
		return nil
	}
	if m.proc != nil {
		_ = m.proc.proc.Stop()
		GetNetworkManager().Remove(m.id)
		removeStrongSwanRuntime(m.id)
		_ = clearSystemChapSecrets()
		m.proc = nil
		m.id = 0
	}
	// The process group and firewall manager are in-memory. Reclaim any
	// previous group for this id before writing a fresh config after restart.
	stopOrphan(inst.Id)
	if m.proc == nil {
		if err := clearSystemChapSecrets(); err != nil {
			return err
		}
	}
	GetNetworkManager().Remove(inst.Id)
	if err := writeConfig(inst); err != nil {
		return err
	}
	if err := GetNetworkManager().Apply(inst); err != nil {
		_ = clearSystemChapSecrets()
		return err
	}
	proc := newProcess(inst.Id)
	if err := proc.Start(); err != nil {
		GetNetworkManager().Remove(inst.Id)
		_ = clearSystemChapSecrets()
		m.proc = &managed{proc: proc, tag: inst.Tag, fingerprint: fp, lastErrAt: time.Now(), last: map[string]interfaceCounter{}, online: map[string]bool{}}
		m.id = inst.Id
		return err
	}
	m.proc = &managed{proc: proc, tag: inst.Tag, fingerprint: fp, last: map[string]interfaceCounter{}, online: map[string]bool{}}
	m.id = inst.Id
	m.ready = true
	logger.Infof("l2tp: started strongSwan/xl2tpd for inbound %d on UDP 500, 4500 and 1701", inst.Id)
	return nil
}

// Reconcile keeps the daemon group aligned with the enabled local inbound.
// More than one desired instance is a programming/data error; only the first
// is considered so the fixed ports can never be double-bound.
func (m *Manager) Reconcile(desired []Instance) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.manualStop {
		if m.proc != nil {
			_ = m.proc.proc.Stop()
			GetNetworkManager().Remove(m.id)
			removeStrongSwanRuntime(m.id)
			_ = os.RemoveAll(dataDirForID(m.id))
			m.proc = nil
			m.id = 0
			m.ready = false
		}
		cleanupOrphanData(0)
		GetNetworkManager().RemoveAll()
		return
	}
	if len(desired) == 0 {
		m.lastError = ""
		if m.proc != nil {
			_ = m.proc.proc.Stop()
			GetNetworkManager().Remove(m.id)
			removeStrongSwanRuntime(m.id)
			_ = os.RemoveAll(dataDirForID(m.id))
			m.proc = nil
			m.id = 0
			m.ready = false
		}
		cleanupOrphanData(0)
		if err := clearSystemChapSecrets(); err != nil {
			logger.Warningf("l2tp: failed to clear system PPP chap-secrets: %v", err)
		}
		return
	}
	if len(desired) > 1 {
		logger.Warning("l2tp: more than one desired inbound was supplied; only the first will be reconciled")
	}
	inst := desired[0]
	err := m.ensureLocked(inst)
	cleanupOrphanData(inst.Id)
	if err != nil {
		m.lastError = err.Error()
		if m.proc != nil && time.Since(m.proc.lastErrAt) < time.Minute {
			return
		}
		if m.proc != nil {
			m.proc.lastErrAt = time.Now()
		}
		logger.Warningf("l2tp: reconcile failed for inbound %d: %v", inst.Id, err)
	} else {
		m.lastError = ""
	}
}

// StopManually stops the L2TP/IPsec daemon group and holds reconciliation
// until Resume is called by the dashboard.
func (m *Manager) StopManually() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manualStop = true
	m.lastError = ""
	if m.proc != nil {
		_ = m.proc.proc.Stop()
		GetNetworkManager().Remove(m.id)
		removeStrongSwanRuntime(m.id)
		_ = os.RemoveAll(dataDirForID(m.id))
		m.proc = nil
		m.id = 0
		m.ready = false
	}
	cleanupOrphanData(0)
	GetNetworkManager().RemoveAll()
	_ = clearSystemChapSecrets()
}

// Resume clears an explicit dashboard stop request. The next reconcile tick
// starts the enabled L2TP inbound again.
func (m *Manager) Resume() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manualStop = false
	m.lastError = ""
}

// Restart stops the current daemon group while leaving reconciliation enabled.
func (m *Manager) Restart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manualStop = false
	m.lastError = ""
	if m.proc != nil {
		_ = m.proc.proc.Stop()
		GetNetworkManager().Remove(m.id)
		removeStrongSwanRuntime(m.id)
		_ = os.RemoveAll(dataDirForID(m.id))
		m.proc = nil
		m.id = 0
		m.ready = false
	}
	cleanupOrphanData(0)
	GetNetworkManager().RemoveAll()
	_ = clearSystemChapSecrets()
}

// Status returns the secret-free state used by the dashboard.
func (m *Manager) Status() RuntimeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := RuntimeStatus{ManualStop: m.manualStop, Error: m.lastError}
	if m.proc != nil && m.proc.proc != nil {
		status.Running = m.proc.proc.IsRunning()
		if status.Running {
			status.InboundCount = 1
			status.OnlineClients = len(m.proc.online)
		}
	}
	return status
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc != nil && (id == 0 || m.id == id) {
		_ = m.proc.proc.Stop()
		GetNetworkManager().Remove(m.id)
		removeStrongSwanRuntime(m.id)
		_ = os.RemoveAll(dataDirForID(m.id))
		m.proc = nil
		m.id = 0
		m.ready = false
	}
	if id == 0 {
		cleanupOrphanData(0)
		if err := clearSystemChapSecrets(); err != nil {
			logger.Warningf("l2tp: failed to clear system PPP chap-secrets: %v", err)
		}
		return
	}
	if m.proc == nil || m.id == id {
		stopOrphan(id)
		GetNetworkManager().Remove(id)
		removeStrongSwanRuntime(id)
		_ = os.RemoveAll(dataDirForID(id))
	}
	if m.proc == nil {
		if err := clearSystemChapSecrets(); err != nil {
			logger.Warningf("l2tp: failed to clear system PPP chap-secrets: %v", err)
		}
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	m.manualStop = false
	m.lastError = ""
	m.mu.Unlock()
	m.Remove(0)
	GetNetworkManager().RemoveAll()
}

// CollectTraffic reads pppd session markers and Linux interface counters. pppd
// invokes the managed ip-up/ip-down hooks with PEERNAME and the interface name
// in $1/$IFNAME (some distro wrappers also export PPP_IFACE), which gives us a
// stable email-to-interface mapping without inventing a profile or requiring a
// RADIUS accounting server.
func (m *Manager) CollectTraffic() (inbounds []InboundTraffic, clients []ClientTrafficDelta) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proc == nil || !m.proc.proc.IsRunning() {
		if m.proc != nil {
			m.proc.online = map[string]bool{}
		}
		return nil, nil
	}
	if m.proc.last == nil {
		m.proc.last = make(map[string]interfaceCounter)
	}
	if m.proc.online == nil {
		m.proc.online = make(map[string]bool)
	}
	current := readSessions(m.id)
	newOnline := make(map[string]bool, len(current))
	deltaByEmail := make(map[string]ClientTrafficDelta)
	var totalUp, totalDown int64
	for key, counter := range current {
		separator := strings.IndexByte(key, 0)
		if separator < 0 {
			continue
		}
		email := key[separator+1:]
		newOnline[email] = true
		previous := m.proc.last[key]
		up, down := counter.rx, counter.tx
		if counter.rx >= previous.rx {
			up = counter.rx - previous.rx
		}
		if counter.tx >= previous.tx {
			down = counter.tx - previous.tx
		}
		addDelta(deltaByEmail, m.id, email, up, down)
		totalUp += up
		totalDown += down
		m.proc.last[key] = counter
	}
	for key := range m.proc.last {
		if _, ok := current[key]; !ok {
			delete(m.proc.last, key)
		}
	}
	m.proc.online = newOnline
	if totalUp > 0 || totalDown > 0 {
		inbounds = append(inbounds, InboundTraffic{InboundId: m.id, Tag: m.proc.tag, Up: totalUp, Down: totalDown})
	}
	return inbounds, sortedClientDeltas(deltaByEmail)
}

// OnlineEmails returns the currently connected L2TP usernames, deduplicated
// when a panel client has more than one PPP session.
func (m *Manager) OnlineEmails() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]bool)
	for email := range m.procOnline() {
		seen[email] = true
	}
	out := make([]string, 0, len(seen))
	for email := range seen {
		out = append(out, email)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) procOnline() map[string]bool {
	if m.proc == nil {
		return nil
	}
	return m.proc.online
}

func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc != nil && m.proc.proc.IsRunning()
}
