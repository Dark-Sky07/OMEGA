package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

// seedSubClient links a client row to an inbound the way the production
// Create path does (canonical clients + client_inbounds rows), so
// getInboundsBySubId resolves the subscription through the same joins.
// Passing rec.Id > 0 reuses the existing client row (multi-inbound client).
func seedSubClient(t *testing.T, ib *model.Inbound, rec *model.ClientRecord) {
	t.Helper()
	db := database.GetDB()
	if rec.Id == 0 {
		if err := db.Create(rec).Error; err != nil {
			t.Fatalf("create client %s: %v", rec.Email, err)
		}
	}
	assoc := model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id}
	if err := db.Create(&assoc).Error; err != nil {
		t.Fatalf("create association %s: %v", rec.Email, err)
	}
}

// seedSubInbounds creates a vless-ws-tls inbound plus a shadowsocks inbound
// that both serve the given subId.
func seedSubInbounds(t *testing.T, subId string) {
	t.Helper()
	db := database.GetDB()
	rec := &model.ClientRecord{Email: "u1@example.com", SubID: subId}
	mk := func(tag string, port int, protocol model.Protocol, settings string, stream string) {
		ib := &model.Inbound{Tag: tag, Enable: true, Port: port, Protocol: protocol,
			Settings: settings, StreamSettings: stream}
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("create inbound %s: %v", tag, err)
		}
		seedSubClient(t, ib, rec)
	}
	vlessClients := `{"clients":[{"id":"27848739-7e62-4138-9fd3-098a63964b6b","flow":"xtls-rprx-vision","email":"u1@example.com","enable":true,"expiryTime":0,"subId":"` + subId + `"}]}`
	ssSettings := `{"method":"aes-128-gcm","password":"server-pass","clients":[{"password":"cli-pass","email":"u1@example.com","enable":true,"expiryTime":0,"subId":"` + subId + `"}]}`
	mk("vless-ws", 443, model.VLESS, vlessClients,
		`{"network":"ws","security":"tls","wsSettings":{"path":"/ws","headers":{"Host":"cdn.example.com"}},"tlsSettings":{"serverName":"sni.example.com","alpn":["h2","http/1.1"],"settings":{"fingerprint":"chrome","allowInsecure":true}}}`)
	mk("ss-plain", 8388, model.Shadowsocks, ssSettings,
		`{"network":"tcp","security":"none"}`)
}

// The sing-box subscription emits one outbound per (client × inbound) inside
// a single {"outbounds": [...]} document, with tls/transport mapped from the
// inbound's Xray stream settings.
func TestGetSingbox_OutboundsDocument(t *testing.T) {
	initSubDB(t)
	seedSubInbounds(t, "sub-sb-1")

	svc := NewSubSingboxService(NewSubService(true, "-io"))
	svc.SubService.PrepareForRequest("sub.example.com")

	body, header, err := svc.GetSingbox("sub-sb-1", "sub.example.com")
	if err != nil {
		t.Fatalf("GetSingbox: %v", err)
	}
	if !strings.Contains(header, "upload=") || !strings.Contains(header, "expire=") {
		t.Fatalf("missing traffic header: %q", header)
	}

	var doc struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	if len(doc.Outbounds) != 2 {
		t.Fatalf("want 2 outbounds (one per inbound), got %d\n%s", len(doc.Outbounds), body)
	}

	var vless, ss map[string]any
	for _, ob := range doc.Outbounds {
		switch ob["type"] {
		case "vless":
			vless = ob
		case "shadowsocks":
			ss = ob
		}
	}
	if vless == nil {
		t.Fatalf("no vless outbound:\n%s", body)
	}
	if vless["uuid"] != "27848739-7e62-4138-9fd3-098a63964b6b" {
		t.Fatalf("vless uuid = %v", vless["uuid"])
	}
	if vless["flow"] != "xtls-rprx-vision" {
		t.Fatalf("vless flow = %v", vless["flow"])
	}
	if vless["server"] == "" || vless["server_port"] == nil {
		t.Fatalf("vless server/port missing: %v", vless)
	}

	tlsObj, _ := vless["tls"].(map[string]any)
	if tlsObj == nil || tlsObj["enabled"] != true {
		t.Fatalf("vless tls missing: %v", vless["tls"])
	}
	if tlsObj["server_name"] != "sni.example.com" {
		t.Fatalf("tls server_name = %v", tlsObj["server_name"])
	}
	if tlsObj["insecure"] != true {
		t.Fatalf("allowInsecure must map to tls.insecure: %v", tlsObj)
	}
	utls, _ := tlsObj["utls"].(map[string]any)
	if utls == nil || utls["fingerprint"] != "chrome" {
		t.Fatalf("tls utls = %v", tlsObj["utls"])
	}

	tr, _ := vless["transport"].(map[string]any)
	if tr == nil || tr["type"] != "ws" {
		t.Fatalf("vless transport = %v", vless["transport"])
	}
	if tr["path"] != "/ws" {
		t.Fatalf("transport path = %v", tr["path"])
	}

	if ss == nil {
		t.Fatalf("no shadowsocks outbound:\n%s", body)
	}
	if ss["method"] != "aes-128-gcm" {
		t.Fatalf("ss method = %v", ss["method"])
	}
	if ss["password"] != "cli-pass" {
		t.Fatalf("ss password = %v", ss["password"])
	}
}

