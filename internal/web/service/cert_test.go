package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

func initCertTestDB(t *testing.T) CertService {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	return CertService{}
}

func TestValidateCertDomain(t *testing.T) {
	good := map[string]string{
		"Example.COM":  "example.com",
		" sub.a-b.ir ": "sub.a-b.ir",
		"192.0.2.1":    "192.0.2.1",
	}
	for in, want := range good {
		got, err := ValidateCertDomain(in)
		if err != nil || got != want {
			t.Fatalf("ValidateCertDomain(%q) = (%q, %v), want (%q, nil)", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "../etc/passwd", "https://a.com", "a/b.com", "a..b.com", "a b.com"} {
		if _, err := ValidateCertDomain(bad); err == nil {
			t.Fatalf("ValidateCertDomain(%q) must fail", bad)
		}
	}
}

func TestIssueSelfSigned_IssuesInspectsAndApplies(t *testing.T) {
	svc := initCertTestDB(t)
	baseDir := t.TempDir()

	st, err := svc.IssueSelfSigned("panel.example.com", baseDir, true)
	if err != nil {
		t.Fatalf("IssueSelfSigned: %v", err)
	}
	if !st.Configured || !st.SelfSigned || st.Expired {
		t.Fatalf("unexpected status: %+v", st)
	}
	if st.Subject != "panel.example.com" {
		t.Fatalf("subject = %q, want panel.example.com", st.Subject)
	}
	if st.DaysLeft < 360 {
		t.Fatalf("daysLeft = %d, want >= 360", st.DaysLeft)
	}

	// Paths persisted to settings.
	cur := svc.CurrentStatus()
	if !cur.Configured || cur.CertFile != st.CertFile || cur.KeyFile != st.KeyFile {
		t.Fatalf("CurrentStatus = %+v, want paths applied", cur)
	}

	// Self-signed certs are skipped by RenewIfDue.
	renewed, err := svc.RenewIfDue(context.Background())
	if err != nil || renewed {
		t.Fatalf("RenewIfDue on self-signed = (%v, %v), want (false, nil)", renewed, err)
	}
}

func TestIssueLetsEncrypt_RejectsBareIP(t *testing.T) {
	svc := initCertTestDB(t)
	if _, err := svc.IssueLetsEncrypt(context.Background(), "1.2.3.4", "", t.TempDir(), false); err == nil {
		t.Fatal("bare IP must be rejected for HTTP-01")
	}
}
