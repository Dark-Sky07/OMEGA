package l2tp

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// InboundTraffic is a byte delta for the L2TP inbound.
type InboundTraffic struct {
	InboundId int
	Tag       string
	Up        int64
	Down      int64
}

// ClientTrafficDelta attributes a byte delta to an existing panel client by
// email. The panel's normal traffic writer persists these deltas alongside
// Xray and OpenVPN traffic.
type ClientTrafficDelta struct {
	InboundId int
	Email     string
	Up        int64
	Down      int64
}

type interfaceCounter struct {
	rx int64
	tx int64
}

// l2tpSysClassNetRoot is a variable so filesystem accounting tests can use a
// temporary interface tree. Production always points at Linux sysfs.
var l2tpSysClassNetRoot = "/sys/class/net"

func sessionDirForID(id int) string {
	return filepath.Join(dataDirForID(id), "sessions")
}

func validInterfaceName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

const maxInterfaceCounter = int64(^uint64(0) >> 1)

func readCounter(path string) (int64, bool) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, parseErr := strconv.ParseUint(strings.TrimSpace(string(contents)), 10, 64)
	if parseErr != nil {
		return 0, false
	}
	// Linux exposes unsigned 64-bit counters while the panel traffic model is
	// signed. Saturating at MaxInt64 is safer than wrapping into a negative
	// value and lets the next lower counter be treated as a reset.
	if value > uint64(maxInterfaceCounter) {
		return maxInterfaceCounter, true
	}
	return int64(value), true
}

func readInterfaceCounter(iface string) (interfaceCounter, bool) {
	base := filepath.Join(l2tpSysClassNetRoot, iface, "statistics")
	rx, rxOK := readCounter(filepath.Join(base, "rx_bytes"))
	tx, txOK := readCounter(filepath.Join(base, "tx_bytes"))
	return interfaceCounter{rx: rx, tx: tx}, rxOK && txOK
}

func readSessions(id int) (map[string]interfaceCounter, bool) {
	entries, err := os.ReadDir(sessionDirForID(id))
	if err != nil {
		if os.IsNotExist(err) {
			// No marker directory is the normal no-active-session state.
			return map[string]interfaceCounter{}, true
		}
		return nil, false
	}
	out := make(map[string]interfaceCounter, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !validInterfaceName(entry.Name()) {
			continue
		}
		path := filepath.Join(sessionDirForID(id), entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		line, readErr := bufio.NewReader(file).ReadString('\n')
		_ = file.Close()
		if readErr != nil && len(line) == 0 {
			continue
		}
		email := strings.TrimSpace(line)
		if email == "" || strings.ContainsAny(email, "\r\n") {
			continue
		}
		// A stale ip-up file must not make an offline client appear online.
		if _, err := os.Stat(filepath.Join(l2tpSysClassNetRoot, entry.Name())); err != nil {
			continue
		}
		counter, ok := readInterfaceCounter(entry.Name())
		if !ok {
			// Do not turn a temporarily unreadable counter into a zero delta;
			// that would discard the baseline and double-count the next poll.
			return nil, false
		}
		out[entry.Name()+"\x00"+email] = counter
	}
	return out, true
}

func saturatingCounterAdd(current, delta int64) int64 {
	if delta <= 0 {
		return current
	}
	if current >= maxInterfaceCounter-delta {
		return maxInterfaceCounter
	}
	return current + delta
}

func addDelta(dst map[string]ClientTrafficDelta, id int, email string, up, down int64) {
	if up == 0 && down == 0 {
		return
	}
	item := dst[email]
	item.InboundId = id
	item.Email = email
	item.Up = saturatingCounterAdd(item.Up, up)
	item.Down = saturatingCounterAdd(item.Down, down)
	dst[email] = item
}

func sortedClientDeltas(values map[string]ClientTrafficDelta) []ClientTrafficDelta {
	out := make([]ClientTrafficDelta, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}