// Reality inbounds map to sing-box tls.reality + utls, with server_name and
// short_id drawn from the inbound's reality settings.
func TestGetSingbox_RealityMapping(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	settings := `{"clients":[{"id":"27848739-7e62-4138-9fd3-098a63964b6b","flow":"","email":"u1@example.com","enable":true,"expiryTime":0,"subId":"sub-sb-2"}],"decryption":"none"}`
	stream := `{"network":"tcp","security":"reality","realitySettings":{"dest":"example.com:443","serverNames":["example.com"],"shortIds":["1a2b3c4d"],"settings":{"publicKey":"pbk-value","fingerprint":"firefox"}}}`
	ib := &model.Inbound{Tag: "vless-reality", Enable: true, Port: 443, Protocol: model.VLESS,
		Settings: settings, StreamSettings: stream}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	seedSubClient(t, ib, &model.ClientRecord{Email: "u1@example.com", SubID: "sub-sb-2"})

	svc := NewSubSingboxService(NewSubService(true, "-io"))
	body, _, err := svc.GetSingbox("sub-sb-2", "sub.example.com")
	if err != nil {
		t.Fatalf("GetSingbox: %v", err)
	}
	var doc struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil || len(doc.Outbounds) != 1 {
		t.Fatalf("bad doc: %v\n%s", err, body)
	}
	tlsObj, _ := doc.Outbounds[0]["tls"].(map[string]any)
	if tlsObj == nil {
		t.Fatalf("tls missing:\n%s", body)
	}
	reality, _ := tlsObj["reality"].(map[string]any)
	if reality == nil || reality["enabled"] != true || reality["public_key"] != "pbk-value" {
		t.Fatalf("reality = %v", tlsObj["reality"])
	}
	if reality["short_id"] != "1a2b3c4d" {
		t.Fatalf("short_id = %v", reality["short_id"])
	}
	if tlsObj["server_name"] != "example.com" {
		t.Fatalf("server_name = %v", tlsObj["server_name"])
	}
	utls, _ := tlsObj["utls"].(map[string]any)
	if utls == nil || utls["fingerprint"] != "firefox" {
		t.Fatalf("utls = %v", tlsObj["utls"])
	}
}

// The Outline subscription is a plain-text list of SIP002 ss:// keys —
// non-SS entries of the same subscription are dropped, not emitted broken.
func TestGetOutline_FiltersToSS(t *testing.T) {
	initSubDB(t)
	seedSubInbounds(t, "sub-ol-1")

	svc := NewSubOutlineService(NewSubService(true, "-io"))
	body, header, err := svc.GetOutline("sub-ol-1", "sub.example.com")
	if err != nil {
		t.Fatalf("GetOutline: %v", err)
	}
	if !strings.Contains(header, "upload=") {
		t.Fatalf("missing traffic header: %q", header)
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly the ss:// line, got %d:\n%s", len(lines), body)
	}
	if !strings.HasPrefix(lines[0], "ss://") {
		t.Fatalf("line = %q, want ss:// prefix", lines[0])
	}
	if strings.Contains(body, "vless://") || strings.Contains(body, "trojan://") {
		t.Fatalf("non-SS links must be filtered out:\n%s", body)
	}

	// An unknown subId yields an empty body (the controller maps it to 404).
	body, _, err = svc.GetOutline("nope", "sub.example.com")
	if err != nil {
		t.Fatalf("unknown subId: %v", err)
	}
	if body != "" {
		t.Fatalf("unknown subId must yield empty body, got %q", body)
	}
}
