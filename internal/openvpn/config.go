package openvpn

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// dataDirForID is the per-inbound directory holding the CA, server and client
// material, the rendered openvpn.conf, the daemon log and the pid file. It
// lives under the bin folder next to the panel's own binaries, mirroring the
// mtproto (mtg) layout.
func dataDirForID(id int) string {
	return openvpnDir() + "/" + strconv.Itoa(id)
}

func confPathForID(id int) string  { return filepath.Join(dataDirForID(id), "openvpn.conf") }
func pidPathForID(id int) string   { return filepath.Join(dataDirForID(id), "openvpn.pid") }
func logPathForID(id int) string   { return filepath.Join(dataDirForID(id), "openvpn.log") }
func devNameForID(id int) string   { return "tun" + strconv.Itoa(id) }

// renderServerConf builds the openvpn.conf for one inbound. Directives are
// deliberately limited to ones present since OpenVPN 2.4 so the generated
// file works on both 2.4/2.5 and 2.6 installations.
func renderServerConf(inst Instance, mgmtPort int) string {
	proto := "udp"
	if inst.protoFor() == "tcp" {
		proto = "tcp-server"
	}
	network, mask := inst.serverSubnet()
	dir := dataDirForID(inst.Id)

	var b strings.Builder
	b.WriteString("mode server\n")
	if listen := inst.listenFor(); listen != "" {
		fmt.Fprintf(&b, "local %s\n", listen)
	}
	fmt.Fprintf(&b, "port %d\n", inst.Port)
	fmt.Fprintf(&b, "proto %s\n", proto)
	fmt.Fprintf(&b, "dev %s\n", devNameForID(inst.Id))
	fmt.Fprintf(&b, "ca %s\n", filepath.Join(dir, "ca.crt"))
	fmt.Fprintf(&b, "cert %s\n", filepath.Join(dir, "server.crt"))
	fmt.Fprintf(&b, "key %s\n", filepath.Join(dir, "server.key"))
	b.WriteString("tls-server\ntls-version-min 1.2\n")
	// The generated server certificate is ECDSA P-256. OpenVPN 2.5 still
	// requires an explicit DH setting; `dh none` selects the ECDHE path instead
	// of looking for a legacy finite-field DH parameter file.
	b.WriteString("dh none\necdh-curve prime256v1\n")
	b.WriteString("auth sha256\ncipher AES-256-GCM\n")
	fmt.Fprintf(&b, "server %s %s\n", network, mask)
	// Use subnet topology explicitly so modern OpenVPN Connect clients get
	// one predictable tunnel address rather than the legacy net30 pairing.
	b.WriteString("topology subnet\n")
	b.WriteString("client-to-client\n")
	b.WriteString("verify-client-cert require\nremote-cert-tls client\n")
	b.WriteString("keepalive 10 120\n")
	if inst.protoFor() == "udp" {
		b.WriteString("explicit-exit-notify 1\n")
	}
	b.WriteString("persist-key\npersist-tun\n")
	b.WriteString("mute-replay-warnings\nstatus-version 2\nverb 3\n")
	fmt.Fprintf(&b, "writepid %s\n", pidPathForID(inst.Id))
	fmt.Fprintf(&b, "log %s\n", logPathForID(inst.Id))
	fmt.Fprintf(&b, "management 127.0.0.1 %d\n", mgmtPort)
	if inst.RedirectGw {
		b.WriteString(`push "redirect-gateway def1 bypass-dns"` + "\n")
	}
	if inst.PushDNS {
		fmt.Fprintf(&b, `push "dhcp-option DNS %s"`+"\n", inst.dns1For())
		fmt.Fprintf(&b, `push "dhcp-option DNS %s"`+"\n", inst.dns2For())
	}
	return b.String()
}

// writeConfig renders the server config into the inbound's data directory.
func writeConfig(inst Instance, mgmtPort int) error {
	dir := dataDirForID(inst.Id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return os.WriteFile(confPathForID(inst.Id), []byte(renderServerConf(inst, mgmtPort)), 0o640)
}
