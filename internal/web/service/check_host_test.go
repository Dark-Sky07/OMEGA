package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckHostServiceRunCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/check-tcp") {
			_, _ = w.Write([]byte(`{"ok":1,"request_id":"req-123"}`))
			return
		}
		if r.URL.Path == "/check-result/req-123" {
			_, _ = w.Write([]byte(`{
				"ir1.node.check-host.net": [{"time": 0.034, "address": "198.51.100.1"}],
				"ir3.node.check-host.net": [{"error": "Connection timed out"}],
				"ir5.node.check-host.net": [{"time": 0.041, "address": "198.51.100.1"}],
				"ir6.node.check-host.net": [{"time": 0.039, "address": "198.51.100.1"}],
				"de1.node.check-host.net": [{"time": 0.012, "address": "198.51.100.1"}]
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	svc := &CheckHostService{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}
	report, err := svc.RunCheck(context.Background(), "example.com:443", "tcp")
	if err != nil {
		t.Fatalf("RunCheck failed: %v", err)
	}
	if len(report.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(report.Nodes))
	}
	if !report.Nodes[0].Success || report.Nodes[0].LatencyMs != 34 {
		t.Fatalf("unexpected node[0]: %+v", report.Nodes[0])
	}
	if report.Nodes[1].Success || report.Nodes[1].Message != "Connection timed out" {
		t.Fatalf("unexpected node[1]: %+v", report.Nodes[1])
	}
}
