package controller

import (
	"encoding/json"
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/web/entity"
)

func TestPreserveResellerNotificationSettingsForOlderClients(t *testing.T) {
	current := &entity.AllSetting{
		ResellerNotifTrafficWarningPercent:  72,
		ResellerNotifTrafficCriticalPercent: 91,
		ResellerNotifClientWarningPercent:   68,
		ResellerNotifExpiryWarningDays:      12,
		ResellerNotifInPanelEnable:          false,
		ResellerNotifTelegramEnable:         true,
	}
	legacyValue, err := json.Marshal(false)
	if err != nil {
		t.Fatal(err)
	}
	submitted := map[string]json.RawMessage{
		// A key explicitly set to false must not be treated as omitted.
		"resellerNotifTelegramEnable": legacyValue,
	}

	preserveResellerNotificationSettings(submitted, current)

	var got entity.AllSetting
	encoded, err := json.Marshal(submitted)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.ResellerNotifTrafficWarningPercent != 72 || got.ResellerNotifTrafficCriticalPercent != 91 ||
		got.ResellerNotifClientWarningPercent != 68 || got.ResellerNotifExpiryWarningDays != 12 ||
		got.ResellerNotifInPanelEnable || got.ResellerNotifTelegramEnable {
		t.Fatalf("preserved settings = %+v, want stored values and explicit false channel preserved", got)
	}
}

func TestPreserveBrandSettingsForOlderClients(t *testing.T) {
	current := &entity.AllSetting{
		BrandName: "Main", BrandLogo: "/main.svg", BrandColor: "#123456", BrandTheme: "ocean",
		SubBrandName: "Subscription", SubBrandLogo: "/sub.svg", SubBrandColor: "#654321", SubBrandTheme: "forest",
	}
	explicitEmpty, err := json.Marshal("")
	if err != nil {
		t.Fatal(err)
	}
	submitted := map[string]json.RawMessage{"brandName": explicitEmpty}

	preserveBrandSettings(submitted, current)

	encoded, err := json.Marshal(submitted)
	if err != nil {
		t.Fatal(err)
	}
	var got entity.AllSetting
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.BrandName != "" || got.BrandLogo != "/main.svg" || got.BrandColor != "#123456" || got.BrandTheme != "ocean" ||
		got.SubBrandName != "Subscription" || got.SubBrandLogo != "/sub.svg" || got.SubBrandColor != "#654321" || got.SubBrandTheme != "forest" {
		t.Fatalf("preserved branding = %+v, want omitted settings retained and explicit empty preserved", got)
	}
}
