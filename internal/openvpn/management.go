package openvpn

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// clientCounters are the cumulative byte counters the management interface
// reports for one connected client: Rx is what the server received from the
// client (the client's upload), Tx is what the server sent to the client
// (the client's download).
type clientCounters struct {
	Rx int64
	Tx int64
}

// clientList queries the daemon's management interface (CLIENT_LIST) and
// returns the current connected clients keyed by common name, with their
// cumulative byte counters. A timeout or parse failure returns an error; an
// idle (no clients) daemon yields an empty map, not an error.
func clientList(mgmtPort int) (map[string]clientCounters, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", mgmtPort), 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if _, err := conn.Write([]byte("CLIENT_LIST\n")); err != nil {
		return nil, err
	}

	// Monitor mode: the connection is a stream; asynchronous EVENT lines may
	// interleave with our reply. Read until the <END that closes the
	// CLIENT_LIST block.
	out := make(map[string]clientCounters)
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	seenStart := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !seenStart {
			if strings.HasPrefix(line, "<CLIENT_LIST") {
				seenStart = true
			}
			continue
		}
		if line == "<END" {
			break
		}
		if !strings.HasPrefix(line, "<CLIENT_LIST") {
			continue
		}
		fields := strings.Fields(line)
		// <CLIENT_LIST <id> <cn> <real-addr> <virt-addr> <bytes-rx> <bytes-tx> <connected-since>
		if len(fields) < 8 {
			continue
		}
		cn := fields[2]
		rx, errRx := strconv.ParseInt(fields[len(fields)-3], 10, 64)
		tx, errTx := strconv.ParseInt(fields[len(fields)-2], 10, 64)
		if errRx != nil || errTx != nil {
			continue
		}
		out[cn] = clientCounters{Rx: rx, Tx: tx}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, net.ErrClosed) {
			return out, nil
		}
		if !seenStart {
			return nil, err
		}
	}
	return out, nil
}
