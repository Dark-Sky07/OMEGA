package sub

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/tuic"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
)

// SubSingboxService generates sing-box subscription documents: a JSON object
// with a single `outbounds` array holding one sing-box outbound per
// (client × external proxy). Clients merge these outbounds into their own
// configuration, which keeps the output version-stable across sing-box
// releases (no opinionated dns/inbounds/route sections).
//
// The outbound shapes follow the sing-box outbound schema:
// vless / vmess / trojan / shadowsocks / hysteria2 / hysteria, with `tls`
// (incl. reality) and `transport` (ws / grpc / httpupgrade / http) objects
// derived from the inbound's Xray stream settings.
type SubSingboxService struct {
	SubService *SubService
}

// NewSubSingboxService creates a sing-box subscription service.
func NewSubSingboxService(subService *SubService) *SubSingboxService {
	return &SubSingboxService{SubService: subService}
}

// GetSingbox renders the sing-box outbounds document for a subscription.
// Returns the JSON body and the Subscription-Userinfo header value.
func (s *SubSingboxService) GetSingbox(subId string, host string) (string, string, error) {
	s.SubService.PrepareForRequest(host)
	inbounds, err := s.SubService.getInboundsBySubId(subId)
	if err != nil || len(inbounds) == 0 {
		return "", "", err
	}

	var outbounds []map[string]any
	seenEmails := make(map[string]struct{})
	seenTags := make(map[string]int)

	for _, inbound := range inbounds {
		clients := s.SubService.matchingClients(inbound, subId)
		if len(clients) == 0 {
			continue
		}
		s.SubService.projectThroughFallbackMaster(inbound)

		for _, client := range clients {
			seenEmails[client.Email] = struct{}{}
			for _, ob := range s.getOutbounds(inbound, client, host) {
				if tag, ok := ob["tag"].(string); ok {
					if n, dup := seenTags[tag]; dup {
						seenTags[tag] = n + 1
						ob["tag"] = fmt.Sprintf("%s %d", tag, n+1)
					} else {
						seenTags[tag] = 1
					}
				}
				outbounds = append(outbounds, ob)
			}
		}
	}

	if len(outbounds) == 0 {
		return "", "", nil
	}

	emails := make([]string, 0, len(seenEmails))
	for e := range seenEmails {
		emails = append(emails, e)
	}
	traffic, _ := s.SubService.AggregateTrafficByEmails(emails)
	header := fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", traffic.Up, traffic.Down, traffic.Total, traffic.ExpiryTime/1000)

	doc := map[string]any{"outbounds": outbounds}
	body, _ := json.MarshalIndent(doc, "", "  ")
	return string(body), header, nil
}

// getOutbounds builds the sing-box outbound(s) for one client on one inbound,
// honouring externalProxy expansion exactly like SubJsonService.getConfig.
func (s *SubSingboxService) getOutbounds(inbound *model.Inbound, client model.Client, host string) []map[string]any {
	stream := parseStreamSettings(inbound.StreamSettings)

	defaultDest := s.SubService.resolveInboundAddress(inbound)
	if defaultDest == "" {
		defaultDest = host
	}

	externalProxies, ok := stream["externalProxy"].([]any)
	hasExternalProxy := ok && len(externalProxies) > 0
	if !hasExternalProxy {
		externalProxies = []any{
			map[string]any{
				"forceTls": "same",
				"dest":     defaultDest,
				"port":     float64(inbound.Port),
				"remark":   "",
			},
		}
	}
	delete(stream, "externalProxy")

	result := make([]map[string]any, 0, len(externalProxies))
	for _, ep := range externalProxies {
		extPrxy, ok := ep.(map[string]any)
		if !ok {
			continue
		}
		dest, _ := extPrxy["dest"].(string)
		port := toInt(extPrxy["port"])
		remark, _ := extPrxy["remark"].(string)

		newStream := cloneStreamForExternalProxy(stream)
		forceTls, _ := extPrxy["forceTls"].(string)
		switch forceTls {
		case "tls":
			if newStream["security"] != "tls" {
				newStream["security"] = "tls"
				newStream["tlsSettings"] = map[string]any{}
			}
		case "none":
			if newStream["security"] != "none" {
				newStream["security"] = "none"
				delete(newStream, "tlsSettings")
			}
		}
		security, _ := newStream["security"].(string)
		if hasExternalProxy {
			applyExternalProxyTLSToStream(extPrxy, newStream, security)
			if inbound.Protocol == model.TUIC {
				newStream["tuicExternalProxy"] = extPrxy
			}
		}

		ob := s.genOutbound(inbound, client, newStream, dest, port)
		if ob == nil {
			continue
		}
		ob["tag"] = s.SubService.genRemark(inbound, client.Email, remark)
		result = append(result, ob)
	}
	return result
}

