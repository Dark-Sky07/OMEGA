package l2tp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
)

const (
	strongSwanRuntimeRoot = "/etc/strongswan.d/omega-l2tp"

	// pppd does not accept a per-options-file chap-secrets path. It always
	// reads /etc/ppp/chap-secrets, so OMEGA owns only a marked block there and
	// preserves entries managed by other PPP services.
	systemChapSecretsPath = "/etc/ppp/chap-secrets"
	omegaChapSecretsBegin = "# BEGIN OMEGA L2TP MANAGED CREDENTIALS"
	omegaChapSecretsEnd   = "# END OMEGA L2TP MANAGED CREDENTIALS"
)

// l2tpBinDir resolves the panel's binary directory to an absolute path. The
// default XUI_BIN_FOLDER is relative ("bin"), but pppd runs ip-up/ip-down
// scripts from its own working directory rather than the panel's
// WorkingDirectory. Embedding a relative path in those scripts silently
// loses the session marker, so traffic collection must use the same absolute
// root as the panel process.
func l2tpBinDir() string {
	dir := config.GetBinFolderPath()
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), dir)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// l2tpRootOverride is test-only indirection for the filesystem accounting
// tests. It remains empty in production, so runtime paths stay under the
// configured panel binary directory.
var l2tpRootOverride string

func l2tpRoot() string {
	if l2tpRootOverride != "" {
		return l2tpRootOverride
	}
	return filepath.Join(l2tpBinDir(), "l2tp")
}

func dataDirForID(id int) string {
	return filepath.Join(l2tpRoot(), fmt.Sprintf("%d", id))
}

func strongSwanDirForID(id int) string {
	return filepath.Join(strongSwanRuntimeRoot, fmt.Sprintf("%d", id))
}

func ipsecConfigPath(id int) string  { return filepath.Join(dataDirForID(id), "ipsec.conf") }
func ipsecSecretsPath(id int) string { return filepath.Join(strongSwanDirForID(id), "ipsec.secrets") }
func strongSwanConfigPath(id int) string {
	return filepath.Join(strongSwanDirForID(id), "strongswan.conf")
}
func xl2tpdConfigPath(id int) string { return filepath.Join(dataDirForID(id), "xl2tpd.conf") }
func pppOptionsPath(id int) string   { return filepath.Join(dataDirForID(id), "options.xl2tpd") }
func chapSecretsPath(id int) string  { return filepath.Join(dataDirForID(id), "chap-secrets") }
func xl2tpdPIDPath(id int) string    { return filepath.Join(dataDirForID(id), "xl2tpd.pid") }
func sessionDirPath(id int) string   { return filepath.Join(dataDirForID(id), "sessions") }
func ipUpScriptPath(id int) string   { return filepath.Join(dataDirForID(id), "ip-up") }
func ipDownScriptPath(id int) string { return filepath.Join(dataDirForID(id), "ip-down") }
func strongSwanPIDDir(id int) string { return dataDirForID(id) }

func removeStrongSwanRuntime(id int) {
	_ = os.RemoveAll(strongSwanDirForID(id))
}

