package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestFilterResellerInboundClientsUsesExplicitMappingsOnly(t *testing.T) {
	settings := `{"clients":[{"email":"Alice@example.com","id":"alice-secret"},{"email":"bob@example.com","id":"bob-secret"}],"decryption":"none","clientsStatsHint":"keep"}`
	inbounds := []*model.Inbound{{
		Settings: settings,
		ClientStats: []xray.ClientTraffic{
			{Email: "Alice@example.com", Up: 10, Down: 20},
			{Email: "bob@example.com", Up: 30, Down: 40},
		},
	}}

	filterResellerInboundClients(inbounds, map[string]struct{}{
		"alice@example.com": {},
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(inbounds[0].Settings), &parsed); err != nil {
		t.Fatalf("filtered settings are not JSON: %v", err)
	}
	clients, ok := parsed["clients"].([]any)
	if !ok || len(clients) != 1 {
		t.Fatalf("got clients=%v, want exactly one explicitly mapped client", parsed["clients"])
	}
	client := clients[0].(map[string]any)
	if client["email"] != "Alice@example.com" || client["id"] != "alice-secret" {
		t.Fatalf("unexpected visible client: %v", client)
	}
	if parsed["decryption"] != "none" || parsed["clientsStatsHint"] != "keep" {
		t.Fatalf("non-client inbound settings were not preserved: %v", parsed)
	}
	if len(inbounds[0].ClientStats) != 1 || inbounds[0].ClientStats[0].Email != "Alice@example.com" {
		t.Fatalf("client stats were not filtered to explicit mappings: %+v", inbounds[0].ClientStats)
	}
}

func TestFilterResellerSettingsClientsFailsClosedOnMalformedJSON(t *testing.T) {
	got := filterResellerSettingsClients(`{"clients":[{"email":"secret@example.com"}]`, map[string]struct{}{
		"secret@example.com": {},
	})
	if got != `{"clients":[]}` {
		t.Fatalf("malformed settings leaked or changed unexpectedly: %q", got)
	}
}

func TestFilterResellerSettingsClientsPreservesSettingsWithoutClients(t *testing.T) {
	settings := `{"decryption":"none","fallbacks":[{"dest":"example.com"}]}`
	if got := filterResellerSettingsClients(settings, map[string]struct{}{}); got != settings {
		t.Fatalf("settings without a clients key changed: got %q want %q", got, settings)
	}
}

func TestGetInboundsForResellerDoesNotInheritInboundClients(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()

	reseller := &model.Reseller{Username: "scope-test", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	inbound := &model.Inbound{
		UserId:   1,
		Tag:      "owned-with-shared-clients",
		Enable:   true,
		Port:     19443,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"email":"visible@example.com","id":"visible-id","totalGB":10},{"email":"hidden@example.com","id":"hidden-id","totalGB":20}],"decryption":"none","fallbacks":[{"dest":"127.0.0.1:8080"}]}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err := db.Create(&model.ResellerInbound{ResellerId: reseller.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatalf("assign inbound: %v", err)
	}
	if err := db.Create(&model.ResellerClient{ResellerId: reseller.Id, Email: "visible@example.com"}).Error; err != nil {
		t.Fatalf("assign explicit client: %v", err)
	}
	for _, stat := range []*xray.ClientTraffic{
		{InboundId: inbound.Id, Email: "visible@example.com", Up: 11, Down: 12, Enable: true},
		{InboundId: inbound.Id, Email: "hidden@example.com", Up: 21, Down: 22, Enable: true},
	} {
		if err := db.Create(stat).Error; err != nil {
			t.Fatalf("create traffic row %s: %v", stat.Email, err)
		}
	}

	rows, err := (&InboundService{}).GetInboundsForReseller(reseller.Id)
	if err != nil {
		t.Fatalf("GetInboundsForReseller: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d inbounds, want one owned inbound", len(rows))
	}
	if len(rows[0].ClientStats) != 1 || rows[0].ClientStats[0].Email != "visible@example.com" {
		t.Fatalf("client stats leaked or were lost: %+v", rows[0].ClientStats)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(rows[0].Settings), &settings); err != nil {
		t.Fatalf("scoped settings are not JSON: %v", err)
	}
	clients, ok := settings["clients"].([]any)
	if !ok || len(clients) != 1 {
		t.Fatalf("scoped settings clients = %v, want one explicit client", settings["clients"])
	}
	if clients[0].(map[string]any)["email"] != "visible@example.com" {
		t.Fatalf("unexpected visible client: %v", clients[0])
	}
	if settings["decryption"] != "none" || settings["fallbacks"].([]any)[0].(map[string]any)["dest"] != "127.0.0.1:8080" {
		t.Fatalf("non-client inbound configuration was not preserved: %v", settings)
	}
}
