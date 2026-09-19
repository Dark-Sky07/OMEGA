package l2tp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func validInstance() Instance {
	return Instance{
		Id:       7,
		Tag:      "in-1701",
		Port:     DefaultPort,
		PSK:      "0123456789abcdef0123456789abcdef",
		PoolCIDR: DefaultPoolCIDR,
		LocalIP:  DefaultLocalIP,
		PoolStart: DefaultPoolStart,
		PoolEnd:   DefaultPoolEnd,
		DNS1:      DefaultDNS1,
		DNS2:      DefaultDNS2,
		Credentials: []Credential{{
			Email:    "alice@example.com",
			Password: "correct horse battery staple",
		}},
	}
}

func TestValidateSettings(t *testing.T) {
	if err := ValidateSettings(validInstance()); err != nil {
		t.Fatalf("valid L2TP settings rejected: %v", err)
	}

	cases := []struct {
		name string
		edit func(*Instance)
	}{
		{"wrong port", func(inst *Instance) { inst.Port = 500 }},
		{"IPv6 pool", func(inst *Instance) { inst.PoolCIDR = "2001:db8::/64" }},
		{"range outside pool", func(inst *Instance) { inst.PoolStart = "192.0.2.10" }},
		{"bad DNS", func(inst *Instance) { inst.DNS1 = "not-an-ip" }},
		{"unsafe interface", func(inst *Instance) { inst.OutboundInterface = "eth0;touch" }},
		{"unsafe password", func(inst *Instance) { inst.Credentials[0].Password = "bad\nsecret" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst := validInstance()
			tc.edit(&inst)
			if err := ValidateSettings(inst); err == nil {
				t.Fatal("expected invalid settings to be rejected")
			}
		})
	}
}

func TestInstanceFromInboundUsesStoredClients(t *testing.T) {
	stored := map[string]any{
		"psk":       "0123456789abcdef0123456789abcdef",
		"poolCIDR":  DefaultPoolCIDR,
		"localIP":   DefaultLocalIP,
		"poolStart": DefaultPoolStart,
		"poolEnd":   DefaultPoolEnd,
		"dns1":      DefaultDNS1,
		"dns2":      DefaultDNS2,
		"clients": []model.Client{
			{Email: "enabled@example.com", Password: "one", Enable: true},
			{Email: "disabled@example.com", Password: "two", Enable: false},
		},
	}
	settings, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	ib := &model.Inbound{Id: 4, Tag: "l2tp-4", Protocol: model.L2TP, Port: DefaultPort, Settings: string(settings)}
	inst, ok := InstanceFromInbound(ib, nil)
	if !ok {
		t.Fatal("expected stored L2TP settings to produce an instance")
	}
	if len(inst.Credentials) != 1 || inst.Credentials[0].Email != "enabled@example.com" {
		t.Fatalf("unexpected credentials: %#v", inst.Credentials)
	}
}

func TestRenderConfigQuotesCredentials(t *testing.T) {
	inst := validInstance()
	inst.Credentials[0].Password = `pa ss "word"`
	chap := renderChapSecrets(inst)
	if !strings.Contains(chap, `\"word\"`) || !strings.Contains(chap, `"pa ss`) {
		t.Fatalf("password was not quoted in chap-secrets: %q", chap)
	}
	if !strings.Contains(renderPPPOptions(inst), "ip-up-script") {
		t.Fatal("PPP options do not install accounting hooks")
	}
	strongSwan := renderStrongSwanConf(inst)
	if !strings.Contains(strongSwan, "include /etc/strongswan.d/charon/*.conf") || !strings.Contains(strongSwan, "secrets_file = "+ipsecSecretsPath(inst.Id)) {
		t.Fatalf("strongSwan runtime config does not load modular plugins and redirect secrets: %s", strongSwan)
	}
	if !strings.Contains(renderIPUpScript(inst), "PEERNAME") || !strings.Contains(renderIPDownScript(inst), "PPP_IFACE") {
		t.Fatal("PPP accounting hooks do not reference session environment")
	}
}

func TestRenderIPsecConfigSupportsWindowsL2TP(t *testing.T) {
	conf := renderIPsecConf(validInstance())
	for _, expected := range []string{
		"keyexchange=ikev1",
		"ike=aes256-sha1-modp2048,aes256-sha1-modp1024,aes128-sha1-modp1024,3des-sha1-modp1024",
		"esp=aes256-sha1,aes128-sha1,3des-sha1",
		"leftprotoport=17/1701",
		"rightprotoport=17/%any",
	} {
		if !strings.Contains(conf, expected) {
			t.Fatalf("Windows-compatible IPsec setting %q is missing from config: %s", expected, conf)
		}
	}
	if strings.Contains(conf, "pfs=") {
		t.Fatal("strongSwan 5.9.x must not emit deprecated pfs keyword")
	}
	xl2tpd := renderXL2TPDConf(validInstance())
	if !strings.Contains(xl2tpd, "ipsec saref = no") {
		t.Fatal("xl2tpd must disable legacy SAref probing on kernel XFRM")
	}
	if strings.HasPrefix(xl2tpd, "#") {
		t.Fatal("xl2tpd config must use semicolon comments before its first section")
	}
}

func TestProcessEnvironmentReplacesInheritedValues(t *testing.T) {
	t.Setenv("OMEGA_L2TP_TEST_ENV", "old")
	env := processEnvironment("OMEGA_L2TP_TEST_ENV=new")
	matches := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, "OMEGA_L2TP_TEST_ENV=") {
			matches++
			if entry != "OMEGA_L2TP_TEST_ENV=new" {
				t.Fatalf("unexpected override value: %q", entry)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("expected one overridden environment entry, got %d", matches)
	}
}

func TestESPInputRule(t *testing.T) {
	got := strings.Join(espInputRule(), " ")
	if got != "-p esp -j ACCEPT" {
		t.Fatalf("unexpected ESP firewall rule: %q", got)
	}
}
