package openvpn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildProfile renders a ready-to-use .ovpn client profile for one client of
// one inbound, embedding the CA and the client's own keypair. `host` is the
// public address the client should dial (share address / domain / panel
// host).
//
// Inline blocks follow the OpenVPN convention strictly: every opening and
// closing tag occupies its own line. OpenVPN Connect (and the iOS/Android
// clients) reject profiles where a closing tag shares a line with the PEM
// footer (e.g. `-----END CERTIFICATE-----</ca>`) with
// `option <ca> was not properly closed out`.
//
// Only the standard <ca>/<cert>/<key> inline options are emitted. The CA
// already authenticates the server certificate (which carries the serverAuth
// EKU, enforced client-side via `remote-cert-tls server`), so no extra
// non-standard block is needed.
//
// EnsureProfileMaterial is intentionally idempotent. The reconcile job normally
// creates these files, but a user can request a profile immediately after
// attaching a client, before the next ten-second reconcile tick. Provisioning
// on demand closes that race and also heals a partially-created inbound.
func EnsureProfileMaterial(inst Instance, email string) error {
	if strings.TrimSpace(email) == "" {
		return fmt.Errorf("openvpn: client email is empty")
	}
	dir := dataDirForID(inst.Id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := ensureServerCert(dir); err != nil {
		return err
	}
	return ensureClientCert(dir, email)
}

func BuildProfile(inst Instance, email, host string) (string, error) {
	if err := EnsureProfileMaterial(inst, email); err != nil {
		return "", fmt.Errorf("openvpn: provision certificate for client %s: %w", email, err)
	}
	dir := dataDirForID(inst.Id)
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return "", fmt.Errorf("openvpn: CA for inbound %d not found (daemon not provisioned yet): %w", inst.Id, err)
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
	fmt.Fprintf(&b, "<ca>\n%s\n</ca>\n", strings.TrimRight(string(ca), "\r\n"))
	fmt.Fprintf(&b, "<cert>\n%s\n</cert>\n", strings.TrimRight(string(crt), "\r\n"))
	fmt.Fprintf(&b, "<key>\n%s\n</key>\n", strings.TrimRight(string(key), "\r\n"))
	return b.String(), nil
}
