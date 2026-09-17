package openvpn

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func tempBinFolder(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
}

// inboundStub returns a minimal enabled openvpn inbound for parser tests.
func inboundStub() *model.Inbound {
	return &model.Inbound{
		Id:       7,
		Tag:      "ovpn1",
		Protocol: model.OpenVPN,
		Port:     1194,
		Enable:   true,
	}
}

func TestInstanceFromInboundDefaults(t *testing.T) {
	tempBinFolder(t)
	ib := inboundStub()
	inst, ok := InstanceFromInbound(ib, []string{"b@x.com", "a@x.com"})
	if !ok {
		t.Fatal("expected a usable instance")
	}
	if inst.Id != 7 || inst.Port != 1194 || inst.Tag != "ovpn1" {
		t.Fatalf("bad identity: %+v", inst)
	}
	// Toggles default on when the key is missing.
	if !inst.RedirectGw || !inst.PushDNS {
		t.Fatalf("expected redirect-gateway and DNS push on by default: %+v", inst)
	}
	if inst.protoFor() != "udp" {
		t.Fatalf("default proto should be udp, got %q", inst.Proto)
	}
	if inst.dns1For() != "1.1.1.1" || inst.dns2For() != "8.8.8.8" {
		t.Fatalf("default DNS not applied: %s %s", inst.dns1For(), inst.dns2For())
	}
	// Subnet is deterministic and unique per inbound id.
	net1, mask := inst.serverSubnet()
	if net1 != "10.7.0.0" || mask != "255.255.255.0" {
		t.Fatalf("unexpected subnet for id 7: %s %s", net1, mask)
	}
}

func TestInstanceFromInboundExplicitFalse(t *testing.T) {
	tempBinFolder(t)
	ib := inboundStub()
	ib.Settings = `{"proto":"tcp","redirectGateway":false,"pushDNS":false,"dns1":"9.9.9.9"}`
	inst, ok := InstanceFromInbound(ib, nil)
	if !ok {
		t.Fatal("expected a usable instance")
	}
	if inst.protoFor() != "tcp" {
		t.Fatalf("expected tcp proto, got %q", inst.Proto)
	}
	if inst.RedirectGw || inst.PushDNS {
		t.Fatalf("explicit false must be respected: %+v", inst)
	}
	if inst.dns1For() != "9.9.9.9" {
		t.Fatalf("custom dns1 ignored: %s", inst.dns1For())
	}
}

func TestInstanceFromInboundRejectsOtherProtocols(t *testing.T) {
	tempBinFolder(t)
	ib := inboundStub()
	ib.Protocol = "vless"
	if _, ok := InstanceFromInbound(ib, nil); ok {
		t.Fatal("vless inbound must not produce an openvpn instance")
	}
}

func TestFingerprintChangesWithClientsAndSettings(t *testing.T) {
	base := Instance{Id: 1, Port: 1194, Proto: "udp", RedirectGw: true, PushDNS: true, Clients: []string{"a@x.com"}}
	same := Instance{Id: 1, Port: 1194, Proto: "udp", RedirectGw: true, PushDNS: true, Clients: []string{"a@x.com"}}
	addClient := Instance{Id: 1, Port: 1194, Proto: "udp", RedirectGw: true, PushDNS: true, Clients: []string{"a@x.com", "b@x.com"}}
	newPort := Instance{Id: 1, Port: 1195, Proto: "udp", RedirectGw: true, PushDNS: true, Clients: []string{"a@x.com"}}
	newProto := Instance{Id: 1, Port: 1194, Proto: "tcp", RedirectGw: true, PushDNS: true, Clients: []string{"a@x.com"}}
	newGw := Instance{Id: 1, Port: 1194, Proto: "udp", RedirectGw: false, PushDNS: true, Clients: []string{"a@x.com"}}
	reordered1 := Instance{Id: 1, Port: 1194, Proto: "udp", RedirectGw: true, PushDNS: true, Clients: []string{"c@x.com", "a@x.com"}}
	reordered2 := Instance{Id: 1, Port: 1194, Proto: "udp", RedirectGw: true, PushDNS: true, Clients: []string{"a@x.com", "c@x.com"}}
	if same.fingerprint() != base.fingerprint() {
		t.Fatal("identical instances must have identical fingerprints")
	}
	if addClient.fingerprint() == base.fingerprint() {
		t.Fatal("adding a client must change the fingerprint")
	}
	if newPort.fingerprint() == base.fingerprint() {
		t.Fatal("port change must change the fingerprint")
	}
	if newProto.fingerprint() == base.fingerprint() {
		t.Fatal("proto change must change the fingerprint")
	}
	if newGw.fingerprint() == base.fingerprint() {
		t.Fatal("redirect-gateway change must change the fingerprint")
	}
	// Order of the client list must not matter.
	if reordered1.fingerprint() != reordered2.fingerprint() {
		t.Fatal("client order must not affect the fingerprint")
	}
}

