package service

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestImportClientFlowOverrideRequestsRuntimeRestartAndUpdatesGeneratedSettings(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()

	inbound := &model.Inbound{
		UserId:         1,
		Tag:            "transfer-flow-target",
		Enable:         true,
		Port:           19444,
		Protocol:       model.VLESS,
		StreamSettings: `{"network":"tcp","security":"reality"}`,
		Settings:       `{"clients":[{"email":"flow@example.com","id":"destination-id","flow":""}],"decryption":"none"}`,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	client := &model.ClientRecord{Email: "flow@example.com", UUID: "destination-id", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatalf("create client association: %v", err)
	}

	report, err := (&ClientService{}).ImportClients(&InboundService{}, ClientTransferEnvelope{
		Format:  clientTransferFormat,
		Version: clientTransferVersion,
		Clients: []ClientTransferEntry{{
			Client:        model.Client{Email: client.Email},
			InboundIds:    []int{inbound.Id},
			FlowOverrides: map[string]string{itoaForTransferTest(inbound.Id): "xtls-rprx-vision"},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("ImportClients: %v", err)
	}
	if report.Failed != 0 || report.Updated != 1 {
		t.Fatalf("unexpected import report: %+v", report)
	}
	if !report.NeedRestart {
		t.Fatal("flow override change did not request an Xray restart")
	}

	var link model.ClientInbound
	if err := db.Where("client_id = ? AND inbound_id = ?", client.Id, inbound.Id).First(&link).Error; err != nil {
		t.Fatalf("load client association: %v", err)
	}
	if link.FlowOverride != "xtls-rprx-vision" {
		t.Fatalf("persisted flow override = %q, want Vision", link.FlowOverride)
	}
	var got model.Inbound
	if err := db.First(&got, inbound.Id).Error; err != nil {
		t.Fatalf("load inbound: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(got.Settings), &settings); err != nil {
		t.Fatalf("generated settings are not JSON: %v", err)
	}
	entry := settings["clients"].([]any)[0].(map[string]any)
	if entry["flow"] != "xtls-rprx-vision" {
		t.Fatalf("inbound client flow = %v, want Vision", entry["flow"])
	}

	// Re-importing the same override is idempotent and must not schedule a
	// needless runtime restart.
	report, err = (&ClientService{}).ImportClients(&InboundService{}, ClientTransferEnvelope{
		Format:  clientTransferFormat,
		Version: clientTransferVersion,
		Clients: []ClientTransferEntry{{
			Client:        model.Client{Email: client.Email},
			InboundIds:    []int{inbound.Id},
			FlowOverrides: map[string]string{itoaForTransferTest(inbound.Id): "xtls-rprx-vision"},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("second ImportClients: %v", err)
	}
	if report.NeedRestart {
		t.Fatal("idempotent flow import requested an unnecessary restart")
	}
}

func itoaForTransferTest(id int) string {
	return strconv.Itoa(id)
}
