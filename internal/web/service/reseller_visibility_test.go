package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestResellerClientMappingNormalizesEmailIdentity(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "mapping-normalization", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	client := &model.ClientRecord{Email: "Canonical@Example.com", UUID: "mapping-uuid", SubID: "mapping-sub"}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ResellerClient{ResellerId: reseller.Id, Email: "  canonical@example.COM  "}).Error; err != nil {
		t.Fatalf("create legacy mapping: %v", err)
	}

	service := &ResellerService{}
	emails, err := service.OwnedEmails(reseller.Id)
	if err != nil {
		t.Fatalf("OwnedEmails: %v", err)
	}
	if len(emails) != 1 || emails[0] != client.Email {
		t.Fatalf("OwnedEmails = %v, want canonical %q", emails, client.Email)
	}
	owned, err := service.OwnsClient(reseller.Id, " CANONICAL@example.com ")
	if err != nil || !owned {
		t.Fatalf("OwnsClient normalized lookup = %v, %v; want true", owned, err)
	}
	if err := service.AssignClient(reseller.Id, " canonical@EXAMPLE.com "); err != nil {
		t.Fatalf("AssignClient should be idempotent across case/space: %v", err)
	}
	var mappingCount int64
	if err := db.Model(&model.ResellerClient{}).Where("reseller_id = ?", reseller.Id).Count(&mappingCount).Error; err != nil {
		t.Fatalf("count mappings: %v", err)
	}
	if mappingCount != 1 {
		t.Fatalf("mapping count = %d, want one", mappingCount)
	}
	if err := service.UnassignClient(reseller.Id, " CANONICAL@example.COM "); err != nil {
		t.Fatalf("UnassignClient normalized lookup: %v", err)
	}
	if owned, err := service.OwnsClient(reseller.Id, client.Email); err != nil || owned {
		t.Fatalf("mapping remained after normalized unassign: owned=%v err=%v", owned, err)
	}
}

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

