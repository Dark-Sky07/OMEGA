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

func readCounter(path string) int64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	line, err := bufio.NewReader(file).ReadString('\n')
	if err != nil && len(line) == 0 {
		return 0
	}
	value, parseErr := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
	if parseErr != nil || value < 0 {
		return 0
	}
	return value
}

func readInterfaceCounter(iface string) interfaceCounter {
	base := filepath.Join("/sys/class/net", iface, "statistics")
	return interfaceCounter{
		rx: readCounter(filepath.Join(base, "rx_bytes")),
		tx: readCounter(filepath.Join(base, "tx_bytes")),
	}
}

func readSessions(id int) map[string]interfaceCounter {
	entries, err := os.ReadDir(sessionDirForID(id))
	if err != nil {
		return nil
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
		if _, err := os.Stat(filepath.Join("/sys/class/net", entry.Name())); err != nil {
			continue
		}
		out[entry.Name()+"\x00"+email] = readInterfaceCounter(entry.Name())
	}
	return out
}

func addDelta(dst map[string]ClientTrafficDelta, id int, email string, up, down int64) {
	if up == 0 && down == 0 {
		return
	}
	item := dst[email]
	item.InboundId = id
	item.Email = email
	item.Up += up
	item.Down += down
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