// genOutbound maps one (protocol, client, stream) to a sing-box outbound.
// Returns nil for protocols sing-box subscriptions cannot express.
func (s *SubSingboxService) genOutbound(inbound *model.Inbound, client model.Client, stream map[string]any, server string, port int) map[string]any {
	var settings map[string]any
	json.Unmarshal([]byte(inbound.Settings), &settings)

	out := map[string]any{
		"server":      server,
		"server_port": port,
	}

	switch inbound.Protocol {
	case model.VLESS:
		out["type"] = "vless"
		out["uuid"] = client.ID
		if client.Flow != "" {
			out["flow"] = client.Flow
		}
	case model.VMESS:
		out["type"] = "vmess"
		out["uuid"] = client.ID
		security := client.Security
		if security == "" {
			security = "auto"
		}
		out["security"] = security
		out["alter_id"] = 0
	case model.Trojan:
		out["type"] = "trojan"
		out["password"] = client.Password
	case model.Shadowsocks:
		out["type"] = "shadowsocks"
		method, _ := settings["method"].(string)
		password := client.Password
		if strings.HasPrefix(method, "2022") {
			if serverPassword, ok := settings["password"].(string); ok && serverPassword != "" {
				password = fmt.Sprintf("%s:%s", serverPassword, client.Password)
			}
		}
		out["method"] = method
		out["password"] = password
	case model.Hysteria:
		version := toInt(settings["version"])
		if version >= 2 {
			out["type"] = "hysteria2"
			out["password"] = client.Auth
		} else {
			out["type"] = "hysteria"
			out["auth_str"] = client.Auth
			if hyStream, ok := stream["hysteriaSettings"].(map[string]any); ok {
				if obfs, ok := hyStream["obfs"].(string); ok && obfs != "" {
					out["obfs"] = obfs
				}
			}
		}
	case model.TUIC:
		inst, ok := tuic.InstanceFromInbound(inbound)
		if !ok {
			return nil
		}
		uuid := client.ID
		password := client.Password
		for _, c := range inst.Clients {
			if c.Email == client.Email {
				if uuid == "" {
					uuid = c.UUID
				}
				if password == "" {
					password = c.Password
				}
				break
			}
		}
		if uuid == "" || password == "" {
			return nil
		}
		out["type"] = "tuic"
		out["uuid"] = uuid
		out["password"] = password
		if inst.CongestionControl != "" {
			out["congestion_control"] = inst.CongestionControl
		}
		if inst.UDPRelayMode != "" {
			out["udp_relay_mode"] = inst.UDPRelayMode
		}
		if inst.ZeroRTTHandshake {
			out["zero_rtt_handshake"] = true
		}
		tlsObj := map[string]any{
			"enabled": true,
		}
		if inst.SNI != "" {
			tlsObj["server_name"] = inst.SNI
		}
		if len(inst.ALPN) > 0 {
			tlsObj["alpn"] = inst.ALPN
		}
		if ep, ok := stream["tuicExternalProxy"].(map[string]any); ok {
			if sni, ok := externalProxySNI(ep); ok {
				tlsObj["server_name"] = sni
			}
			if alpn, ok := externalProxyALPN(ep["alpn"]); ok {
				tlsObj["alpn"] = strings.Split(alpn, ",")
			}
			if ai, ok := ep["allowInsecure"].(bool); ok && ai {
				tlsObj["insecure"] = true
			}
		}
		out["tls"] = tlsObj
		return out
	default:
		return nil
	}

	// TLS / reality
	network, _ := stream["network"].(string)
	security, _ := stream["security"].(string)
	isHysteria := inbound.Protocol == model.Hysteria
	if isHysteria || security == "tls" || security == "reality" {
		if tlsObj := singboxTLS(stream, security, isHysteria); tlsObj != nil {
			out["tls"] = tlsObj
		}
	}

	// Transport (not applicable to hysteria or plain shadowsocks-with-plugin)
	if !isHysteria {
		if tr := singboxTransport(stream, network); tr != nil {
			out["transport"] = tr
		}
	}
	return out
}

