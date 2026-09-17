package openvpn

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// InboundTraffic is the per-inbound byte delta for one polling round.
type InboundTraffic struct {
	InboundId int
	Tag       string
	Up        int64
	Down      int64
}

// ClientTrafficDelta is the per-client byte delta for one polling round.
// Email carries the client's CN (its email address), which the traffic
// pipeline uses to attribute usage exactly like xray-reported clients.
type ClientTrafficDelta struct {
	InboundId int
	Email     string
	Up        int64
	Down      int64
}

type managed struct {
	proc        *Process
	tag         string
	fingerprint string
	mgmtPort    int
	clients     []string

	// Last observed cumulative counters per client CN; deltas are computed
	// against these each round. Reset handling (daemon restart zeroes the
	// counters) is folded into the delta computation in CollectTraffic.
	lastRx   map[string]int64
	lastTx   map[string]int64
	online   map[string]bool
	haveLast bool

	// Start-failure throttling: a broken daemon (missing binary, busy port)
	// must not spam the log every 10 s tick.
	lastErrAt time.Time
}

// Manager owns the set of running openvpn daemons keyed by inbound id.
type Manager struct {
	mu sync.Mutex
	// procs maps inbound id -> running (or last-started) daemon state.
	procs map[int]*managed
}

var (
	managerOnce sync.Once
	manager     *Manager
)

// Manager returns the process-wide openvpn manager singleton.
func Manager() *Manager {
	managerOnce.Do(func() {
		manager = &Manager{procs: map[int]*managed{}}
	})
	return manager
}

// ensure brings one inbound's daemon to the desired state: starts it when
// missing or crashed, restarts it when the fingerprint (config + client
// certs) changed, and no-ops when it is already running with the same
// fingerprint. Certificates are (re)generated before every (re)start.
func (m *Manager) ensure(inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureLocked(inst)
}

func (m *Manager) ensureLocked(inst Instance) error {
	fp := inst.fingerprint()
	dir := dataDirForID(inst.Id)
	if cur, ok := m.procs[inst.Id]; ok && cur.fingerprint == fp && cur.proc.IsRunning() {
		cur.tag = inst.Tag
		cur.clients = inst.Clients
		// Certs may have been lost on a flaky disk; top them up cheaply.
		if err := ensureServerCert(dir); err != nil {
			logger.Warningf("openvpn: inbound %d: heal server cert failed: %v", inst.Id, err)
		}
		for _, email := range inst.Clients {
			if err := ensureClientCert(dir, email); err != nil {
				logger.Warningf("openvpn: inbound %d: heal client cert for %s failed: %v", inst.Id, email, err)
			}
		}
		return nil
	}
	if cur, ok := m.procs[inst.Id]; ok {
		_ = cur.proc.Stop()
		delete(m.procs, inst.Id)
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := ensureServerCert(dir); err != nil {
		return err
	}
	for _, email := range inst.Clients {
		if err := ensureClientCert(dir, email); err != nil {
			return err
		}
	}
	if err := pruneClientCerts(dir, inst.Clients); err != nil {
		logger.Warningf("openvpn: inbound %d: prune client certs failed: %v", inst.Id, err)
	}

	mgmtPort, err := FreeLocalPort()
	if err != nil {
		return err
	}
	cfgPath := confPathForID(inst.Id)
	if err := writeConfig(inst, mgmtPort); err != nil {
		return err
	}
	proc := newProcess(cfgPath, "inbound "+strconv.Itoa(inst.Id))
	if err := proc.Start(); err != nil {
		m.procs[inst.Id] = &managed{
			proc:        proc,
			tag:         inst.Tag,
			fingerprint: fp,
			clients:     inst.Clients,
			lastErrAt:   time.Now(),
		}
		return err
	}
	m.procs[inst.Id] = &managed{
		proc:        proc,
		tag:         inst.Tag,
		fingerprint: fp,
		mgmtPort:    mgmtPort,
		clients:     inst.Clients,
		lastRx:      map[string]int64{},
		lastTx:      map[string]int64{},
		online:      map[string]bool{},
	}
	logger.Infof("openvpn: started daemon for inbound %d (port %d, %s, %d client(s))",
		inst.Id, inst.Port, inst.protoFor(), len(inst.Clients))
	return nil
}

// Reconcile drives the running set toward the desired instances: it stops
// daemons that are no longer wanted (and removes their data directory) and
// (re)starts the rest. Called periodically by the openvpn job; also the
// only place orphaned daemons from a previous x-ui run get reclaimed, via
// the per-inbound pid files.
func (m *Manager) Reconcile(desired []Instance) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := make(map[int]struct{}, len(desired))
	for _, inst := range desired {
		want[inst.Id] = struct{}{}
	}
	for id, cur := range m.procs {
		if _, ok := want[id]; !ok {
			_ = cur.proc.Stop()
			_ = removeDataDir(id)
			delete(m.procs, id)
			logger.Infof("openvpn: stopped daemon for inbound %d (no longer desired)", id)
		}
	}
	for _, inst := range desired {
		if err := m.ensureLocked(inst); err != nil {
			// Throttled: a persistent failure (no binary, busy port) logs
			// at most once a minute instead of every 10 s tick.
			if cur, ok := m.procs[inst.Id]; ok && time.Since(cur.lastErrAt) < time.Minute {
				continue
			}
			if cur, ok := m.procs[inst.Id]; ok {
				cur.lastErrAt = time.Now()
			}
			logger.Warningf("openvpn: reconcile failed for inbound %d: %v", inst.Id, err)
		}
	}
}

