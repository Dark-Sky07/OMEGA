package sub

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// TestGetOpenvpnEmailsBySubId covers the sub-page lookup behind the .ovpn
// download row: it must return exactly the clients that hold this subId and
// are attached to an enabled, local (non-node) openvpn inbound.
func TestGetOpenvpnEmailsBySubId(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	const subId = "sub-ovpn-1"

	openEnabled := &model.Inbound{Tag: "ovpn-on", Enable: true, Port: 1194, Protocol: model.OpenVPN, Settings: `{"proto":"udp"}`}
	openDisabled := &model.Inbound{Tag: "ovpn-off", Enable: false, Port: 1195, Protocol: model.OpenVPN, Settings: `{"proto":"udp"}`}
	vless := &model.Inbound{Tag: "vless-in", Enable: true, Port: 443, Protocol: model.VLESS, Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}
	for _, ib := range []*model.Inbound{openEnabled, openDisabled, vless} {
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("seed inbound %s: %v", ib.Tag, err)
		}
	}

	attached := &model.ClientRecord{Email: "has-ovpn@example.com", SubID: subId, Enable: true}
	onDisabledInbound := &model.ClientRecord{Email: "off-ovpn@example.com", SubID: "sub-ovpn-off", Enable: true}
	otherSub := &model.ClientRecord{Email: "other-sub@example.com", SubID: "sub-other", Enable: true}
	for _, cl := range []*model.ClientRecord{attached, onDisabledInbound, otherSub} {
		if err := db.Create(cl).Error; err != nil {
			t.Fatalf("seed client %s: %v", cl.Email, err)
		}
	}
	links := []model.ClientInbound{
		{ClientId: attached.Id, InboundId: openEnabled.Id},
		{ClientId: attached.Id, InboundId: vless.Id},
		{ClientId: onDisabledInbound.Id, InboundId: openDisabled.Id},
		{ClientId: otherSub.Id, InboundId: openEnabled.Id},
	}
	for i := range links {
		if err := db.Create(&links[i]).Error; err != nil {
			t.Fatalf("seed client_inbound: %v", err)
		}
	}

	s := NewSubService(false, "-ieo")
	emails, err := s.GetOpenvpnEmailsBySubId(subId)
	if err != nil {
		t.Fatalf("GetOpenvpnEmailsBySubId: %v", err)
	}
	if len(emails) != 1 || emails[0] != attached.Email {
		t.Fatalf("emails = %v, want exactly [%s]", emails, attached.Email)
	}

	// A subId whose openvpn inbound is disabled yields nothing.
	emailsOff, err := s.GetOpenvpnEmailsBySubId("sub-ovpn-off")
	if err != nil {
		t.Fatalf("GetOpenvpnEmailsBySubId(disabled): %v", err)
	}
	if len(emailsOff) != 0 {
		t.Fatalf("disabled openvpn inbound must yield no emails, got %v", emailsOff)
	}
}
