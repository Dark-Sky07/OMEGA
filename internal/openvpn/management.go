package openvpn

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
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

// clientListLayout describes the columns in a CSV CLIENT_LIST response. The
// management interface has emitted more than one status format over the
// lifetime of OpenVPN: some versions use a comma-separated response with a
// HEADER row, while older versions use space-separated rows. Keep the offsets
// relative to the first data column because a HEADER row may start with
// `HEADER,CLIENT_LIST` while data rows start with `CLIENT_LIST`.
type clientListLayout struct {
	name int
	rx   int
	tx   int
}

func normalizedColumn(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func csvFields(line string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(line))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	return reader.Read()
}

// clientListLayoutFromHeader recognises both
//
//	CLIENT_LIST,Common Name,...,Bytes Received,Bytes Sent,...
//
// and
//
//	HEADER,CLIENT_LIST,Common Name,...,Bytes Received,Bytes Sent,...
//
// forms used by OpenVPN status/management replies.
func clientListLayoutFromHeader(fields []string) (clientListLayout, bool) {
	base := 0
	if len(fields) > 0 && normalizedColumn(fields[0]) == "header" {
		if len(fields) < 2 || normalizedColumn(fields[1]) != "client_list" {
			return clientListLayout{}, false
		}
		base = 2
	} else if len(fields) > 0 && normalizedColumn(fields[0]) == "client_list" {
		base = 1
	}

	layout := clientListLayout{name: -1, rx: -1, tx: -1}
	for i := base; i < len(fields); i++ {
		switch normalizedColumn(fields[i]) {
		case "common name", "common_name", "cn":
			layout.name = i - base
		case "bytes received", "bytes_received", "bytes rx", "rx":
			layout.rx = i - base
		case "bytes sent", "bytes_sent", "bytes tx", "tx":
			layout.tx = i - base
		}
	}
	if layout.name < 0 || layout.rx < 0 || layout.tx < 0 {
		return clientListLayout{}, false
	}
	return layout, true
}

func parseCounterPair(fields []string, start int) (clientCounters, bool) {
	// In a status-version-2 row the virtual IPv6 column may be empty. Looking
	// for the first pair of numeric columns after the address fields handles
	// both the version-1 and version-2 layouts without mistaking the later
	// client-id/peer-id fields for traffic counters.
	for i := start; i+1 < len(fields) && i < start+7; i++ {
		rx, errRx := strconv.ParseInt(strings.TrimSpace(fields[i]), 10, 64)
		tx, errTx := strconv.ParseInt(strings.TrimSpace(fields[i+1]), 10, 64)
		if errRx == nil && errTx == nil && rx >= 0 && tx >= 0 {
			return clientCounters{Rx: rx, Tx: tx}, true
		}
	}
	return clientCounters{}, false
}

func parseCSVClientRow(fields []string, layout *clientListLayout) (string, clientCounters, bool) {
	if len(fields) == 0 {
		return "", clientCounters{}, false
	}

	// A header can be prefixed with HEADER,CLIENT_LIST, whereas a data row is
	// normally prefixed with CLIENT_LIST. Convert the relative layout to the
	// current row's base before indexing it.
	base := 0
	if normalizedColumn(fields[0]) == "client_list" {
		base = 1
	} else if normalizedColumn(fields[0]) == "header" {
		return "", clientCounters{}, false
	}
	if layout != nil {
		nameIndex := base + layout.name
		rxIndex := base + layout.rx
		txIndex := base + layout.tx
		if nameIndex >= 0 && rxIndex >= 0 && txIndex >= 0 &&
			nameIndex < len(fields) && rxIndex < len(fields) && txIndex < len(fields) {
			cn := strings.TrimSpace(fields[nameIndex])
			rx, errRx := strconv.ParseInt(strings.TrimSpace(fields[rxIndex]), 10, 64)
			tx, errTx := strconv.ParseInt(strings.TrimSpace(fields[txIndex]), 10, 64)
			if cn != "" && errRx == nil && errTx == nil && rx >= 0 && tx >= 0 {
				return cn, clientCounters{Rx: rx, Tx: tx}, true
			}
		}
		return "", clientCounters{}, false
	}

	// Traditional status output has no CLIENT_LIST prefix and places the
	// counters immediately after Common Name and Real Address. A management
	// response without a header uses the same row shape but may carry the
	// CLIENT_LIST prefix.
	cnIndex := base
	if cnIndex >= len(fields) {
		return "", clientCounters{}, false
	}
	cn := strings.TrimSpace(fields[cnIndex])
	if cn == "" {
		return "", clientCounters{}, false
	}
	start := base + 2
	counters, ok := parseCounterPair(fields, start)
	if !ok {
		return "", clientCounters{}, false
	}
	return cn, counters, true
}