func TestCertGenerationChain(t *testing.T) {
	tempBinFolder(t)
	dir := dataDirForID(42)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ensureServerCert(dir); err != nil {
		t.Fatalf("ensureServerCert: %v", err)
	}
	email := "user 1@example.com" // unusual chars exercise sanitization
	if err := ensureClientCert(dir, email); err != nil {
		t.Fatalf("ensureClientCert: %v", err)
	}
	caCrt, caKey, err := loadKeyPair(caFiles(dir))
	if err != nil {
		t.Fatalf("load CA: %v", err)
	}
	if !caCrt.IsCA {
		t.Fatal("CA certificate must have IsCA set")
	}
	if _, err := caCrt.CheckSignatureFrom(caCrt); err != nil {
		t.Fatalf("CA self-signature invalid: %v", err)
	}
	_ = caKey

	// Server cert chains to the CA and carries server auth.
	srvCrtPath, srvKeyPath := serverFiles(dir)
	srvCrt, _, err := loadKeyPair(srvCrtPath, srvKeyPath)
	if err != nil {
		t.Fatalf("load server cert: %v", err)
	}
	if err := srvCrt.CheckSignatureFrom(caCrt); err != nil {
		t.Fatalf("server cert not signed by CA: %v", err)
	}
	foundServerAuth := false
	for _, eu := range srvCrt.ExtKeyUsage {
		if eu == x509.ExtKeyUsageServerAuth {
			foundServerAuth = true
		}
	}
	if !foundServerAuth {
		t.Fatal("server cert missing serverAuth EKU")
	}

	// Client cert: CN must be the raw email (that is the identity openvpn
	// reports), file name must be sanitized, and it must chain to the CA.
	crtPath, keyPath := clientFiles(dir, email)
	if filepath.Base(crtPath) != "user_1_example.com.crt" {
		t.Fatalf("expected sanitized client file name, got %s", crtPath)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("client key missing: %v", err)
	}
	clientCrt, _, err := loadKeyPair(crtPath, keyPath)
	if err != nil {
		t.Fatalf("load client cert: %v", err)
	}
	if clientCrt.Subject.CommonName != email {
		t.Fatalf("client CN must be the raw email, got %q", clientCrt.Subject.CommonName)
	}
	if err := clientCrt.CheckSignatureFrom(caCrt); err != nil {
		t.Fatalf("client cert not signed by CA: %v", err)
	}

	// Idempotency: regenerating must not replace existing material.
	before, _ := os.ReadFile(crtPath)
	if err := ensureClientCert(dir, email); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(crtPath)
	if string(before) != string(after) {
		t.Fatal("existing client cert must not be regenerated")
	}
}

