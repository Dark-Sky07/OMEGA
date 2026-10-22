package service

import (
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/web/entity"
)

func TestNormalizeResellerBranding(t *testing.T) {
	name, logo, color, err := NormalizeResellerBranding("  Northstar  ", "/assets/logo.svg", "#ab12EF")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Northstar" || logo != "/assets/logo.svg" || color != "#AB12EF" {
		t.Fatalf("normalized branding = %q, %q, %q", name, logo, color)
	}

	if _, _, _, err := NormalizeResellerBranding("Brand", "https://cdn.example/logo.png", "#123456"); err != nil {
		t.Fatalf("HTTPS logo rejected: %v", err)
	}

	invalid := []struct {
		name  string
		logo  string
		color string
	}{
		{name: "Brand", logo: "http://cdn.example/logo.png", color: "#123456"},
		{name: "Brand", logo: "//evil.example/logo.png", color: "#123456"},
		{name: "Brand", logo: "javascript:alert(1)", color: "#123456"},
		{name: "Brand", logo: "/logo.png", color: "red"},
		{name: "Brand\nInjected", logo: "/logo.png", color: "#123456"},
	}
	for _, tc := range invalid {
		t.Run(tc.name+tc.logo+tc.color, func(t *testing.T) {
			if _, _, _, err := NormalizeResellerBranding(tc.name, tc.logo, tc.color); err == nil {
				t.Fatal("expected invalid branding to be rejected")
			}
		})
	}
}

func TestUpdateResellerBrandingPersistsWithoutChangingAccount(t *testing.T) {
	if err := database.InitDB(t.TempDir() + "/x-ui.db"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	var svc ResellerService
	reseller := &model.Reseller{Username: "brand-owner", Name: "Owner", Enable: true}
	if err := svc.Add(reseller, "pass1234"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateBranding(reseller.Id, "Northstar", "https://cdn.example/logo.svg", "#abcdef"); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Get(reseller.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.BrandName != "Northstar" || updated.BrandLogo != "https://cdn.example/logo.svg" || updated.BrandColor != "#ABCDEF" {
		t.Fatalf("persisted branding = %#v", updated)
	}
	if updated.Username != reseller.Username || updated.Name != reseller.Name {
		t.Fatalf("branding update changed reseller account: %#v", updated)
	}
}

func TestUpdateResellerBrandSettingsValidatesThemeAndUniqueDomain(t *testing.T) {
	if err := database.InitDB(t.TempDir() + "/x-ui.db"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	var svc ResellerService
	first := &model.Reseller{Username: "domain-owner-one", Enable: true}
	second := &model.Reseller{Username: "domain-owner-two", Enable: true}
	if err := svc.Add(first, "pass1234"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Add(second, "pass1234"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateBrandSettings(first.Id, "Northstar", "/logo.svg", "#abcdef", "ocean", "Sub.Example.com"); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Get(first.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.BrandTheme != "ocean" || updated.SubDomain != "sub.example.com" {
		t.Fatalf("persisted theme/domain = %q/%q", updated.BrandTheme, updated.SubDomain)
	}
	if err := svc.UpdateBrandSettings(second.Id, "Other", "", "", "invalid", "other.example.com"); err == nil {
		t.Fatal("unsupported theme was accepted")
	}
	if err := svc.UpdateBrandSettings(second.Id, "Other", "", "", "forest", "SUB.EXAMPLE.COM"); err == nil {
		t.Fatal("duplicate reseller domain was accepted")
	}
}

func TestValidateBrandSettingsNormalizesAndRejectsInvalidTheme(t *testing.T) {
	settings := &entity.AllSetting{
		BrandName: "  Main Brand  ", BrandLogo: "/brand.svg", BrandColor: "#abcdef", BrandTheme: "OCEAN",
		SubBrandName: "  Subscription  ", SubBrandTheme: "forest",
	}
	if err := validateBrandSettings(settings); err != nil {
		t.Fatal(err)
	}
	if settings.BrandName != "Main Brand" || settings.BrandColor != "#ABCDEF" || settings.BrandTheme != "ocean" {
		t.Fatalf("brand settings not normalized: %#v", settings)
	}
	settings.SubBrandTheme = "neon"
	if err := validateBrandSettings(settings); err == nil {
		t.Fatal("unsupported subscription theme was accepted")
	}
}
