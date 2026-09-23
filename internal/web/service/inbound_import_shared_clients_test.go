package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// makeImportInbound builds an inbound shaped like the import payload: a clients
// JSON blob plus carried-over ClientStats (the exported traffic counters). The
// stats mirror what controller.importInbound feeds AddInbound after zeroing ids.
func makeImportInbound(tag string, port int, settings string, stats []xray.ClientTraffic) *model.Inbound {
	for i := range stats {
		stats[i].Id = 0
		stats[i].Enable = true
	}
	return &model.Inbound{
		UserId:         1,
		Tag:            tag,
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           port,
		Protocol:       model.VLESS,
		StreamSettings: `{"network":"tcp"}`,
		Settings:       settings,
		ClientStats:    stats,
	}
}

// TestAddInbound_ImportTwoInboundsSharingClients reproduces the panel report:
// importing inbound #1 then inbound #2 when both carry the same clients (same
// email + subId) used to fail with "UNIQUE constraint failed: client_traffics.email".
// The shared email already owns a row from the first import, and the second
// inbound's ClientStats association tried to plain-INSERT it again.
func TestAddInbound_ImportTwoInboundsSharingClients(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}

	// Inbound #1: clients alice (shared) and bob (unique to #1).
	settings1 := `{"clients":[` +
		`{"id":"11111111-1111-1111-1111-111111111111","email":"alice","subId":"s-alice","enable":true},` +
		`{"id":"22222222-2222-2222-2222-222222222222","email":"bob","subId":"s-bob","enable":true}` +
		`],"decryption":"none","encryption":"none"}`
	in1 := makeImportInbound("in-9101-tcp", 9101, settings1, []xray.ClientTraffic{
		{Email: "alice", Up: 100, Down: 200, Total: 1000},
		{Email: "bob", Up: 1, Down: 2, Total: 1000},
	})
	if _, _, err := svc.AddInbound(in1); err != nil {
		t.Fatalf("import inbound #1: %v", err)
	}

	// Inbound #2: clients alice (same email+subId as #1) and carol (unique to #2).
	settings2 := `{"clients":[` +
		`{"id":"11111111-1111-1111-1111-111111111111","email":"alice","subId":"s-alice","enable":true},` +
		`{"id":"33333333-3333-3333-3333-333333333333","email":"carol","subId":"s-carol","enable":true}` +
		`],"decryption":"none","encryption":"none"}`
	in2 := makeImportInbound("in-9102-tcp", 9102, settings2, []xray.ClientTraffic{
		{Email: "alice", Up: 999, Down: 999, Total: 9999}, // would clobber the shared row if inserted
		{Email: "carol", Up: 3, Down: 4, Total: 1000},
	})
	if _, _, err := svc.AddInbound(in2); err != nil {
		t.Fatalf("import inbound #2 (the reported failure): %v", err)
	}

	// One traffic row per distinct email — no duplicate "alice".
	for _, tc := range []struct {
		email string
		want  int64
	}{
		{"alice", 100}, // preserved from import #1, not clobbered by #2's 999
		{"bob", 1},
		{"carol", 3},
	} {
		var rows []xray.ClientTraffic
		if err := database.GetDB().Where("email = ?", tc.email).Find(&rows).Error; err != nil {
			t.Fatalf("query %s: %v", tc.email, err)
		}
		if len(rows) != 1 {
			t.Fatalf("email %q: got %d traffic rows, want exactly 1", tc.email, len(rows))
		}
		if rows[0].Up != tc.want {
			t.Fatalf("email %q: Up = %d, want %d (shared row should keep the first import's counters)", tc.email, rows[0].Up, tc.want)
		}
	}
}

// TestAddInbound_ImportStatsMissingClientStillGetsTrafficRow covers an import
// payload whose clientStats doesn't cover every client in settings (older
// exports / hand-edited JSON): the uncovered client must still end up with a
// traffic row, or it would escape quota and expiry accounting.
func TestAddInbound_ImportStatsMissingClientStillGetsTrafficRow(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}

	settings := `{"clients":[` +
		`{"id":"44444444-4444-4444-4444-444444444444","email":"dave","subId":"s-dave","enable":true,"totalGB":1000},` +
		`{"id":"55555555-5555-5555-5555-555555555555","email":"erin","subId":"s-erin","enable":true,"totalGB":2000}` +
		`],"decryption":"none","encryption":"none"}`
	// Stats cover dave only; erin is missing.
	in := makeImportInbound("in-9103-tcp", 9103, settings, []xray.ClientTraffic{
		{Email: "dave", Up: 7, Down: 8, Total: 1000},
	})
	if _, _, err := svc.AddInbound(in); err != nil {
		t.Fatalf("import inbound: %v", err)
	}

	var dave xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", "dave").First(&dave).Error; err != nil {
		t.Fatalf("dave row: %v", err)
	}
	if dave.Up != 7 {
		t.Fatalf("dave Up = %d, want 7 (imported counters preserved)", dave.Up)
	}

	var erin xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", "erin").First(&erin).Error; err != nil {
		t.Fatalf("erin must still get a traffic row despite missing from clientStats: %v", err)
	}
	if erin.Up != 0 || erin.Down != 0 {
		t.Fatalf("erin counters = %d/%d, want zeroed", erin.Up, erin.Down)
	}
	if erin.Total != 2000 {
		t.Fatalf("erin Total = %d, want 2000 (quota taken from client settings)", erin.Total)
	}
}

