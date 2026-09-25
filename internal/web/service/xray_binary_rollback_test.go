package service

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestXrayBinarySnapshotRestoresBytesAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xray")
	original := []byte("previous-xray-binary")
	if err := os.WriteFile(path, original, 0711); err != nil {
		t.Fatalf("write original binary: %v", err)
	}
	if err := os.Chmod(path, 0711); err != nil {
		t.Fatalf("chmod original binary: %v", err)
	}

	snapshot, err := snapshotXrayBinary(path)
	if err != nil {
		t.Fatalf("snapshotXrayBinary: %v", err)
	}
	if !snapshot.exists || !bytes.Equal(snapshot.contents, original) || snapshot.mode != 0711 {
		t.Fatalf("unexpected snapshot: exists=%v bytes=%q mode=%#o", snapshot.exists, snapshot.contents, snapshot.mode)
	}

	if err := os.WriteFile(path, []byte("failed replacement"), 0755); err != nil {
		t.Fatalf("write replacement: %v", err)
	}
	if err := restoreXrayBinary(path, snapshot); err != nil {
		t.Fatalf("restoreXrayBinary: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored binary: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat restored binary: %v", err)
	}
	if !bytes.Equal(got, original) || info.Mode().Perm() != 0711 {
		t.Fatalf("restored binary = %q mode=%#o, want original bytes/mode", got, info.Mode().Perm())
	}
}

func TestRestoreXrayBinaryRemovesNewFileWhenThereWasNoPreviousBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xray")
	snapshot, err := snapshotXrayBinary(path)
	if err != nil {
		t.Fatalf("snapshot missing binary: %v", err)
	}
	if snapshot.exists {
		t.Fatal("missing binary was reported as present")
	}
	if err := os.WriteFile(path, []byte("new binary"), 0755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}
	if err := restoreXrayBinary(path, snapshot); err != nil {
		t.Fatalf("restore missing binary: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("restored missing binary stat error = %v, want not-exist", err)
	}
}

func TestWriteXrayBinaryAtomicallyRejectsOversizedReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xray")
	original := []byte("still running")
	if err := os.WriteFile(path, original, 0755); err != nil {
		t.Fatalf("write original binary: %v", err)
	}
	if err := writeXrayBinaryAtomically(path, bytes.NewReader([]byte("too large")), int64(len("too")), 0755); err == nil {
		t.Fatal("oversized replacement unexpectedly succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original after rejected replacement: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("rejected replacement changed target to %q", got)
	}
}

func TestUpdateXrayRejectsMalformedVersionBeforeNetwork(t *testing.T) {
	if err := (&ServerService{}).UpdateXray("latest"); err == nil {
		t.Fatal("malformed Xray version unexpectedly accepted")
	}
}

func TestCompareXrayVersionsUsesNumericComponents(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{left: "v26.10.0", right: "v26.9.9", want: 1},
		{left: "v26.9.9", right: "v26.9.9", want: 0},
		{left: "v25.12.99", right: "v26.1.0", want: -1},
	} {
		if got := compareXrayVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareXrayVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestXrayReleasesAPIUsesBoundedRecentPage(t *testing.T) {
	parsed, err := url.Parse(xrayReleasesAPIURL)
	if err != nil {
		t.Fatalf("parse Xray releases API URL: %v", err)
	}
	pageSize, err := strconv.Atoi(parsed.Query().Get("per_page"))
	if err != nil || pageSize <= 0 || pageSize > 20 {
		t.Fatalf("Xray releases API page size = %q, want a positive bounded page of at most 20", parsed.Query().Get("per_page"))
	}
}

func TestSelectLatestXrayReleaseSkipsDraftsInvalidTagsAndMissingAssets(t *testing.T) {
	assetName := "Xray-linux-64.zip"
	releases := []xrayRelease{
		{TagName: "v26.10.0", Assets: nil},
		{TagName: "v26.9.9", Prerelease: true, Assets: []xrayReleaseAsset{{Name: assetName}}},
		{TagName: "v28.0.0", Draft: true, Assets: []xrayReleaseAsset{{Name: assetName}}},
		{TagName: "v27.0.0-rc1", Prerelease: true, Assets: []xrayReleaseAsset{{Name: assetName}}},
		{TagName: "v26.3.27", Assets: []xrayReleaseAsset{{Name: assetName}}},
	}

	latest, ok := selectLatestXrayRelease(releases, assetName)
	if !ok || latest.TagName != "v26.9.9" {
		t.Fatalf("selected release = %q, ok=%v; want v26.9.9", latest.TagName, ok)
	}
}
