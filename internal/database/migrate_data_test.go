package database

import (
	"os"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMigrateData_CompositeKeyTableLargerThanBatch(t *testing.T) {
	dsn := os.Getenv("XUI_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set XUI_TEST_PG_DSN to a reachable Postgres to run this test")
	}

	// Seed a SQLite source with the full schema and >500 client_inbounds rows.
	srcPath := t.TempDir() + "/x-ui.db"
	src, err := gorm.Open(sqlite.Open(srcPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, m := range migrationModels() {
		if err := src.AutoMigrate(m); err != nil {
			t.Fatalf("automigrate %T: %v", m, err)
		}
	}
	const n = 600 // > batchSize (500) so the between-batches path is exercised
	links := make([]model.ClientInbound, 0, n)
	for i := 1; i <= n; i++ {
		links = append(links, model.ClientInbound{ClientId: i, InboundId: 1})
	}
	if err := src.CreateInBatches(links, 200).Error; err != nil {
		t.Fatalf("seed client_inbounds: %v", err)
	}
	if sqlDB, err := src.DB(); err == nil {
		sqlDB.Close() // flush before MigrateData reopens the file
	}

	// Make the test re-runnable: drop any tables from a previous run.
	dst, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := dst.Migrator().DropTable(migrationModels()...); err != nil {
		t.Fatalf("drop tables: %v", err)
	}

	if err := MigrateData(srcPath, dsn); err != nil {
		t.Fatalf("MigrateData: %v", err) // fails here before the fix
	}

	var got int64
	if err := dst.Model(&model.ClientInbound{}).Count(&got).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != n {
		t.Fatalf("client_inbounds rows = %d, want %d", got, n)
	}
}

func TestMigrateData_PreservesFalseDefaultedColumns(t *testing.T) {
	dsn := os.Getenv("XUI_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set XUI_TEST_PG_DSN to a reachable Postgres to run this test")
	}

	srcPath := t.TempDir() + "/x-ui.db"
	src, err := gorm.Open(sqlite.Open(srcPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, m := range migrationModels() {
		if err := src.AutoMigrate(m); err != nil {
			t.Fatalf("automigrate %T: %v", m, err)
		}
	}

	if err := src.Create([]*model.ClientRecord{
		{Email: "on@example.com"},
		{Email: "off@example.com"},
	}).Error; err != nil {
		t.Fatalf("seed clients: %v", err)
	}
	if err := src.Model(&model.ClientRecord{}).Where("email = ?", "off@example.com").
		Update("enable", false).Error; err != nil {
		t.Fatalf("disable client: %v", err)
	}
	if err := src.Create(&model.Node{Name: "n-off", Address: "1.2.3.4", Port: 1, ApiToken: "tok"}).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := src.Model(&model.Node{}).Where("name = ?", "n-off").
		Update("enable", false).Error; err != nil {
		t.Fatalf("disable node: %v", err)
	}
	if sqlDB, err := src.DB(); err == nil {
		sqlDB.Close()
	}

	dst, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := dst.Migrator().DropTable(migrationModels()...); err != nil {
		t.Fatalf("drop tables: %v", err)
	}

	if err := MigrateData(srcPath, dsn); err != nil {
		t.Fatalf("MigrateData: %v", err)
	}

	var off model.ClientRecord
	if err := dst.Where("email = ?", "off@example.com").First(&off).Error; err != nil {
		t.Fatalf("load disabled client: %v", err)
	}
	if off.Enable {
		t.Fatalf("disabled client re-enabled after migration (enable=%v)", off.Enable)
	}

	var on model.ClientRecord
	if err := dst.Where("email = ?", "on@example.com").First(&on).Error; err != nil {
		t.Fatalf("load enabled client: %v", err)
	}
	if !on.Enable {
		t.Fatalf("enabled client wrongly disabled after migration")
	}

	var node model.Node
	if err := dst.Where("name = ?", "n-off").First(&node).Error; err != nil {
		t.Fatalf("load node: %v", err)
	}
	if node.Enable {
		t.Fatalf("disabled node re-enabled after migration")
	}
}

func TestInitDB_DropsLegacyResellerInboundUniqueIndex(t *testing.T) {
	dbPath := t.TempDir() + "/x-ui-legacy.db"
	raw, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := raw.AutoMigrate(&model.ResellerInbound{}); err != nil {
		t.Fatalf("automigrate reseller_inbounds: %v", err)
	}
	if err := raw.Exec("CREATE UNIQUE INDEX ux_reseller_inbounds_inbound ON reseller_inbounds(inbound_id)").Error; err != nil {
		t.Fatalf("create legacy unique index: %v", err)
	}
	if sqlDB, err := raw.DB(); err == nil {
		sqlDB.Close()
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		_ = CloseDB()
	})
	if db.Migrator().HasIndex(&model.ResellerInbound{}, "ux_reseller_inbounds_inbound") {
		t.Fatal("legacy unique index ux_reseller_inbounds_inbound was not dropped")
	}
	if err := db.Create(&model.ResellerInbound{ResellerId: 1, InboundId: 10}).Error; err != nil {
		t.Fatalf("first reseller inbound mapping: %v", err)
	}
	if err := db.Create(&model.ResellerInbound{ResellerId: 2, InboundId: 10}).Error; err != nil {
		t.Fatalf("shared reseller inbound mapping rejected after upgrade: %v", err)
	}
}