func TestOwnedAssociatedEmailSetRequiresExplicitMappingAndOwnedAssociation(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "associated-scope", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	ownedInbound := &model.Inbound{UserId: 1, Tag: "owned-associated", Port: 19501, Protocol: model.VLESS, Settings: `{"clients":[]}`}
	adminInbound := &model.Inbound{UserId: 1, Tag: "admin-associated", Port: 19502, Protocol: model.VLESS, Settings: `{"clients":[]}`}
	if err := db.Create(ownedInbound).Error; err != nil {
		t.Fatalf("create owned inbound: %v", err)
	}
	if err := db.Create(adminInbound).Error; err != nil {
		t.Fatalf("create admin inbound: %v", err)
	}
	if err := db.Create(&model.ResellerInbound{ResellerId: reseller.Id, InboundId: ownedInbound.Id}).Error; err != nil {
		t.Fatalf("assign owned inbound: %v", err)
	}
	clients := []*model.ClientRecord{
		{Email: "shared@example.com", UUID: "shared"},
		{Email: "admin-only@example.com", UUID: "admin-only"},
		{Email: "unmapped@example.com", UUID: "unmapped"},
	}
	for _, client := range clients {
		if err := db.Create(client).Error; err != nil {
			t.Fatalf("create client %s: %v", client.Email, err)
		}
	}
	if err := db.Create(&model.ResellerClient{ResellerId: reseller.Id, Email: "shared@example.com"}).Error; err != nil {
		t.Fatalf("map shared client: %v", err)
	}
	if err := db.Create(&model.ResellerClient{ResellerId: reseller.Id, Email: "admin-only@example.com"}).Error; err != nil {
		t.Fatalf("map admin-only client: %v", err)
	}
	var shared, adminOnly, unmapped model.ClientRecord
	if err := db.Where("email = ?", "shared@example.com").First(&shared).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("email = ?", "admin-only@example.com").First(&adminOnly).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("email = ?", "unmapped@example.com").First(&unmapped).Error; err != nil {
		t.Fatal(err)
	}
	for _, link := range []model.ClientInbound{
		{ClientId: shared.Id, InboundId: ownedInbound.Id},
		{ClientId: shared.Id, InboundId: adminInbound.Id},
		{ClientId: adminOnly.Id, InboundId: adminInbound.Id},
		{ClientId: unmapped.Id, InboundId: ownedInbound.Id},
	} {
		if err := db.Create(&link).Error; err != nil {
			t.Fatalf("create client association: %v", err)
		}
	}

	set, err := (&ResellerService{}).OwnedAssociatedEmailSet(reseller.Id)
	if err != nil {
		t.Fatalf("OwnedAssociatedEmailSet: %v", err)
	}
	if _, ok := set["shared@example.com"]; !ok {
		t.Fatalf("shared explicitly mapped client with owned association was omitted: %v", set)
	}
	if _, ok := set["admin-only@example.com"]; ok {
		t.Fatalf("admin-only association was incorrectly included: %v", set)
	}
	if _, ok := set["unmapped@example.com"]; ok {
		t.Fatalf("unmapped client was incorrectly included: %v", set)
	}

	isolated, err := (&ResellerService{}).OwnedIsolatedAssociatedEmailSet(reseller.Id)
	if err != nil {
		t.Fatalf("OwnedIsolatedAssociatedEmailSet: %v", err)
	}
	if _, ok := isolated["shared@example.com"]; ok {
		t.Fatalf("shared client traffic scope was incorrectly included: %v", isolated)
	}
	if _, ok := isolated["admin-only@example.com"]; ok {
		t.Fatalf("admin-only client was incorrectly included in isolated scope: %v", isolated)
	}

	traffic, err := (&InboundService{}).GetClientTrafficByEmailForInbounds("shared@example.com", map[int]struct{}{ownedInbound.Id: {}})
	if err != nil {
		t.Fatalf("scoped traffic lookup: %v", err)
	}
	if traffic != nil {
		t.Fatalf("shared client traffic was exposed through an owned inbound: %+v", traffic)
	}

	ownedOnly := &model.ClientRecord{Email: "owned-only@example.com", UUID: "owned-only"}
	if err := db.Create(ownedOnly).Error; err != nil {
		t.Fatalf("create owned-only client: %v", err)
	}
	if err := db.Create(&model.ResellerClient{ResellerId: reseller.Id, Email: ownedOnly.Email}).Error; err != nil {
		t.Fatalf("map owned-only client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: ownedOnly.Id, InboundId: ownedInbound.Id}).Error; err != nil {
		t.Fatalf("associate owned-only client: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{
		Email:     ownedOnly.Email,
		InboundId: adminInbound.Id,
		Up:        123,
		Down:      456,
	}).Error; err != nil {
		t.Fatalf("create owned-only traffic row: %v", err)
	}

	traffic, err = (&InboundService{}).GetClientTrafficByEmailForInbounds(ownedOnly.Email, map[int]struct{}{ownedInbound.Id: {}})
	if err != nil {
		t.Fatalf("isolated scoped traffic lookup: %v", err)
	}
	if traffic == nil || traffic.Up != 123 || traffic.Down != 456 {
		t.Fatalf("isolated traffic counters were not returned: %+v", traffic)
	}
	if traffic.InboundId != 0 {
		t.Fatalf("scoped traffic leaked unrelated inbound id %d", traffic.InboundId)
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
	visibleRecord := &model.ClientRecord{Email: "visible@example.com", UUID: "visible-id", SubID: "visible-sub"}
	if err := db.Create(visibleRecord).Error; err != nil {
		t.Fatalf("create visible client record: %v", err)
	}
	hiddenRecord := &model.ClientRecord{Email: "hidden@example.com", UUID: "hidden-id", SubID: "hidden-sub"}
	if err := db.Create(hiddenRecord).Error; err != nil {
		t.Fatalf("create hidden client record: %v", err)
	}
	for _, link := range []model.ClientInbound{
		{ClientId: visibleRecord.Id, InboundId: inbound.Id},
		{ClientId: hiddenRecord.Id, InboundId: inbound.Id},
	} {
		if err := db.Create(&link).Error; err != nil {
			t.Fatalf("create client association: %v", err)
		}
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