// singboxTLS builds the sing-box `tls` object from Xray stream settings.
func singboxTLS(stream map[string]any, security string, force bool) map[string]any {
	tlsObj := map[string]any{"enabled": true}

	if security == "reality" {
		rData, _ := stream["realitySettings"].(map[string]any)
		if rData == nil {
			return nil
		}
		rSettings, _ := rData["settings"].(map[string]any)
		serverName := firstString(rData, "serverNames")
		shortId := firstString(rData, "shortIds")
		tlsObj["server_name"] = serverName
		reality := map[string]any{
			"enabled":    true,
			"public_key": rSettings["publicKey"],
		}
		if shortId != "" {
			reality["short_id"] = shortId
		}
		tlsObj["reality"] = reality
		fp := stringValue(rSettings["fingerprint"])
		if fp == "" {
			fp = "chrome"
		}
		tlsObj["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		return tlsObj
	}

	// Plain TLS. Hysteria always speaks TLS.
	tData, _ := stream["tlsSettings"].(map[string]any)
	if tData == nil {
		if !force {
			return nil
		}
		tData = map[string]any{}
	}
	if sni := stringValue(tData["serverName"]); sni != "" {
		tlsObj["server_name"] = sni
	}
	if alpn, ok := tData["alpn"].([]any); ok && len(alpn) > 0 {
		tlsObj["alpn"] = alpn
	}
	tSettings, _ := tData["settings"].(map[string]any)
	if tSettings != nil {
		if fp := stringValue(tSettings["fingerprint"]); fp != "" {
			tlsObj["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		if insecure, ok := tSettings["allowInsecure"].(bool); ok && insecure {
			tlsObj["insecure"] = true
		}
		if echList := stringValue(tSettings["echConfigList"]); echList != "" {
			tlsObj["ech"] = map[string]any{"enabled": true, "config": echList}
		}
	}
	return tlsObj
}

// singboxTransport builds the sing-box `transport` object for the network.
func singboxTransport(stream map[string]any, network string) map[string]any {
	switch network {
	case "ws":
		ws, _ := stream["wsSettings"].(map[string]any)
		if ws == nil {
			return nil
		}
		tr := map[string]any{"type": "ws"}
		if path := stringValue(ws["path"]); path != "" {
			tr["path"] = path
		}
		if headers, ok := ws["headers"].(map[string]any); ok && len(headers) > 0 {
			tr["headers"] = headers
		}
		return tr
	case "grpc":
		g, _ := stream["grpcSettings"].(map[string]any)
		if g == nil {
			return nil
		}
		tr := map[string]any{"type": "grpc"}
		if name := stringValue(g["serviceName"]); name != "" {
			tr["service_name"] = name
		}
		if multi, ok := g["multiMode"].(bool); ok && multi {
			tr["multi_mode"] = true
		}
		return tr
	case "httpupgrade":
		hu, _ := stream["httpupgradeSettings"].(map[string]any)
		if hu == nil {
			return nil
		}
		tr := map[string]any{"type": "httpupgrade"}
		if path := stringValue(hu["path"]); path != "" {
			tr["path"] = path
		}
		if host := stringValue(hu["host"]); host != "" {
			tr["host"] = host
		}
		return tr
	case "http":
		h, _ := stream["httpSettings"].(map[string]any)
		if h == nil {
			return nil
		}
		tr := map[string]any{"type": "http"}
		if path := stringValue(h["path"]); path != "" {
			tr["path"] = path
		}
		if host := stringValue(h["host"]); host != "" {
			tr["host"] = []string{host}
		}
		return tr
	case "tcp":
		// HTTP header obfuscation over TCP is an HTTP transport in sing-box.
		tcp, _ := stream["tcpSettings"].(map[string]any)
		if tcp == nil {
			return nil
		}
		header, _ := tcp["header"].(map[string]any)
		headerType, _ := header["type"].(string)
		if headerType != "http" {
			return nil
		}
		tr := map[string]any{"type": "http"}
		req, _ := header["request"].(map[string]any)
		if req != nil {
			if pathAny, ok := req["path"]; ok {
				switch p := pathAny.(type) {
				case string:
					if p != "" {
						tr["path"] = p
					}
				case []any:
					if len(p) > 0 {
						tr["path"] = p[0]
					}
				}
			}
			if hostAny, ok := req["headers"].(map[string]any); ok {
				if host, ok := hostAny["Host"]; ok {
					switch h := host.(type) {
					case string:
						if h != "" {
							tr["host"] = []string{h}
						}
					case []any:
						if len(h) > 0 {
							tr["host"] = h
						}
					}
				}
			}
		}
		return tr
	}
	return nil
}

// parseStreamSettings unmarshals an inbound's stream settings JSON.
func parseStreamSettings(raw string) map[string]any {
	stream := make(map[string]any)
	if raw != "" {
		json.Unmarshal([]byte(raw), &stream)
	}
	return stream
}

// firstString picks a random entry from the named array setting (serverNames,
// shortIds) — mirroring SubJsonService.realityData's random selection.
func firstString(data map[string]any, key string) string {
	arr, ok := data[key].([]any)
	if !ok || len(arr) == 0 {
		return ""
	}
	return stringValue(arr[random.Num(len(arr))])
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}
