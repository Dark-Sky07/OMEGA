package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNormalizeCheckHostTarget(t *testing.T) {
	if got := normalizeCheckHostTarget("https://example.com:443/path", "ping"); got != "example.com" {
		t.Fatalf("normalizeCheckHostTarget(ping) = %q, want example.com", got)
	}
	if got := normalizeCheckHostTarget("198.51.100.10:2053", "ping"); got != "198.51.100.10" {
		t.Fatalf("normalizeCheckHostTarget(ping ip:port) = %q, want 198.51.100.10", got)
	}
	if got := normalizeCheckHostTarget("example.com", "tcp"); got != "example.com:443" {
		t.Fatalf("normalizeCheckHostTarget(tcp bare host) = %q, want example.com:443", got)
	}
	if got := normalizeCheckHostTarget("https://example.com:8443/", "tcp"); got != "example.com:8443" {
		t.Fatalf("normalizeCheckHostTarget(tcp url) = %q, want example.com:8443", got)
	}
}

func TestCheckHostServiceRunCheck(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/check-tcp") {
			_, _ = w.Write([]byte(`{
				"ok": 1,
				"request_id": "req-123",
				"permanent_link": "https://check-host.net/check-report/req-123",
				"nodes": {
					"ir1.node.check-host.net": ["ir", "Iran", "Tehran", "185.105.238.209", "AS47430"],
					"ir3.node.check-host.net": ["ir", "Iran", "Shiraz", "77.104.108.3", "AS213953"],
					"ir5.node.check-host.net": ["ir", "Iran", "Tehran", "88.135.75.5", "AS214431"],
					"de1.node.check-host.net": ["de", "Germany", "Nuremberg", "142.132.174.167", "AS24940"]
				}
			}`))
			return
		}
		if r.URL.Path == "/check-result/req-123" {
			n := atomic.AddInt32(&polls, 1)
			if n == 1 {
				// First poll: some nodes still null (pending)
				_, _ = w.Write([]byte(`{
					"ir1.node.check-host.net": null,
					"ir3.node.check-host.net": null,
					"ir5.node.check-host.net": null,
					"de1.node.check-host.net": [{"time": 0.012, "address": "198.51.100.1"}]
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"ir1.node.check-host.net": [{"time": 0.034, "address": "198.51.100.1"}],
				"ir3.node.check-host.net": [{"error": "Connection timed out"}],
				"ir5.node.check-host.net": [{"error": "Connection refused"}],
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
	report, err := svc.RunCheck(context.Background(), "example.com", "tcp")
	if err != nil {
		t.Fatalf("RunCheck failed: %v", err)
	}
	if report.Target != "example.com:443" {
		t.Fatalf("expected target example.com:443, got %q", report.Target)
	}
	if len(report.Nodes) != 4 {
		t.Fatalf("expected 4 scheduled nodes, got %d", len(report.Nodes))
	}
	if !report.Nodes[0].Success || report.Nodes[0].Status != "ok" || report.Nodes[0].LatencyMs != 34 {
		t.Fatalf("unexpected node[0]: %+v", report.Nodes[0])
	}
	if report.Nodes[1].Success || report.Nodes[1].Status != "filtered" {
		t.Fatalf("unexpected node[1]: %+v", report.Nodes[1])
	}
	if !report.Nodes[2].Success || report.Nodes[2].Status != "closed_port" {
		t.Fatalf("expected Connection refused to be classified as closed_port (IP reachable), got %+v", report.Nodes[2])
	}
}
