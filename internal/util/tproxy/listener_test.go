package tproxy

import (
	"net"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestScanProcSockets(t *testing.T) {
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:F9AB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0100007F:F9AC 0100007F:C350 01 00000000:00000000 00:00000000 00000000     0        0 12346 1 0000000000000000 20 4 30 10 -1\n"
	if !scanProcSockets(strings.NewReader(tcp), procSocketAddrs(0xF9AB), "0A") {
		t.Fatal("listening socket on 127.0.0.1:63915 must be found")
	}
	if scanProcSockets(strings.NewReader(tcp), procSocketAddrs(0xF9AC), "0A") {
		t.Fatal("an established socket must not count as a listener")
	}
	if scanProcSockets(strings.NewReader(tcp), procSocketAddrs(0xF9AD), "0A") {
		t.Fatal("unbound port must not be found")
	}
	udp := "   sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops\n" +
		"  100: 00000000:F9AB 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 555 2 0000000000000000 0\n"
	if !scanProcSockets(strings.NewReader(udp), procSocketAddrs(0xF9AB), "") {
		t.Fatal("wildcard-bound UDP socket must be accepted")
	}
	if scanProcSockets(strings.NewReader(""), procSocketAddrs(1), "") {
		t.Fatal("empty table must not match")
	}
}

func TestListenerBound(t *testing.T) {
	if runtime.GOOS != "linux" {
		if ListenerBound(63901) {
			t.Fatal("ListenerBound must be false without /proc/net")
		}
		return
	}
	ln, err := net.Listen("tcp", ListenIP+":0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	// TCP alone is not enough: the relay needs the UDP half as well.
	if ListenerBound(port) {
		t.Fatal("ListenerBound must require the UDP socket too")
	}
	pc, err := net.ListenPacket("udp", ListenIP+":"+portStr)
	if err != nil {
		t.Skipf("cannot bind udp on loopback: %v", err)
	}
	if !ListenerBound(port) {
		pc.Close()
		t.Fatal("ListenerBound must see the TCP listener + UDP socket pair")
	}
	pc.Close()
	ln.Close()
	if ListenerBound(port) {
		t.Fatal("ListenerBound must be false once the sockets are closed")
	}
	if ListenerBound(0) || ListenerBound(70000) {
		t.Fatal("out-of-range ports must be false")
	}
}
