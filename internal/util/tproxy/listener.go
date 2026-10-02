package tproxy

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// ListenerBound reports whether some process in this network namespace
// currently has both a listening TCP socket and a bound UDP socket on
// ListenIP:port — i.e. whether the relay listener Xray was asked to open is
// really there. The network managers only install divert rules while this
// holds: a TPROXY rule pointing at a port nobody listens on makes the kernel
// drop every diverted packet, so the check closes the gap between "the
// inbound is in the config" and "the socket exists" (Xray still starting
// after a restart, a listener that failed to bind) with the same fail-open
// behaviour as everything else here. Always false where /proc/net is not
// available.
func ListenerBound(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	return procHasSocket("/proc/net/tcp", port, "0A") && procHasSocket("/proc/net/udp", port, "")
}

// procSocketAddrs returns the local_address tokens /proc/net/{tcp,udp} print
// for ListenIP:port. The kernel prints the IPv4 address as a native-endian
// 32-bit word, so both byte orders are accepted; a socket bound to the
// wildcard address on that port would also receive the diverted packets.
func procSocketAddrs(port int) []string {
	return []string{
		fmt.Sprintf("0100007F:%04X", port), // 127.0.0.1, little-endian hosts
		fmt.Sprintf("7F000001:%04X", port), // 127.0.0.1, big-endian hosts
		fmt.Sprintf("00000000:%04X", port), // 0.0.0.0
	}
}

func procHasSocket(path string, port int, wantState string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return scanProcSockets(f, procSocketAddrs(port), wantState)
}

// scanProcSockets is the parser behind procHasSocket, split out for tests:
// each data line is "sl local_address rem_address st ..." and only the
// local address (and, for TCP, the LISTEN state 0A) matters.
func scanProcSockets(r io.Reader, addrs []string, wantState string) bool {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	first := true
	for scanner.Scan() {
		if first {
			first = false // header
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		if wantState != "" && !strings.EqualFold(fields[3], wantState) {
			continue
		}
		for _, addr := range addrs {
			if strings.EqualFold(fields[1], addr) {
				return true
			}
		}
	}
	return false
}
