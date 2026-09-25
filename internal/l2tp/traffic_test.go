package l2tp

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTrafficFilesystem(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	l2tpRootOverride = filepath.Join(root, "l2tp")
	l2tpSysClassNetRoot = filepath.Join(root, "sys", "class", "net")
	t.Cleanup(func() {
		l2tpRootOverride = ""
		l2tpSysClassNetRoot = "/sys/class/net"
	})
	return l2tpRootOverride, l2tpSysClassNetRoot
}

func writeSession(t *testing.T, id int, iface, email, rx, tx string, interfaceExists bool) {
	t.Helper()
	sessions := sessionDirForID(id)
	if err := os.MkdirAll(sessions, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessions, iface), []byte(email+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if !interfaceExists {
		return
	}
	stats := filepath.Join(l2tpSysClassNetRoot, iface, "statistics")
	if err := os.MkdirAll(stats, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stats, "rx_bytes"), []byte(rx+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stats, "tx_bytes"), []byte(tx+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
}

func TestReadSessionsSupportsConcurrentSessionsForOneEmail(t *testing.T) {
	setupTrafficFilesystem(t)
	writeSession(t, 7, "ppp0", "alice@example.com", "100", "200", true)
	writeSession(t, 7, "ppp1", "alice@example.com", "300", "400", true)

	got, ok := readSessions(7)
	if !ok {
		t.Fatal("readSessions reported a transient failure")
	}
	if len(got) != 2 {
		t.Fatalf("read %d sessions, want two independent interface/email keys: %#v", len(got), got)
	}
	if got["ppp0\x00alice@example.com"].rx != 100 || got["ppp1\x00alice@example.com"].tx != 400 {
		t.Fatalf("session counters were not read independently: %#v", got)
	}
}

func TestReadSessionsDropsStaleMarkerWithoutInterface(t *testing.T) {
	setupTrafficFilesystem(t)
	writeSession(t, 8, "ppp-stale", "offline@example.com", "10", "20", false)

	got, ok := readSessions(8)
	if !ok {
		t.Fatal("a stale marker should be a healthy empty-session read")
	}
	if len(got) != 0 {
		t.Fatalf("stale marker produced an online session: %#v", got)
	}
}

func TestReadSessionsFailsClosedOnInvalidCounter(t *testing.T) {
	setupTrafficFilesystem(t)
	writeSession(t, 9, "ppp0", "alice@example.com", "not-a-counter", "20", true)

	got, ok := readSessions(9)
	if ok || got != nil {
		t.Fatalf("invalid interface counter was accepted: got=%#v ok=%v", got, ok)
	}
}

func TestReadCounterParsesUnsignedValuesWithoutNegativeWrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "counter")
	if err := os.WriteFile(path, []byte("18446744073709551615\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	value, ok := readCounter(path)
	if !ok || value != maxInterfaceCounter {
		t.Fatalf("unsigned counter = (%d, %v), want saturated MaxInt64", value, ok)
	}

	if err := os.WriteFile(path, []byte("-1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if value, ok := readCounter(path); ok || value != 0 {
		t.Fatalf("negative counter was accepted: (%d, %v)", value, ok)
	}
}