func TestAddInboundPreservingExistingClientsKeepsDestinationRecord(t *testing.T) {
	setupConflictDB(t)

	existing := &model.ClientRecord{
		Email:      "shared@example.com",
		SubID:      "destination-sub",
		UUID:       "destination-uuid",
		Password:   "destination-password",
		Auth:       "destination-auth",
		Flow:       "destination-flow",
		Security:   "tls",
		LimitIP:    7,
		TotalGB:    321,
		ExpiryTime: 123456,
		Enable:     false,
		Comment:    "destination-comment",
		CreatedAt:  111,
		UpdatedAt:  222,
	}
	if err := database.GetDB().Create(existing).Error; err != nil {
		t.Fatalf("seed canonical client: %v", err)
	}

	inbound := &model.Inbound{
		UserId:         1,
		Tag:            "import-preserve-9443",
		Enable:         true,
		Listen:         "0.0.0.0",
		Port:           9443,
		Protocol:       model.VLESS,
		StreamSettings: `{"network":"tcp"}`,
		Settings:       `{"clients":[{"email":"shared@example.com","subId":"destination-sub","id":"source-uuid","password":"source-password","totalGB":9999,"enable":true,"flow":"source-flow"}]}`,
	}
	if err := (&InboundService{}).PrepareInboundImport(inbound); err != nil {
		t.Fatalf("PrepareInboundImport: %v", err)
	}
	if _, _, err := (&InboundService{}).AddInboundPreservingExistingClients(inbound); err != nil {
		t.Fatalf("AddInboundPreservingExistingClients: %v", err)
	}

	var got model.ClientRecord
	if err := database.GetDB().Where("email = ?", existing.Email).First(&got).Error; err != nil {
		t.Fatalf("load canonical client: %v", err)
	}
	if got.SubID != existing.SubID || got.UUID != existing.UUID || got.Password != existing.Password ||
		got.Auth != existing.Auth || got.Flow != existing.Flow || got.Security != existing.Security ||
		got.LimitIP != existing.LimitIP || got.TotalGB != existing.TotalGB || got.ExpiryTime != existing.ExpiryTime ||
		got.Enable != existing.Enable || got.Comment != existing.Comment || got.CreatedAt != existing.CreatedAt || got.UpdatedAt != existing.UpdatedAt {
		t.Fatalf("destination client was overwritten: got %+v want %+v", got, *existing)
	}

	var linkCount int64
	if err := database.GetDB().Model(&model.ClientInbound{}).
		Where("client_id = ? AND inbound_id = ?", got.Id, inbound.Id).
		Count(&linkCount).Error; err != nil {
		t.Fatalf("query imported association: %v", err)
	}
	if linkCount != 1 {
		t.Fatalf("imported association count = %d, want 1", linkCount)
	}
}

func TestPrepareInboundImportPreservesExistingCanonicalClient(t *testing.T) {
	setupConflictDB(t)

	existing := &model.ClientRecord{
		Email: "shared@example.com",
		SubID: "destination-sub",
		UUID: "destination-uuid",
		Password: "destination-password",
		Auth: "destination-auth",
		Flow: "destination-flow",
		LimitIP: 7,
		TotalGB: 321,
		ExpiryTime: 123456,
		Enable: false,
		Comment: "destination-comment",
		CreatedAt: 111,
		UpdatedAt: 222,
	}
	if err := database.GetDB().Create(existing).Error; err != nil {
		t.Fatalf("seed canonical client: %v", err)
	}

	inbound := &model.Inbound{Settings: `{"clients":[{"email":"shared@example.com","subId":"destination-sub","id":"source-uuid","password":"source-password","totalGB":9999,"enable":true},{"email":"new@example.com","id":"new-uuid"}],"decryption":"none"}`}
	if err := (&InboundService{}).PrepareInboundImport(inbound); err != nil {
		t.Fatalf("PrepareInboundImport: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		t.Fatalf("prepared settings are not JSON: %v", err)
	}
	clients, ok := settings["clients"].([]any)
	if !ok || len(clients) != 2 {
		t.Fatalf("prepared clients = %v, want two entries", settings["clients"])
	}
	canonical := clients[0].(map[string]any)
	if canonical["id"] != "destination-uuid" || canonical["password"] != "destination-password" || canonical["totalGB"] != float64(321) || canonical["enable"] != false {
		t.Fatalf("existing canonical client was replaced: %v", canonical)
	}
	newClient := clients[1].(map[string]any)
	if newClient["id"] != "new-uuid" {
		t.Fatalf("new client payload was unexpectedly changed: %v", newClient)
	}
}