func TestPruneClientCerts(t *testing.T) {
	tempBinFolder(t)
	dir := dataDirForID(43)
	if err := os.MkdirAll(filepath.Join(dir, "clients"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ensureServerCert(dir); err != nil {
		t.Fatal(err)
	}
	keep := "keep@example.com"
	drop := "drop@example.com"
	for _, e := range []string{keep, drop} {
		if err := ensureClientCert(dir, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneClientCerts(dir, []string{keep}); err != nil {
		t.Fatal(err)
	}
	dropCrt, _ := clientFiles(dir, drop)
	if _, err := os.Stat(dropCrt); !os.IsNotExist(err) {
		t.Fatal("dropped client cert must be removed")
	}
	keepCrt, _ := clientFiles(dir, keep)
	if _, err := os.Stat(keepCrt); err != nil {
		t.Fatal("kept client cert must survive pruning")
	}
}

func TestRenderServerConf(t *testing.T) {
	tempBinFolder(t)
	inst := Instance{
		Id: 5, Tag: "ovpn", Port: 1194, Proto: "tcp",
		RedirectGw: true, PushDNS: true, DNS1: "1.1.1.1", DNS2: "8.8.8.8",
		Clients: []string{"a@x.com"},
	}
	conf := renderServerConf(inst, 43210)
	mustContain := []string{
		"mode server",
		"port 1194",
		"proto tcp-server",
		"dev tun5",
		"tls-server",
		"tls-version-min 1.2",
		"auth sha256",
		"cipher AES-256-GCM",
		"server 10.5.0.0 255.255.255.0",
		"keepalive 10 120",
		"management 127.0.0.1 43210",
		`push "redirect-gateway def1 bypass-dns"`,
		`push "dhcp-option DNS 1.1.1.1"`,
		`push "dhcp-option DNS 8.8.8.8"`,
	}
	for _, want := range mustContain {
		if !strings.Contains(conf, want) {
			t.Errorf("config missing %q:\n%s", want, conf)
		}
	}
	if !strings.Contains(conf, filepath.Join(dataDirForID(5), "ca.crt")) {
		t.Errorf("config missing ca path:\n%s", conf)
	}

	// Toggles off -> no push directives, udp transport.
	inst.Proto = "udp"
	inst.RedirectGw = false
	inst.PushDNS = false
	conf2 := renderServerConf(inst, 1)
	if !strings.Contains(conf2, "proto udp") {
		t.Errorf("expected proto udp:\n%s", conf2)
	}
	if strings.Contains(conf2, "redirect-gateway") || strings.Contains(conf2, "dhcp-option") {
		t.Errorf("disabled pushes must be omitted:\n%s", conf2)
	}
	// Default listen (0.0.0.0 / empty) must not emit a `local` directive.
	if strings.Contains(conf2, "local ") {
		t.Errorf("default listen must not emit local:\n%s", conf2)
	}
	inst.Listen = "192.0.2.1"
	if !strings.Contains(renderServerConf(inst, 1), "local 192.0.2.1") {
		t.Error("explicit listen must emit local directive")
	}
}

func TestWriteConfigCreatesFiles(t *testing.T) {
	tempBinFolder(t)
	inst := Instance{Id: 9, Tag: "t", Port: 1194, Proto: "udp", RedirectGw: true, PushDNS: true}
	if err := writeConfig(inst, 40000); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(confPathForID(9))
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if !strings.Contains(string(data), "management 127.0.0.1 40000") {
		t.Error("written config missing management line")
	}
}

// fakeMgmtServer speaks just enough of the OpenVPN management protocol to
// drive clientList and the manager's traffic accounting.
type fakeMgmtServer struct {
	ln      net.Listener
	port    int
	mu      sync.Mutex
	clients map[string]clientCounters
}

func (s *fakeMgmtServer) start(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serveConn(conn)
		}
	}()
}

func (s *fakeMgmtServer) snapshot() map[string]clientCounters {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]clientCounters, len(s.clients))
	for k, v := range s.clients {
		out[k] = v
	}
	return out
}

func (s *fakeMgmtServer) serveConn(conn net.Conn) {
	defer conn.Close()
	fmt.Fprintln(conn, "OK")
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	req := strings.TrimSpace(string(buf[:n]))
	if req != "CLIENT_LIST" {
		return
	}
	fmt.Fprintln(conn, "<CLIENT_LIST VERSION 1")
	fmt.Fprintln(conn, "<CLIENT_LIST HEADER ID Common Name Real Address Virtual Address Bytes Received Bytes Sent Connected Since")
	for cn, c := range s.snapshot() {
		fmt.Fprintf(conn, "<CLIENT_LIST 1 %s 203.0.113.7:40000 10.0.0.2 %d %d 1735689600\n", cn, c.Rx, c.Tx)
	}
	fmt.Fprintln(conn, "<END")
}

