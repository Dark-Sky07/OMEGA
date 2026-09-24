package service

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestUpdateXrayRejectsUnpinnedVersionBeforeDownload(t *testing.T) {
	if err := (&ServerService{}).UpdateXray("v26.9.8"); err == nil {
		t.Fatal("unpinned Xray version unexpectedly accepted")
	}
}