func parseSpaceClientRow(line string) (string, clientCounters, bool) {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return "", clientCounters{}, false
	}

	first := strings.TrimSpace(fields[0])
	if first != "CLIENT_LIST" && first != "<CLIENT_LIST" {
		return "", clientCounters{}, false
	}
	if strings.EqualFold(fields[1], "header") || strings.EqualFold(fields[1], "version") {
		return "", clientCounters{}, false
	}

	// `<CLIENT_LIST 1 CN ...>` includes a numeric client id; the regular
	// `CLIENT_LIST CN ...>` form does not. Common names are email addresses,
	// so a numeric token immediately after the marker is unambiguously the id.
	base := 1
	if first == "<CLIENT_LIST" {
		base = 2
	} else if _, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
		base = 2
	}
	if base >= len(fields) {
		return "", clientCounters{}, false
	}
	cn := fields[base]
	counters, ok := parseCounterPair(fields, base+3)
	if !ok || cn == "" {
		return "", clientCounters{}, false
	}
	return cn, counters, true
}

// parseClientListResponse parses one complete management response. It is kept
// separate from the socket code so the real OpenVPN response formats can be
// tested without requiring an OpenVPN daemon in CI.
func parseClientListResponse(reader io.Reader) (map[string]clientCounters, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	out := make(map[string]clientCounters)
	var layout *clientListLayout
	terminated := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "OK" || strings.HasPrefix(line, ">INFO:") ||
			strings.HasPrefix(line, "TITLE,") || strings.HasPrefix(line, "TIME,") {
			continue
		}
		if line == "END" || line == "<END" {
			terminated = true
			break
		}
		if strings.HasPrefix(line, "ERROR:") {
			return nil, errors.New(strings.TrimSpace(line))
		}
		if strings.HasPrefix(line, "OpenVPN CLIENT LIST") ||
			line == "ROUTING TABLE" || line == "GLOBAL STATS" ||
			strings.HasPrefix(line, "HEADER,ROUTING_TABLE") {
			continue
		}

		// The old management response is space-separated and starts with
		// <CLIENT_LIST (or CLIENT_LIST). Try it before CSV parsing because its
		// header contains spaces and is not a valid CSV schema by itself.
		if cn, counters, ok := parseSpaceClientRow(line); ok {
			out[cn] = counters
			continue
		}

		if !strings.Contains(line, ",") {
			continue
		}
		fields, err := csvFields(line)
		if err != nil {
			continue
		}
		if parsed, ok := clientListLayoutFromHeader(fields); ok {
			layoutCopy := parsed
			layout = &layoutCopy
			continue
		}
		if cn, counters, ok := parseCSVClientRow(fields, layout); ok {
			out[cn] = counters
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !terminated {
		return nil, errors.New("openvpn management response ended before END")
	}
	return out, nil
}

func queryClientListCommand(mgmtPort int, command string) (map[string]clientCounters, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", mgmtPort), 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if _, err := conn.Write([]byte(command + "\n")); err != nil {
		return nil, err
	}
	return parseClientListResponse(conn)
}

// clientList queries the daemon's management interface (CLIENT_LIST/status)
// and returns current connected clients keyed by common name. OpenVPN builds
// in the status command on all supported server versions, while a few older
// builds expose the dedicated CLIENT_LIST command; try the latter first to
// preserve compatibility with both response styles.
func clientList(mgmtPort int) (map[string]clientCounters, error) {
	var lastErr error
	for _, command := range []string{"CLIENT_LIST", "status 2"} {
		clients, err := queryClientListCommand(mgmtPort, command)
		if err == nil {
			return clients, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