func (s *fakeMgmtServer) setClient(cn string, c clientCounters) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[cn] = c
}

func (s *fakeMgmtServer) clearClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients = map[string]clientCounters{}
}

func (s *fakeMgmtServer) close() {
	_ = s.ln.Close()
}

func TestClientListParsing(t *testing.T) {
	srv := &fakeMgmtServer{clients: map[string]clientCounters{
		"a@x.com": {Rx: 100, Tx: 200},
		"b@x.com": {Rx: 1, Tx: 2},
	}}
	srv.start(t)
	defer srv.close()

	got, err := clientList(srv.port)
	if err != nil {
		t.Fatalf("clientList: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 clients, got %d: %v", len(got), got)
	}
	if got["a@x.com"].Rx != 100 || got["a@x.com"].Tx != 200 {
		t.Fatalf("bad counters for a: %+v", got["a@x.com"])
	}
	if got["b@x.com"].Rx != 1 || got["b@x.com"].Tx != 2 {
		t.Fatalf("bad counters for b: %+v", got["b@x.com"])
	}
}

func TestClientListEmpty(t *testing.T) {
	srv := &fakeMgmtServer{clients: map[string]clientCounters{}}
	srv.start(t)
	defer srv.close()
	got, err := clientList(srv.port)
	if err != nil {
		t.Fatalf("clientList: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no clients, got %v", got)
	}
}

func TestClientListErrorWhenClosed(t *testing.T) {
	srv := &fakeMgmtServer{}
	srv.start(t)
	port := srv.port
	srv.close()
	if _, err := clientList(port); err == nil {
		t.Fatal("expected an error against a closed management port")
	}
}

func TestManagerCollectTrafficDeltas(t *testing.T) {
	tempBinFolder(t)
	m := &Manager{procs: map[int]*managed{}}
	srv := &fakeMgmtServer{clients: map[string]clientCounters{"a@x.com": {Rx: 1000, Tx: 500}}}
	srv.start(t)
	defer srv.close()

	m.procs[1] = &managed{
		tag:         "ovpn1",
		fingerprint: "x",
		mgmtPort:    srv.port,
		clients:     []string{"a@x.com"},
		lastRx:      map[string]int64{},
		lastTx:      map[string]int64{},
		online:      map[string]bool{},
	}

	// First poll: full counters count as delta (fresh daemon view).
	ins, cls := m.CollectTraffic()
	if len(ins) != 1 || ins[0].Up != 1000 || ins[0].Down != 500 {
		t.Fatalf("first poll inbound delta wrong: %+v", ins)
	}
	if len(cls) != 1 || cls[0].Email != "a@x.com" || cls[0].Up != 1000 || cls[0].Down != 500 {
		t.Fatalf("first poll client delta wrong: %+v", cls)
	}
	if online := m.OnlineEmails(); len(online) != 1 || online[0] != "a@x.com" {
		t.Fatalf("online set wrong: %v", online)
	}

	// Second poll: only the increment is reported.
	srv.setClient("a@x.com", clientCounters{Rx: 1600, Tx: 800})
	ins, cls = m.CollectTraffic()
	if len(ins) != 1 || ins[0].Up != 600 || ins[0].Down != 300 {
		t.Fatalf("second poll must report deltas only: %+v", ins)
	}
	if len(cls) != 1 || cls[0].Up != 600 || cls[0].Down != 300 {
		t.Fatalf("second poll client delta wrong: %+v", cls)
	}

	// Third poll: no movement -> nothing reported, client still online.
	ins, cls = m.CollectTraffic()
	if len(ins) != 0 || len(cls) != 0 {
		t.Fatalf("idle poll must be empty: in=%+v c=%+v", ins, cls)
	}
	if online := m.OnlineEmails(); len(online) != 1 {
		t.Fatalf("idle client must stay online: %v", online)
	}

	// Counter reset (daemon restart) must count from zero, never negative.
	srv.setClient("a@x.com", clientCounters{Rx: 10, Tx: 20})
	ins, _ = m.CollectTraffic()
	if len(ins) != 1 || ins[0].Up != 10 || ins[0].Down != 20 {
		t.Fatalf("reset handling wrong: %+v", ins)
	}

	// Client disconnects: dropped from the online set, counters forgotten.
	srv.clearClients()
	_, _ = m.CollectTraffic()
	if online := m.OnlineEmails(); len(online) != 0 {
		t.Fatalf("disconnected client must leave the online set: %v", online)
	}
	if _, ok := m.procs[1].lastRx["a@x.com"]; ok {
		t.Fatal("disconnected client counters must be forgotten")
	}
}