func quoteIPsec(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// quotePPP quotes a chap-secrets field. pppd accepts quoted fields and this
// keeps spaces and punctuation in an existing panel password from becoming
// additional columns. Newlines are rejected before this function is called.
func quotePPP(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func renderIPsecConf(inst Instance) string {
	var b strings.Builder
	b.WriteString("# Managed by OMEGA. Do not edit; changes are reconciled from the panel.\n")
	b.WriteString("config setup\n")
	b.WriteString("    uniqueids=no\n")
	// Keep library/config diagnostics enabled until the distro-specific
	// strongswan.d layout has been validated; otherwise charon only reports the
	// generic "invalid configuration" exit and hides the offending file/line.
	b.WriteString("    charondebug=\"ike 1, knl 1, cfg 1, lib 1\"\n\n")
	b.WriteString("conn omega-l2tp\n")
	b.WriteString("    auto=add\n")
	b.WriteString("    type=transport\n")
	b.WriteString("    keyexchange=ikev1\n")
	b.WriteString("    authby=secret\n")
	b.WriteString("    rekey=no\n")
	b.WriteString("    forceencaps=yes\n")
	b.WriteString("    dpdaction=clear\n")
	b.WriteString("    dpddelay=30s\n")
	b.WriteString("    dpdtimeout=120s\n")
	// Windows' built-in L2TP client defaults to IKEv1 with MODP1024 and
	// advertises AES256/AES128/3DES. Keep MODP2048 first for clients that
	// support it, but include the Windows-compatible fallbacks so the initial
	// IKE negotiation does not fail with error 789.
	b.WriteString("    ike=aes256-sha1-modp2048,aes256-sha1-modp1024,aes128-sha1-modp1024,3des-sha1-modp1024\n")
	b.WriteString("    esp=aes256-sha1,aes128-sha1,3des-sha1\n")
	b.WriteString("    left=%defaultroute\n")
	b.WriteString("    leftprotoport=17/1701\n")
	b.WriteString("    right=%any\n")
	b.WriteString("    rightprotoport=17/%any\n")
	return b.String()
}

func renderIPsecSecrets(inst Instance) string {
	return "# Managed by OMEGA. File mode must remain 0600.\n%any %any : PSK " + quoteIPsec(inst.PSK) + "\n"
}

func renderStrongSwanConf(inst Instance) string {
	var b strings.Builder
	b.WriteString("# Managed by OMEGA. Do not edit; changes are reconciled from the panel.\n")
	// The distro ipsec wrapper compiles /etc as its IPSEC_CONFDIR and resets
	// that environment variable before launching starter. Keep the connection
	// file explicit via starter --conf and redirect the stroke secrets loader
	// here, without copying the PSK into the global /etc/ipsec.secrets file.
	// These two strongSwan files intentionally live below /etc/strongswan.d:
	// Debian/Ubuntu's AppArmor profile permits charon to read that tree but
	// rejects the panel's /usr/local/x-ui/bin/l2tp path.
	b.WriteString("charon {\n")
	b.WriteString("    load_modular = yes\n")
	b.WriteString("    plugins {\n")
	b.WriteString("        include /etc/strongswan.d/charon/*.conf\n")
	b.WriteString("        stroke {\n")
	b.WriteString("            load = yes\n")
	fmt.Fprintf(&b, "            secrets_file = %s\n", ipsecSecretsPath(inst.Id))
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

func renderXL2TPDConf(inst Instance) string {
	var b strings.Builder
	// xl2tpd uses semicolons for comments; a leading '#' is parsed as data
	// before the first section by xl2tpd 1.3.x.
	b.WriteString("; Managed by OMEGA. Do not edit; changes are reconciled from the panel.\n")
	b.WriteString("[global]\n")
	// SAref is for the old MAST/SAref IPsec stack. Modern Linux XFRM with
	// strongSwan uses the normal kernel transport path, and explicitly
	// disabling SAref avoids xl2tpd probing an unavailable socket option.
	b.WriteString("ipsec saref = no\n")
	b.WriteString("port = 1701\n")
	b.WriteString("listen-addr = ")
	b.WriteString(inst.listenFor())
	b.WriteString("\n")
	b.WriteString("access control = no\n\n")
	b.WriteString("[lns default]\n")
	fmt.Fprintf(&b, "ip range = %s-%s\n", inst.poolStartFor(), inst.poolEndFor())
	fmt.Fprintf(&b, "local ip = %s\n", inst.localIPFor())
	b.WriteString("require authentication = yes\n")
	b.WriteString("require chap = yes\n")
	b.WriteString("refuse pap = yes\n")
	fmt.Fprintf(&b, "pppoptfile = %s\n", pppOptionsPath(inst.Id))
	b.WriteString("length bit = yes\n")
	return b.String()
}

func renderPPPOptions(inst Instance) string {
	var b strings.Builder
	b.WriteString("# pppd reads the OMEGA-managed block in /etc/ppp/chap-secrets.\n")
	b.WriteString("require-mschap-v2\n")
	b.WriteString("refuse-pap\n")
	b.WriteString("refuse-eap\n")
	b.WriteString("auth\n")
	b.WriteString("noccp\n")
	b.WriteString("mtu 1400\n")
	b.WriteString("mru 1400\n")
	b.WriteString("proxyarp\n")
	b.WriteString("nodefaultroute\n")
	b.WriteString("lcp-echo-interval 30\n")
	b.WriteString("lcp-echo-failure 4\n")
	fmt.Fprintf(&b, "ms-dns %s\n", inst.dns1For())
	fmt.Fprintf(&b, "ms-dns %s\n", inst.dns2For())
	fmt.Fprintf(&b, "ip-up-script %s\n", ipUpScriptPath(inst.Id))
	fmt.Fprintf(&b, "ip-down-script %s\n", ipDownScriptPath(inst.Id))
	return b.String()
}

func renderIPUpScript(inst Instance) string {
	// The directory contains only a numeric inbound id, so it is safe to embed
	// after shell-quoting. Runtime values are always quoted and interface names
	// are validated in traffic.go before they are used as path components.
	dir := strings.ReplaceAll(sessionDirPath(inst.Id), "'", "'\\''")
	return "#!/bin/sh\nset -eu\n" +
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin\nexport PATH\n" +
		"dir='" + dir + "'\n" +
		// pppd's documented interface name is $1/$IFNAME. PPP_IFACE is
		// exported by some distro wrapper scripts, but is not guaranteed when
		// pppd invokes an explicit ip-up-script directly.
		"iface=\"${PPP_IFACE:-${IFNAME:-${1:-}}}\"\n" +
		// PEERNAME is the authenticated PPP username and is the stable identity
		// that maps a session back to the panel client email.
		"peer=\"${PEERNAME:-${PPP_PEERNAME:-}}\"\n" +
		"case \"$iface\" in\n" +
		"  ''|*[!A-Za-z0-9_.-]*) exit 0 ;;\n" +
		"esac\n" +
		"[ -n \"$peer\" ] || exit 0\n" +
		"mkdir -p \"$dir\"\n" +
		"printf '%s\\n' \"$peer\" > \"$dir/$iface\"\n"
}

func renderIPDownScript(inst Instance) string {
	dir := strings.ReplaceAll(sessionDirPath(inst.Id), "'", "'\\''")
	return "#!/bin/sh\nset -eu\n" +
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin\nexport PATH\n" +
		"dir='" + dir + "'\n" +
		"iface=\"${PPP_IFACE:-${IFNAME:-${1:-}}}\"\n" +
		"case \"$iface\" in\n" +
		"  ''|*[!A-Za-z0-9_.-]*) exit 0 ;;\n" +
		"esac\n" +
		"rm -f \"$dir/$iface\"\n"
}

func renderChapSecrets(inst Instance) string {
	var b strings.Builder
	b.WriteString("# Managed by OMEGA. username server password addresses\n")
	for _, c := range inst.Credentials {
		fmt.Fprintf(&b, "%s * %s *\n", quotePPP(c.Email), quotePPP(c.Password))
	}
	return b.String()
}

// stripManagedChapSecrets removes every block previously written by OMEGA and
// leaves unrelated PPP credentials untouched. A trailing newline is returned
// for non-empty content so the next managed block cannot join another entry.
func stripManagedChapSecrets(contents string) string {
	lines := strings.Split(contents, "\n")
	kept := make([]string, 0, len(lines))
	inManaged := false
	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case omegaChapSecretsBegin:
			inManaged = true
			continue
		case omegaChapSecretsEnd:
			inManaged = false
			continue
		}
		if !inManaged {
			kept = append(kept, line)
		}
	}
	cleaned := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if cleaned == "" {
		return ""
	}
	return cleaned + "\n"
}

func renderManagedChapSecrets(inst Instance) string {
	return omegaChapSecretsBegin + "\n" +
		strings.TrimRight(renderChapSecrets(inst), "\n") + "\n" +
		omegaChapSecretsEnd + "\n"
}

func renderSystemChapSecrets(existing string, inst Instance) string {
	base := strings.TrimRight(stripManagedChapSecrets(existing), "\n")
	managed := strings.TrimRight(renderManagedChapSecrets(inst), "\n")
	if base == "" {
		return managed + "\n"
	}
	return base + "\n\n" + managed + "\n"
}

func writeSystemChapSecrets(contents string) error {
	dir := filepath.Dir(systemChapSecretsPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".omega-chap-secrets-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, systemChapSecretsPath); err != nil {
		return err
	}
	return nil
}

func reconcileSystemChapSecrets(inst Instance) error {
	existing, err := os.ReadFile(systemChapSecretsPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	contents := renderSystemChapSecrets(string(existing), inst)
	if contents == string(existing) {
		return nil
	}
	return writeSystemChapSecrets(contents)
}

func clearSystemChapSecrets() error {
	existing, err := os.ReadFile(systemChapSecretsPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cleaned := stripManagedChapSecrets(string(existing))
	if cleaned == string(existing) {
		return nil
	}
	// Keep the system file in place even when no unrelated entries remain. It
	// may have been installed by the PPP package or an administrator.
	return writeSystemChapSecrets(cleaned)
}

func writeConfig(inst Instance) error {
	if err := ValidateSettings(inst); err != nil {
		return err
	}
	dir := dataDirForID(inst.Id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(strongSwanDirForID(inst.Id), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(sessionDirPath(inst.Id), 0o750); err != nil {
		return err
	}
	files := []struct {
		path string
		data string
		mode os.FileMode
	}{
		{ipsecConfigPath(inst.Id), renderIPsecConf(inst), 0o640},
		{ipsecSecretsPath(inst.Id), renderIPsecSecrets(inst), 0o600},
		{strongSwanConfigPath(inst.Id), renderStrongSwanConf(inst), 0o640},
		{xl2tpdConfigPath(inst.Id), renderXL2TPDConf(inst), 0o640},
		{pppOptionsPath(inst.Id), renderPPPOptions(inst), 0o600},
		{chapSecretsPath(inst.Id), renderChapSecrets(inst), 0o600},
		{ipUpScriptPath(inst.Id), renderIPUpScript(inst), 0o750},
		{ipDownScriptPath(inst.Id), renderIPDownScript(inst), 0o750},
	}
	for _, file := range files {
		if err := os.WriteFile(file.path, []byte(file.data), file.mode); err != nil {
			return err
		}
		if err := os.Chmod(file.path, file.mode); err != nil {
			return err
		}
	}
	if err := reconcileSystemChapSecrets(inst); err != nil {
		return fmt.Errorf("reconcile system PPP chap-secrets: %w", err)
	}
	return nil
}
