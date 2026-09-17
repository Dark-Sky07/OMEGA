package openvpn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildProfile renders a ready-to-use .ovpn client profile for one client of
// one inbound, embedding the CA, the server certificate (for peer
// verification) and the client's own keypair. `host` is the public address
// the client should dial (share address / domain / panel host).
func BuildProfile(inst Instance, email, host string) (string, error) {
	dir := dataDirForID(inst.Id)
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return "", fmt.Errorf("openvpn: CA for inbound %d not found (daemon not provisioned yet): %w", inst.Id, err)
	}
	serverCrt, err := os.ReadFile(filepath.Join(dir, "server.crt"))
	if err != nil {
		return "", fmt.Errorf("openvpn: server certificate for inbound %d not found: %w", inst.Id, err)
	}
	crtPath, keyPath := clientFiles(dir, email)
	crt, err := os.ReadFile(crtPath)
	if err != nil {
		return "", fmt.Errorf("openvpn: certificate for client %s not found (re-attaching the client generates it): %w", email, err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("openvpn: key for client %s not found: %w", email, err)
	}

	proto := "udp"
	if inst.protoFor() == "tcp" {
		proto = "tcp"
	}

	var b strings.Builder
	b.WriteString("client\ndev tun\n")
	fmt.Fprintf(&b, "proto %s\n", proto)
	fmt.Fprintf(&b, "remote %s %d\n", host, inst.Port)
	b.WriteString("float\nresolv-retry infinite\nnobind\n")
	b.WriteString("persist-key\npersist-tun\n")
	b.WriteString("remote-cert-tls server\n")
	b.WriteString("auth sha256\ncipher AES-256-GCM\ntls-version-min 1.2\n")
	b.WriteString("verb 3\nmute-replay-warnings\n")
	fmt.Fprintf(&b, "<ca>\n%s</ca>\n", strings.TrimRight(string(ca), "\n"))
	fmt.Fprintf(&b, "<cert>\n%s</cert>\n", strings.TrimRight(string(crt), "\n"))
	fmt.Fprintf(&b, "<key>\n%s</key>\n", strings.TrimRight(string(key), "\n"))
	fmt.Fprintf(&b, "<ca-peer>\n%s</ca-peer>\n", strings.TrimRight(string(serverCrt), "\n"))
	return b.String(), nil
}
