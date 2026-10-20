package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCloudBackupS3AndWebDAVWithRetention(t *testing.T) {
	var mu sync.Mutex
	var s3PutAuth string
	var s3Deleted []string
	var webdavDeleted []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if strings.HasPrefix(r.URL.Path, "/mybucket") {
			switch r.Method {
			case http.MethodPut:
				s3PutAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Contents><Key>phoenix-backups/x-ui-backup-20260101-000000.db</Key></Contents>
  <Contents><Key>phoenix-backups/x-ui-backup-20260102-000000.db</Key></Contents>
  <Contents><Key>phoenix-backups/x-ui-backup-20260103-000000.db</Key></Contents>
</ListBucketResult>`))
			case http.MethodDelete:
				s3Deleted = append(s3Deleted, r.URL.Path)
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}

		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/dav/phoenix-backups/x-ui-backup-20260101-000000.db</d:href></d:response>
  <d:response><d:href>/dav/phoenix-backups/x-ui-backup-20260102-000000.db</d:href></d:response>
  <d:response><d:href>/dav/phoenix-backups/x-ui-backup-20260103-000000.db</d:href></d:response>
</d:multistatus>`))
		case http.MethodDelete:
			webdavDeleted = append(webdavDeleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	client := srv.Client()

	if err := uploadS3Object(ctx, client, srv.URL, "auto", "mybucket", "phoenix-backups/x-ui-backup-20260103-000000.db", "AKIA123", "SECRET456", []byte("sqlite-bytes")); err != nil {
		t.Fatalf("uploadS3Object failed: %v", err)
	}
	if !strings.HasPrefix(s3PutAuth, "AWS4-HMAC-SHA256 Credential=AKIA123/") {
		t.Fatalf("unexpected S3 Authorization header: %q", s3PutAuth)
	}
	if err := pruneS3Backups(ctx, client, srv.URL, "auto", "mybucket", "phoenix-backups/", "AKIA123", "SECRET456", 2); err != nil {
		t.Fatalf("pruneS3Backups failed: %v", err)
	}
	if len(s3Deleted) != 1 || !strings.HasSuffix(s3Deleted[0], "x-ui-backup-20260101-000000.db") {
		t.Fatalf("unexpected s3Deleted: %v", s3Deleted)
	}

	if err := uploadWebDAV(ctx, client, srv.URL+"/dav", "phoenix-backups/x-ui-backup-20260103-000000.db", "user", "pass", []byte("sqlite-bytes")); err != nil {
		t.Fatalf("uploadWebDAV failed: %v", err)
	}
	if err := pruneWebDAVBackups(ctx, client, srv.URL+"/dav", "phoenix-backups/", "user", "pass", 2); err != nil {
		t.Fatalf("pruneWebDAVBackups failed: %v", err)
	}
	if len(webdavDeleted) != 1 || !strings.HasSuffix(webdavDeleted[0], "x-ui-backup-20260101-000000.db") {
		t.Fatalf("unexpected webdavDeleted: %v", webdavDeleted)
	}
}

func TestCloudBackupGoogleDriveServiceAccountAndOAuthWithRetention(t *testing.T) {
	if got := extractGoogleDriveFolderID("https://drive.google.com/drive/folders/1AbCdEfGhIjKlMnOp?usp=sharing"); got != "1AbCdEfGhIjKlMnOp" {
		t.Fatalf("unexpected extracted folder ID: %q", got)
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})

	var mu sync.Mutex
	var uploadedBodies []string
	var deletedIDs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"ya29.test_token_123"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
			if got := r.Header.Get("Authorization"); got != "Bearer ya29.test_token_123" {
				t.Errorf("unexpected upload Authorization: %q", got)
			}
			b, _ := io.ReadAll(r.Body)
			uploadedBodies = append(uploadedBodies, string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"file-new","name":"x-ui-backup-20260103-000000.db"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/drive/v3/files":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"files":[
				{"id":"id-oldest","name":"x-ui-backup-20260101-000000.db"},
				{"id":"id-middle","name":"x-ui-backup-20260102-000000.db"},
				{"id":"id-newest","name":"x-ui-backup-20260103-000000.db"}
			]}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/drive/v3/files/"):
			deletedIDs = append(deletedIDs, strings.TrimPrefix(r.URL.Path, "/drive/v3/files/"))
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	saJSONBytes, _ := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": "backup-bot@phoenix-test.iam.gserviceaccount.com",
		"private_key":  string(pemBlock),
		"token_uri":    srv.URL + "/token",
	})

	ctx := context.Background()
	folderID := extractGoogleDriveFolderID("https://drive.google.com/drive/folders/1AbCdEfGhIjKlMnOp")
	uploadedKey, err := uploadGoogleDrive(ctx, srv.Client(), srv.URL, "", string(saJSONBytes), folderID, "x-ui-backup-20260103-000000.db", []byte("sqlite-db-payload"), 2)
	if err != nil {
		t.Fatalf("uploadGoogleDrive (service account) failed: %v", err)
	}
	if uploadedKey != "1AbCdEfGhIjKlMnOp/x-ui-backup-20260103-000000.db" {
		t.Fatalf("unexpected uploadedKey: %q", uploadedKey)
	}
	if len(uploadedBodies) != 1 || !strings.Contains(uploadedBodies[0], "1AbCdEfGhIjKlMnOp") || !strings.Contains(uploadedBodies[0], "sqlite-db-payload") {
		t.Fatalf("unexpected multipart upload body: %v", uploadedBodies)
	}
	if len(deletedIDs) != 1 || deletedIDs[0] != "id-oldest" {
		t.Fatalf("expected oldest Google Drive backup to be pruned, got deletedIDs=%v", deletedIDs)
	}
}
