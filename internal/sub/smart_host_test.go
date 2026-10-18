package sub

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestSmartHostWildcardAndTemplates(t *testing.T) {
	ib := &model.Inbound{
		Protocol: model.VLESS,
		Port:     443,
		Remark:   "DE-Edge",
	}
	rawList := []any{
		map[string]any{
			"forceTls": "tls",
			"dest":     "*.cdn.example.com",
			"port":     float64(2053),
			"sni":      "*.sni.example.com",
			"remark":   "{REMARK}-{USERNAME}-{PROTOCOL}",
		},
	}
	resolved := resolveSmartExternalProxyList(rawList, ib, "alice")
	if len(resolved) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resolved))
	}
	ep := resolved[0].(map[string]any)
	dest := ep["dest"].(string)
	sni := ep["sni"].(string)
	remark := ep["remark"].(string)

	wildcardRe := regexp.MustCompile(`^[a-z0-9]{6}\.cdn\.example\.com$`)
	if !wildcardRe.MatchString(dest) {
		t.Fatalf("expected wildcard dest match, got %q", dest)
	}
	sniRe := regexp.MustCompile(`^[a-z0-9]{6}\.sni\.example\.com$`)
	if !sniRe.MatchString(sni) {
		t.Fatalf("expected wildcard sni match, got %q", sni)
	}
	if remark != "DE-Edge-alice-vless" {
		t.Fatalf("unexpected remark %q", remark)
	}
}

func TestSmartHostMultiValuePoolPairedIndex(t *testing.T) {
	ib := &model.Inbound{
		Protocol: model.VLESS,
		Port:     443,
		Remark:   "Pool",
	}
	rawList := []any{
		map[string]any{
			"forceTls": "tls",
			"dest":     "edge1.example.com, edge2.example.com, edge3.example.com",
			"port":     float64(443),
			"sni":      "sni1.example.com, sni2.example.com, sni3.example.com",
			"remark":   "",
		},
	}
	for i := 0; i < 20; i++ {
		resolved := resolveSmartExternalProxyList(rawList, ib, "bob")
		ep := resolved[0].(map[string]any)
		dest := ep["dest"].(string)
		sni := ep["sni"].(string)
		switch dest {
		case "edge1.example.com":
			if sni != "sni1.example.com" {
				t.Fatalf("mismatched pair: dest=%q sni=%q", dest, sni)
			}
		case "edge2.example.com":
			if sni != "sni2.example.com" {
				t.Fatalf("mismatched pair: dest=%q sni=%q", dest, sni)
			}
		case "edge3.example.com":
			if sni != "sni3.example.com" {
				t.Fatalf("mismatched pair: dest=%q sni=%q", dest, sni)
			}
		default:
			t.Fatalf("unexpected dest from pool: %q", dest)
		}
	}
}

func TestSmartHostGetLinkIntegration(t *testing.T) {
	ib := &model.Inbound{
		Protocol: model.VLESS,
		Port:     443,
		Remark:   "VIP",
		Settings: `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"user1","enable":true}],"decryption":"none"}`,
		StreamSettings: `{
			"network":"ws",
			"security":"tls",
			"wsSettings":{"path":"/ws"},
			"tlsSettings":{"serverName":"origin.example.com"},
			"externalProxy":[
				{"forceTls":"tls","dest":"*.edge.example.com","port":2053,"sni":"{USERNAME}.sni.example.com","remark":"{REMARK}-{USERNAME}"}
			]
		}`,
	}
	svc := &SubService{showInfo: false, remarkModel: "-ieo"}
	link := svc.GetLink(ib, "user1")
	if !strings.HasPrefix(link, "vless://11111111-1111-1111-1111-111111111111@") {
		t.Fatalf("unexpected link prefix: %s", link)
	}
	if strings.Contains(link, "*.edge.example.com") {
		t.Fatalf("wildcard was not resolved in link: %s", link)
	}
	if !strings.Contains(link, ".edge.example.com:2053") {
		t.Fatalf("expected resolved .edge.example.com:2053 in link: %s", link)
	}
	if !strings.Contains(link, "sni=user1.sni.example.com") {
		t.Fatalf("expected resolved sni=user1.sni.example.com in link: %s", link)
	}
}
