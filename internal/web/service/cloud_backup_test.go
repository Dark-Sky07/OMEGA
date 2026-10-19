package service

import (
	"context"
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
  <Contents><Key>omega-backups/x-ui-backup-20260101-000000.db</Key></Contents>
  <Contents><Key>omega-backups/x-ui-backup-20260102-000000.db</Key></Contents>
  <Contents><Key>omega-backups/x-ui-backup-20260103-000000.db</Key></Contents>
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
  <d:response><d:href>/dav/omega-backups/x-ui-backup-20260101-000000.db</d:href></d:response>
  <d:response><d:href>/dav/omega-backups/x-ui-backup-20260102-000000.db</d:href></d:response>
  <d:response><d:href>/dav/omega-backups/x-ui-backup-20260103-000000.db</d:href></d:response>
</d:multistatus>`))
		case http.MethodDelete:
			webdavDeleted = append(webdavDeleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	client := srv.Client()

	if err := uploadS3Object(ctx, client, srv.URL, "auto", "mybucket", "omega-backups/x-ui-backup-20260103-000000.db", "AKIA123", "SECRET456", []byte("sqlite-bytes")); err != nil {
		t.Fatalf("uploadS3Object failed: %v", err)
	}
	if !strings.HasPrefix(s3PutAuth, "AWS4-HMAC-SHA256 Credential=AKIA123/") {
		t.Fatalf("unexpected S3 Authorization header: %q", s3PutAuth)
	}
	if err := pruneS3Backups(ctx, client, srv.URL, "auto", "mybucket", "omega-backups/", "AKIA123", "SECRET456", 2); err != nil {
		t.Fatalf("pruneS3Backups failed: %v", err)
	}
	if len(s3Deleted) != 1 || !strings.HasSuffix(s3Deleted[0], "x-ui-backup-20260101-000000.db") {
		t.Fatalf("unexpected s3Deleted: %v", s3Deleted)
	}

	if err := uploadWebDAV(ctx, client, srv.URL+"/dav", "omega-backups/x-ui-backup-20260103-000000.db", "user", "pass", []byte("sqlite-bytes")); err != nil {
		t.Fatalf("uploadWebDAV failed: %v", err)
	}
	if err := pruneWebDAVBackups(ctx, client, srv.URL+"/dav", "omega-backups/", "user", "pass", 2); err != nil {
		t.Fatalf("pruneWebDAVBackups failed: %v", err)
	}
	if len(webdavDeleted) != 1 || !strings.HasSuffix(webdavDeleted[0], "x-ui-backup-20260101-000000.db") {
		t.Fatalf("unexpected webdavDeleted: %v", webdavDeleted)
	}
}
