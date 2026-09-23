package service

import (
	"encoding/json"
	"testing"

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
