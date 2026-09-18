package l2tp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
)

func l2tpRoot() string {
	dir := config.GetBinFolderPath()
	return filepath.Join(dir, "l2tp")
}

func dataDirForID(id int) string {
	return filepath.Join(l2tpRoot(), fmt.Sprintf("%d", id))
}

func ipsecConfigPath(id int) string    { return filepath.Join(dataDirForID(id), "ipsec.conf") }
func ipsecSecretsPath(id int) string   { return filepath.Join(dataDirForID(id), "ipsec.secrets") }
func xl2tpdConfigPath(id int) string   { return filepath.Join(dataDirForID(id), "xl2tpd.conf") }
func pppOptionsPath(id int) string     { return filepath.Join(dataDirForID(id), "options.xl2tpd") }
func chapSecretsPath(id int) string   { return filepath.Join(dataDirForID(id), "chap-secrets") }
func xl2tpdPIDPath(id int) string     { return filepath.Join(dataDirForID(id), "xl2tpd.pid") }
func sessionDirPath(id int) string    { return filepath.Join(dataDirForID(id), "sessions") }
func ipUpScriptPath(id int) string    { return filepath.Join(dataDirForID(id), "ip-up") }
func ipDownScriptPath(id int) string  { return filepath.Join(dataDirForID(id), "ip-down") }
func strongSwanPIDDir(id int) string  { return dataDirForID(id) }

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
	b.WriteString("    charondebug=\"ike 1, knl 1, cfg 0\"\n\n")
	b.WriteString("conn omega-l2tp\n")
	b.WriteString("    auto=add\n")
	b.WriteString("    type=transport\n")
	b.WriteString("    keyexchange=ikev1\n")
	b.WriteString("    authby=secret\n")
	b.WriteString("    pfs=no\n")
	b.WriteString("    rekey=no\n")
	b.WriteString("    forceencaps=yes\n")
	b.WriteString("    dpdaction=clear\n")
	b.WriteString("    dpddelay=30s\n")
	b.WriteString("    dpdtimeout=120s\n")
	b.WriteString("    ike=aes256-sha1-modp2048,aes128-sha1-modp1024\n")
	b.WriteString("    esp=aes256-sha1\n")
	b.WriteString("    left=%defaultroute\n")
	b.WriteString("    leftprotoport=17/1701\n")
	b.WriteString("    right=%any\n")
	b.WriteString("    rightprotoport=17/%any\n")
	return b.String()
}

func renderIPsecSecrets(inst Instance) string {
	return "# Managed by OMEGA. File mode must remain 0600.\n%any %any : PSK " + quoteIPsec(inst.PSK) + "\n"
}

func renderXL2TPDConf(inst Instance) string {
	var b strings.Builder
	b.WriteString("# Managed by OMEGA. Do not edit; changes are reconciled from the panel.\n")
	b.WriteString("[global]\n")
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
	b.WriteString("# Managed by OMEGA. PPP credentials are in the adjacent chap-secrets file.\n")
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
	fmt.Fprintf(&b, "chap-secrets %s\n", chapSecretsPath(inst.Id))
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
		"dir='" + dir + "'\n" +
		"iface=\"${PPP_IFACE:-}\"\n" +
		"peer=\"${PEERNAME:-}\"\n" +
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
		"dir='" + dir + "'\n" +
		"iface=\"${PPP_IFACE:-}\"\n" +
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

func writeConfig(inst Instance) error {
	if err := ValidateSettings(inst); err != nil {
		return err
	}
	dir := dataDirForID(inst.Id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
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
	return nil
}
