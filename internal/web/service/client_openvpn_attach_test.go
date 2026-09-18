package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// OpenVPN inbound settings template as written by the panel: there is no
// Xray-style "clients" array — the daemon is configured from its own keys.
const openvpnSettingsTemplate = `{"proto":"udp","redirectGateway":true,"pushDNS":true,"dns1":"1.1.1.1","dns2":"8.8.8.8"}`

func newOpenvpnInbound(tag string, port int) *model.Inbound {
	return &model.Inbound{
		Tag: tag, Enable: true, Port: port, Protocol: model.OpenVPN,
		Listen: "", StreamSettings: "", Settings: openvpnSettingsTemplate,
	}
}

// TestAttachOpenvpnInbound_NoClientsArray reproduces the reported failure:
// attaching an existing client to an openvpn inbound panicked with an
// interface-conversion error because the inbound settings carry no "clients"
// array (500 from the attach endpoint, client never attached). The
// client-mutation paths must normalize the missing array instead.
func TestAttachOpenvpnInbound_NoClientsArray(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	svc := ClientService{}
	inboundSvc := &InboundService{}

	const email = "ovpnuser@example.com"
	const uid = "ce8d33df-3a64-4f10-8f9b-91c3a8e0d222"
	const sub = "subovpn00000001"

	source := model.Client{Email: email, ID: uid, SubID: sub, Enable: true}
	vless := &model.Inbound{
		Tag: "vless-tcp", Enable: true, Port: 43001, Protocol: model.VLESS,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Settings:       clientsSettings(t, []model.Client{source}),
	}
	if err := db.Create(vless).Error; err != nil {
		t.Fatalf("create vless inbound: %v", err)
	}
	open := newOpenvpnInbound("openvpn-1", 1194)
	if err := db.Create(open).Error; err != nil {
		t.Fatalf("create openvpn inbound: %v", err)
	}

	if err := svc.SyncInbound(nil, vless.Id, []model.Client{clientWithInboundFlow(source, vless)}); err != nil {
		t.Fatalf("SyncInbound(vless): %v", err)
	}
	rec, err := svc.GetRecordByEmail(nil, email)
	if err != nil {
		t.Fatalf("GetRecordByEmail: %v", err)
	}

	// The call that used to panic: attach to the clientless openvpn inbound.
	if _, err := svc.Attach(inboundSvc, rec.Id, []int{open.Id}); err != nil {
		t.Fatalf("Attach(openvpn): %v", err)
	}

	fresh, err := inboundSvc.GetInbound(open.Id)
	if err != nil {
		t.Fatalf("GetInbound(openvpn): %v", err)
	}
	clients, err := inboundSvc.GetClients(fresh)
	if err != nil {
		t.Fatalf("GetClients(openvpn): %v", err)
	}
	if len(clients) != 1 || clients[0].Email != email {
		t.Fatalf("openvpn settings must contain the attached client, got %#v", clients)
	}
	if !strings.Contains(fresh.Settings, `"proto":"udp"`) {
		t.Errorf("openvpn-specific settings keys must be preserved, settings: %s", fresh.Settings)
	}

	links, err := svc.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		t.Fatalf("GetInboundIdsForRecord: %v", err)
	}
	foundOpen := false
	for _, id := range links {
		if id == open.Id {
			foundOpen = true
		}
	}
	if !foundOpen {
		t.Errorf("client_inbounds must link the record to the openvpn inbound, links: %v", links)
	}

	// Editing the client rewrites its entry on every attached inbound —
	// including the openvpn one (previously a second panic site).
	updated := *rec.ToClient()
	updated.Comment = "edited"
	if _, err := svc.Update(inboundSvc, rec.Id, updated); err != nil {
		t.Fatalf("Update with openvpn attachment: %v", err)
	}
	freshAfterUpdate, err := inboundSvc.GetInbound(open.Id)
	if err != nil {
		t.Fatalf("GetInbound(openvpn) after update: %v", err)
	}
	clientsAfterUpdate, err := inboundSvc.GetClients(freshAfterUpdate)
	if err != nil {
		t.Fatalf("GetClients(openvpn) after update: %v", err)
	}
	if len(clientsAfterUpdate) != 1 || clientsAfterUpdate[0].Comment != "edited" {
		t.Errorf("openvpn settings must reflect the client update, got %#v", clientsAfterUpdate)
	}

	// Detach must cleanly remove the client from the openvpn inbound.
	if _, err := svc.Detach(inboundSvc, rec.Id, []int{open.Id}); err != nil {
		t.Fatalf("Detach(openvpn): %v", err)
	}
	freshAfterDetach, err := inboundSvc.GetInbound(open.Id)
	if err != nil {
		t.Fatalf("GetInbound(openvpn) after detach: %v", err)
	}
	clientsAfterDetach, err := inboundSvc.GetClients(freshAfterDetach)
	if err != nil {
		t.Fatalf("GetClients(openvpn) after detach: %v", err)
	}
	if len(clientsAfterDetach) != 0 {
		t.Errorf("openvpn settings must be empty after detach, got %#v", clientsAfterDetach)
	}
	linksAfterDetach, err := svc.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		t.Fatalf("GetInboundIdsForRecord after detach: %v", err)
	}
	for _, id := range linksAfterDetach {
		if id == open.Id {
			t.Errorf("client_inbounds link must be removed after detach")
		}
	}

	// Detaching again (nothing attached) must be a tolerated no-op.
	if _, err := svc.Detach(inboundSvc, rec.Id, []int{open.Id}); err != nil {
		t.Fatalf("Detach(openvpn) again: %v", err)
	}
}

// TestCreateClientDirectlyOnOpenvpn covers creating a brand-new client whose
// first inbound is openvpn — the same clientless-settings path as attach.
func TestCreateClientDirectlyOnOpenvpn(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	svc := ClientService{}
	inboundSvc := &InboundService{}

	open := newOpenvpnInbound("openvpn-1", 1194)
	if err := db.Create(open).Error; err != nil {
		t.Fatalf("create openvpn inbound: %v", err)
	}

	const email = "fresh@example.com"
	_, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: email, Enable: true},
		InboundIds: []int{open.Id},
	})
	if err != nil {
		t.Fatalf("Create on openvpn inbound: %v", err)
	}

	if _, err := svc.GetRecordByEmail(nil, email); err != nil {
		t.Fatalf("GetRecordByEmail: %v", err)
	}
	fresh, err := inboundSvc.GetInbound(open.Id)
	if err != nil {
		t.Fatalf("GetInbound(openvpn): %v", err)
	}
	clients, err := inboundSvc.GetClients(fresh)
	if err != nil {
		t.Fatalf("GetClients(openvpn): %v", err)
	}
	if len(clients) != 1 || clients[0].Email != email {
		t.Fatalf("openvpn settings must contain the created client, got %#v", clients)
	}
}