func TestManagerOnlineEmailsUnion(t *testing.T) {
	tempBinFolder(t)
	m := &Manager{procs: map[int]*managed{}}
	m.procs[1] = &managed{online: map[string]bool{"a@x.com": true}}
	m.procs[2] = &managed{online: map[string]bool{"b@x.com": true, "a@x.com": true}}
	online := m.OnlineEmails()
	seen := map[string]bool{}
	for _, e := range online {
		seen[e] = true
	}
	if !seen["a@x.com"] || !seen["b@x.com"] || len(online) != 2 {
		t.Fatalf("expected union of 2 unique emails, got %v", online)
	}
}

func TestBuildProfile(t *testing.T) {
	tempBinFolder(t)
	id := 11
	dir := dataDirForID(id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	email := "profile@example.com"
	if err := ensureServerCert(dir); err != nil {
		t.Fatal(err)
	}
	if err := ensureClientCert(dir, email); err != nil {
		t.Fatal(err)
	}
	inst := Instance{Id: id, Port: 1195, Proto: "tcp", RedirectGw: true, PushDNS: true}
	profile, err := BuildProfile(inst, email, "vpn.example.com")
	if err != nil {
		t.Fatalf("BuildProfile: %v", err)
	}
	for _, want := range []string{
		"client",
		"dev tun",
		"proto tcp",
		"remote vpn.example.com 1195",
		"nobind",
		"persist-key",
		"persist-tun",
		"remote-cert-tls server",
		"auth sha256",
		"cipher AES-256-GCM",
		"<ca>",
		"<cert>",
		"<key>",
	} {
		if !strings.Contains(profile, want) {
			t.Errorf("profile missing %q", want)
		}
	}
	// Embedded blocks must be parseable PEM.
	for _, block := range []string{"ca", "cert", "key"} {
		start := strings.Index(profile, "<"+block+">")
		end := strings.Index(profile, "</"+block+">")
		if start < 0 || end < 0 || end < start {
			t.Fatalf("profile missing <%s> block", block)
		}
		inner := profile[start+len(block)+2 : end]
		pemBlock, _ := pem.Decode([]byte(inner))
		if pemBlock == nil {
			t.Fatalf("profile <%s> block is not valid PEM", block)
		}
	}
}

func TestBuildProfileMissingCert(t *testing.T) {
	tempBinFolder(t)
	id := 12
	inst := Instance{Id: id, Port: 1194, Proto: "udp"}
	if _, err := BuildProfile(inst, "ghost@example.com", "h"); err == nil {
		t.Fatal("expected an error when the client cert does not exist yet")
	}
}

func TestSanitizeCertName(t *testing.T) {
	cases := map[string]string{
		"plain@example.com": "plain_example.com",
		"first.last+x@ex.com": "first.last+x_ex.com",
		"ünïcödé@ex.com":      "_n_c_d__ex.com",
		"":                    "client",
		"//\\<>|?\"@ex.com":   "_________ex.com",
	}
	for in, want := range cases {
		if got := sanitizeCertName(in); got != want {
			t.Errorf("sanitizeCertName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFreeLocalPort(t *testing.T) {
	port, err := FreeLocalPort()
	if err != nil {
		t.Fatal(err)
	}
	if port < 1 || port > 65535 {
		t.Fatalf("bad port: %d", port)
	}
}
