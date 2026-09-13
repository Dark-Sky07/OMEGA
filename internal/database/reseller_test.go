package database

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestResellerMigrationIsAdditiveAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })

	var before []model.User
	if err := db.Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&model.Reseller{}, &model.ResellerInbound{}, &model.ResellerClient{}} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("missing table for %T", table)
		}
	}

	r := model.Reseller{Username: "representative", PasswordHash: "not-a-real-password-hash", MaxBytes: 1024, MaxClients: 1, ExpiresAt: 2000}
	if err := db.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	if err := initModels(); err != nil {
		t.Fatal(err)
	}
	var after []model.User
	if err := db.Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	oldJSON, _ := json.Marshal(before)
	newJSON, _ := json.Marshal(after)
	if string(oldJSON) != string(newJSON) {
		t.Fatal("reseller migration changed administrator accounts")
	}
	var loaded model.Reseller
	if err := db.First(&loaded, r.Id).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.Enabled || loaded.Username != r.Username || loaded.PasswordHash != r.PasswordHash {
		t.Fatal("migration changed reseller data or enabled an inactive account")
	}
	data, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), r.PasswordHash) || strings.Contains(string(data), "PasswordHash") {
		t.Fatal("reseller password hash exposed in JSON")
	}
}
