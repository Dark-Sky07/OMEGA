package sub

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestAggregateTrafficByEmails_FallsBackToClientLimits(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const email = "node-client@example.com"
	const totalBytes = int64(300) * 1024 * 1024 * 1024
	const expiry = int64(1893456000000)

	db := database.GetDB()
	if err := db.Create(&model.ClientRecord{
		Email:      email,
		TotalGB:    totalBytes,
		ExpiryTime: expiry,
		Enable:     true,
	}).Error; err != nil {
		t.Fatalf("seed client record: %v", err)
	}
	if err := db.Create(&xray.ClientTraffic{
		Email:      email,
		Up:         111,
		Down:       222,
		Total:      0,
		ExpiryTime: 0,
		Enable:     true,
	}).Error; err != nil {
		t.Fatalf("seed client traffic: %v", err)
	}

	var s SubService
	agg, _ := s.AggregateTrafficByEmails([]string{email})

	if agg.Up != 111 || agg.Down != 222 {
		t.Errorf("usage = up %d/down %d, want 111/222", agg.Up, agg.Down)
	}
	if agg.Total != totalBytes {
		t.Errorf("total = %d, want %d (fallback to clients table)", agg.Total, totalBytes)
	}
	if agg.ExpiryTime != expiry {
		t.Errorf("expiry = %d, want %d (fallback to clients table)", agg.ExpiryTime, expiry)
	}
}

func TestBuildStatusNodeLinkAndRecordSubAccess(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	db := database.GetDB()
	if err := db.Create(&model.ClientRecord{
		Email:  "sub-ua@example.com",
		SubID:  "sub-ua-1",
		Enable: true,
	}).Error; err != nil {
		t.Fatalf("seed client record: %v", err)
	}

	var s SubService
	s.RecordSubAccess("sub-ua-1", "v2rayNG/1.8.38")

	var rec model.ClientRecord
	if err := db.Where("email = ?", "sub-ua@example.com").First(&rec).Error; err != nil {
		t.Fatalf("load client record: %v", err)
	}
	if rec.SubLastUpdatedAt <= 0 || rec.SubLastUserAgent != "v2rayNG/1.8.38" {
		t.Fatalf("unexpected sub access fields: ts=%d ua=%q", rec.SubLastUpdatedAt, rec.SubLastUserAgent)
	}

	activeLink := BuildStatusNodeLink(xray.ClientTraffic{
		Up:         1024 * 1024 * 1024,
		Down:       2 * 1024 * 1024 * 1024,
		Total:      10 * 1024 * 1024 * 1024,
		ExpiryTime: time.Now().Add(5 * 24 * time.Hour).UnixMilli(),
		Enable:     true,
	}, "")
	decodedActive, _ := url.PathUnescape(activeLink)
	if !strings.HasPrefix(activeLink, "vless://00000000-0000-0000-0000-000000000000@127.0.0.1:1080") || !strings.Contains(decodedActive, "📊") {
		t.Fatalf("unexpected active status node link: %s", decodedActive)
	}

	expiredLink := BuildStatusNodeLink(xray.ClientTraffic{
		Up:     10 * 1024 * 1024 * 1024,
		Down:   0,
		Total:  10 * 1024 * 1024 * 1024,
		Enable: true,
	}, "اشتراک شما به پایان رسیده است")
	decodedExpired, _ := url.PathUnescape(expiredLink)
	if !strings.Contains(decodedExpired, "اشتراک شما به پایان رسیده است") {
		t.Fatalf("unexpected expired status node link: %s", decodedExpired)
	}
}
