package sub

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/amneziawg"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// genAmneziaWGLink renders the same client profile as the frontend's
// genAmneziaWGConfig and wraps it in AmneziaVPN's vpn:// base64url scheme.
// Keeping this server-side link in the subscription path is important: native
// AmneziaWG peers do not have an Xray share URL, but they still need the same
// client lifecycle as every other inbound.
func (s *SubService) genAmneziaWGLink(inbound *model.Inbound, email string) string {
	if inbound == nil || inbound.Protocol != model.AmneziaWG {
		return ""
	}
	var settings amneziawg.InboundSettings
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil || settings.Server == nil {
		return ""
	}
	var client *model.Client
	for i := range settings.Clients {
		if settings.Clients[i].Email == email {
			client = &settings.Clients[i]
			break
		}
	}
	if client == nil {
		return ""
	}
	conf := amneziaWGConfigText(settings.Server, client, s.resolveInboundAddress(inbound), inbound.Port, inbound.Remark)
	if conf == "" {
		return ""
	}
	return "vpn://" + base64.RawURLEncoding.EncodeToString([]byte(conf))
}

// amneziaWGConfigText is the canonical backend AmneziaWG client-config
// emitter. Its field order and omission rules intentionally mirror the two
// frontend emitters so a downloaded profile and a vpn:// link are
// interchangeable. It returns an empty string for values that could inject
// additional config lines or for incomplete cryptographic material.
func amneziaWGConfigText(server *amneziawg.ServerSettings, client *model.Client, address string, port int, remark string) string {
	if server == nil || client == nil || strings.TrimSpace(address) == "" || port <= 0 {
		return ""
	}
	if !validWireguardKey(client.PrivateKey) || !validWireguardKey(server.PublicKey) {
		return ""
	}
	for _, value := range []string{
		client.PrivateKey,
		server.PublicKey,
		server.PrimaryDNS,
		server.SecondaryDNS,
		server.ExternalInterface,
		server.IPv6ExternalInterface,
		server.HeaderProtectionKey,
		server.ContentPaddingAddition,
		server.RekeyAfterTime,
		server.RekeyTimeout,
		server.RejectAfterTime,
		server.KeepaliveTimeout,
		server.MaxHandshakeAttempts,
		server.H1, server.H2, server.H3, server.H4,
		server.I1, server.I2, server.I3, server.I4, server.I5,
		remark,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return ""
		}
	}
	if len(client.AllowedIPs) == 0 || strings.ContainsAny(strings.Join(client.AllowedIPs, ", "), "\r\n") {
		return ""
	}

	var b strings.Builder
	b.WriteString("[Interface]\n")
	writeKV := func(key, value string) {
		b.WriteString(key)
		b.WriteString(" = ")
		b.WriteString(value)
		b.WriteByte('\n')
	}
	writeKV("PrivateKey", client.PrivateKey)
	writeKV("Address", strings.Join(client.AllowedIPs, ", "))
	if dns := nonEmptyCommaSeparated(server.PrimaryDNS, server.SecondaryDNS); dns != "" {
		writeKV("DNS", dns)
	}
	if server.MTU > 0 {
		writeKV("MTU", itoa(server.MTU))
	}
	writeKV("Jc", itoa(server.Jc))
	writeKV("Jmin", itoa(server.Jmin))
	writeKV("Jmax", itoa(server.Jmax))
	writeKV("S1", itoa(server.S1))
	writeKV("S2", itoa(server.S2))
	if server.S3 != 0 {
		writeKV("S3", itoa(server.S3))
	}
	if server.S4 != 0 {
		writeKV("S4", itoa(server.S4))
	}
	writeKV("H1", hOrDefault(server.H1, "1"))
	writeKV("H2", hOrDefault(server.H2, "2"))
	writeKV("H3", hOrDefault(server.H3, "3"))
	writeKV("H4", hOrDefault(server.H4, "4"))
	for _, field := range []struct{ key, value string }{
		{"I1", server.I1}, {"I2", server.I2}, {"I3", server.I3},
		{"I4", server.I4}, {"I5", server.I5},
		{"HeaderProtectionKey", server.HeaderProtectionKey},
		{"ContentPaddingAddition", server.ContentPaddingAddition},
		{"RekeyAfterTime", server.RekeyAfterTime},
		{"RekeyTimeout", server.RekeyTimeout},
		{"RejectAfterTime", server.RejectAfterTime},
		{"KeepaliveTimeout", server.KeepaliveTimeout},
		{"MaxHandshakeAttempts", server.MaxHandshakeAttempts},
	} {
		if strings.TrimSpace(field.value) != "" {
			writeKV(field.key, field.value)
		}
	}
	if server.RandomTrailers {
		writeKV("RandomTrailers", "on")
	}
	if server.DisableCookies {
		writeKV("DisableCookies", "on")
	}

	b.WriteByte('\n')
	b.WriteString("# ")
	b.WriteString(remark)
	b.WriteString("\n[Peer]\n")
	writeKV("PublicKey", server.PublicKey)
	if client.PreSharedKey != "" {
		if strings.ContainsAny(client.PreSharedKey, "\r\n") || !validWireguardKey(client.PreSharedKey) {
			return ""
		}
		writeKV("PresharedKey", client.PreSharedKey)
	}
	writeKV("AllowedIPs", "0.0.0.0/0, ::/0")
	writeKV("Endpoint", address+":"+itoa(port))
	if client.KeepAlive > 0 {
		writeKV("PersistentKeepalive", itoa(client.KeepAlive))
	}
	// writeKV always adds a newline; the frontend emitters deliberately omit
	// the final newline, so trim exactly that one byte for parity.
	return strings.TrimSuffix(b.String(), "\n")
}

func validWireguardKey(value string) bool {
	decoded, err := base64.StdEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func hOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func nonEmptyCommaSeparated(values ...string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return strings.Join(out, ", ")
}

func itoa(value int) string {
	// strconv.Itoa is kept behind this tiny local helper to make the emitter's
	// formatting call sites easy to compare with the frontend's template lines.
	return strconv.Itoa(value)
}