// Remove stops and forgets the daemon for an inbound id, deleting its data
// directory. Used when an inbound is deleted.
func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.procs[id]; ok {
		_ = cur.proc.Stop()
		delete(m.procs, id)
	}
	_ = removeDataDir(id)
}

// StopAll stops every managed daemon. Called on panel shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, cur := range m.procs {
		_ = cur.proc.Stop()
		delete(m.procs, id)
	}
}

func removeDataDir(id int) error {
	return os.RemoveAll(dataDirForID(id))
}

// CollectTraffic polls each running daemon's management interface and
// returns per-inbound and per-client byte deltas since the previous poll.
// Daemons whose management interface is unreachable contribute nothing (and
// drop their clients from the online set).
func (m *Manager) CollectTraffic() (inbounds []InboundTraffic, clients []ClientTrafficDelta) {
	m.mu.Lock()
	type probe struct {
		id       int
		tag      string
		mgmtPort int
		man      *managed
	}
	probes := make([]probe, 0, len(m.procs))
	for id, man := range m.procs {
		if man.proc.IsRunning() && man.mgmtPort > 0 {
			probes = append(probes, probe{id: id, tag: man.tag, mgmtPort: man.mgmtPort, man: man})
		} else {
			// Not running: its online set is stale.
			for cn := range man.online {
				delete(man.online, cn)
			}
		}
	}
	m.mu.Unlock()

	for _, p := range probes {
		list, err := clientList(p.mgmtPort)
		if err != nil {
			for cn := range p.man.online {
				delete(p.man.online, cn)
			}
			continue
		}
		man := p.man
		up, down := int64(0), int64(0)
		newOnline := make(map[string]bool, len(list))
		for cn, c := range list {
			newOnline[cn] = true
			var baseRx, baseTx int64
			if man.haveLast {
				baseRx, baseTx = man.lastRx[cn], man.lastTx[cn]
			}
			// A restarted daemon zeroes its counters; a lower reading than
			// last time means a reset, so count from zero.
			dRx := c.Rx
			if c.Rx < baseRx {
				dRx = c.Rx
			} else {
				dRx = c.Rx - baseRx
			}
			dTx := c.Tx
			if c.Tx < baseTx {
				dTx = c.Tx
			} else {
				dTx = c.Tx - baseTx
			}
			up += dRx
			down += dTx
			if dRx > 0 || dTx > 0 {
				clients = append(clients, ClientTrafficDelta{
					InboundId: p.id,
					Email:     cn,
					Up:        dRx,
					Down:      dTx,
				})
			}
			man.lastRx[cn] = c.Rx
			man.lastTx[cn] = c.Tx
		}
		// Forget counters of clients that disconnected (and prune the maps).
		for cn := range man.lastRx {
			if !newOnline[cn] {
				delete(man.lastRx, cn)
				delete(man.lastTx, cn)
			}
		}
		man.online = newOnline
		man.haveLast = true
		if up > 0 || down > 0 {
			inbounds = append(inbounds, InboundTraffic{InboundId: p.id, Tag: p.tag, Up: up, Down: down})
		}
	}
	return inbounds, clients
}

// OnlineEmails returns the union of connected client CNs across all running
// daemons — the openvpn half of the panel's online-client set. A client
// attached to several openvpn inbounds is listed once.
func (m *Manager) OnlineEmails() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]bool, 4)
	out := make([]string, 0, 4)
	for _, man := range m.procs {
		for cn := range man.online {
			if !seen[cn] {
				seen[cn] = true
				out = append(out, cn)
			}
		}
	}
	return out
}
