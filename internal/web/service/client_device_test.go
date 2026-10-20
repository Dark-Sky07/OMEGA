package service

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

// Device limit semantics:
//   - limitDevice <= 0: unlimited, nothing is ever registered or blocked;
//   - known HWID: always accepted (last-seen refresh only);
//   - new HWID under the limit: registered;
//   - new HWID at the limit: ErrDeviceLimitReached.
func TestRegisterDevice_EnforcesLimit(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	svc := ClientDeviceService{}

	// Unlimited client: no tracking at all.
	if err := svc.RegisterDevice("free@example.com", "hw-1", 0, "1.1.1.1", "app"); err != nil {
		t.Fatalf("unlimited must pass: %v", err)
	}
	if devices, err := svc.GetDevices("free@example.com"); err != nil || len(devices) != 0 {
		t.Fatalf("unlimited client must not register devices, got %d (%v)", len(devices), err)
	}

	// Two-slot client.
	if err := svc.RegisterDevice("lim@example.com", "hw-1", 2, "1.1.1.1", "app"); err != nil {
		t.Fatalf("device 1: %v", err)
	}
	if err := svc.RegisterDevice("lim@example.com", "hw-2", 2, "1.1.1.2", "app"); err != nil {
		t.Fatalf("device 2: %v", err)
	}
	if err := svc.RegisterDevice("lim@example.com", "hw-3", 2, "1.1.1.3", "app"); !errors.Is(err, ErrDeviceLimitReached) {
		t.Fatalf("device 3 over limit: err = %v, want ErrDeviceLimitReached", err)
	}

	// A known device is always allowed back, even while at the limit.
	if err := svc.RegisterDevice("lim@example.com", "hw-1", 2, "1.1.1.9", "app"); err != nil {
		t.Fatalf("known device must pass at limit: %v", err)
	}

	// Emails are matched case/space-insensitively.
	if err := svc.RegisterDevice("  LIM@example.com ", "hw-2", 2, "1.1.1.2", "app"); err != nil {
		t.Fatalf("normalized email must match: %v", err)
	}
	devices, err := svc.GetDevices("lim@example.com")
	if err != nil || len(devices) != 2 {
		t.Fatalf("want 2 devices, got %d (%v)", len(devices), err)
	}

	// Removing one frees a slot for a new device.
	removed, err := svc.RemoveDevice("lim@example.com", "hw-1")
	if err != nil || !removed {
		t.Fatalf("remove hw-1: removed=%v err=%v", removed, err)
	}
	if err := svc.RegisterDevice("lim@example.com", "hw-3", 2, "1.1.1.3", "app"); err != nil {
		t.Fatalf("freed slot must accept a new device: %v", err)
	}

	// Removing a missing device reports false without error.
	removed, err = svc.RemoveDevice("lim@example.com", "hw-404")
	if err != nil || removed {
		t.Fatalf("remove missing: removed=%v err=%v", removed, err)
	}

	// Raising the limit immediately accepts further devices.
	if err := svc.RegisterDevice("lim@example.com", "hw-4", 4, "1.1.1.4", "app"); err != nil {
		t.Fatalf("raised limit must accept: %v", err)
	}

	// RemoveAllDevices clears the registry.
	if err := svc.RemoveAllDevices("lim@example.com"); err != nil {
		t.Fatalf("remove all: %v", err)
	}
	if devices, err := svc.GetDevices("lim@example.com"); err != nil || len(devices) != 0 {
		t.Fatalf("registry must be empty, got %d (%v)", len(devices), err)
	}
}

// RegisterDevice with an empty HWID is a no-op (clients that do not send a
// HWID keep working without any enforcement).
func TestRegisterDevice_EmptyHwidNoop(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	svc := ClientDeviceService{}
	if err := svc.RegisterDevice("x@example.com", "", 1, "", ""); err != nil {
		t.Fatalf("empty hwid must pass: %v", err)
	}
	if devices, err := svc.GetDevices("x@example.com"); err != nil || len(devices) != 0 {
		t.Fatalf("no rows expected, got %d (%v)", len(devices), err)
	}
}

// Sanity: the model maps to the additive client_devices table with a unique
// (email, hwid) index so double registration cannot fork the registry.
func TestClientDeviceTableMapping(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	dev := model.ClientDevice{Email: "u@example.com", Hwid: "hw-1", IP: "2.2.2.2", UserAgent: "app", CreatedAt: 1, LastSeenAt: 1}
	if err := db.Create(&dev).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	dup := model.ClientDevice{Email: "u@example.com", Hwid: "hw-1", IP: "3.3.3.3", UserAgent: "app", CreatedAt: 2, LastSeenAt: 2}
	if err := db.Create(&dup).Error; err == nil {
		t.Fatal("duplicate (email, hwid) must violate the unique index")
	}
}
