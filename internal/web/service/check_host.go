package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CheckHostNodeResult holds the probe result from a single Check-Host probe node.
type CheckHostNodeResult struct {
	Node      string `json:"node"`
	Location  string `json:"location"`
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latencyMs"`
	Message   string `json:"message,omitempty"`
}

// CheckHostReport is returned by CheckHostService.RunCheck.
type CheckHostReport struct {
	Target    string                `json:"target"`
	Mode      string                `json:"mode"`
	CheckedAt int64                 `json:"checkedAt"`
	Nodes     []CheckHostNodeResult `json:"nodes"`
}

var defaultIranCheckHostNodes = []struct {
	Node     string
	Location string
}{
	{Node: "ir1.node.check-host.net", Location: "🇮🇷 Iran — Tehran"},
	{Node: "ir3.node.check-host.net", Location: "🇮🇷 Iran — Shiraz"},
	{Node: "ir5.node.check-host.net", Location: "🇮🇷 Iran — Tabriz"},
	{Node: "ir6.node.check-host.net", Location: "🇮🇷 Iran — Isfahan"},
	{Node: "de1.node.check-host.net", Location: "🇩🇪 Germany — Frankfurt (Ref)"},
}

// CheckHostService queries check-host.net from Iranian & reference nodes to detect IP/port filtering.
type CheckHostService struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (s *CheckHostService) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return "https://check-host.net"
}

func (s *CheckHostService) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (s *CheckHostService) RunCheck(ctx context.Context, target string, mode string) (*CheckHostReport, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("target host or host:port is required")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "ping" {
		mode = "tcp"
	}

	endpoint := "/check-tcp"
	if mode == "ping" {
		endpoint = "/check-ping"
	}

	q := url.Values{}
	q.Set("host", target)
	for _, n := range defaultIranCheckHostNodes {
		q.Add("node", n.Node)
	}

	reqURL := s.baseURL() + endpoint + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	resp.Body.Close()

	var initPayload struct {
		Ok        int    `json:"ok"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(body, &initPayload); err != nil {
		return nil, fmt.Errorf("failed to parse check-host response: %w", err)
	}
	if initPayload.RequestID == "" {
		if initPayload.Error != "" {
			return nil, errors.New(initPayload.Error)
		}
		return nil, errors.New("check-host did not return a request_id")
	}

	resURL := fmt.Sprintf("%s/check-result/%s", s.baseURL(), url.PathEscape(initPayload.RequestID))
	rawResults := make(map[string]any)

	for attempt := range 4 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(1400 * time.Millisecond):
			}
		}
		rReq, err := http.NewRequestWithContext(ctx, http.MethodGet, resURL, nil)
		if err != nil {
			break
		}
		rReq.Header.Set("Accept", "application/json")
		rResp, err := s.client().Do(rReq)
		if err != nil {
			continue
		}
		rBody, _ := io.ReadAll(io.LimitReader(rResp.Body, 512*1024))
		rResp.Body.Close()
		if err := json.Unmarshal(rBody, &rawResults); err != nil {
			continue
		}
		allReady := true
		for _, n := range defaultIranCheckHostNodes {
			if v, exists := rawResults[n.Node]; !exists || v == nil {
				allReady = false
				break
			}
		}
		if allReady {
			break
		}
	}

	report := &CheckHostReport{
		Target:    target,
		Mode:      mode,
		CheckedAt: time.Now().UnixMilli(),
		Nodes:     make([]CheckHostNodeResult, 0, len(defaultIranCheckHostNodes)),
	}
	for _, n := range defaultIranCheckHostNodes {
		nr := parseCheckHostNodeEntry(n.Node, n.Location, mode, rawResults[n.Node])
		report.Nodes = append(report.Nodes, nr)
	}
	return report, nil
}

func parseCheckHostNodeEntry(node, location, mode string, raw any) CheckHostNodeResult {
	res := CheckHostNodeResult{
		Node:     node,
		Location: location,
	}
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		res.Message = "Pending / timeout"
		return res
	}
	if mode == "tcp" {
		if first, ok := arr[0].(map[string]any); ok {
			if errStr, hasErr := first["error"].(string); hasErr && errStr != "" {
				res.Success = false
				res.Message = errStr
				return res
			}
			if tVal, ok := first["time"].(float64); ok {
				res.Success = true
				res.LatencyMs = max(int64(tVal*1000), 1)
				if addr, _ := first["address"].(string); addr != "" {
					res.Message = addr
				} else {
					res.Message = "Connected"
				}
				return res
			}
		}
	} else {
		// Ping format: [[["OK", 0.042, "1.2.3.4"], ...]]
		if outer, ok := arr[0].([]any); ok {
			var totalSec float64
			var okCount int
			for _, pkt := range outer {
				pArr, ok := pkt.([]any)
				if !ok || len(pArr) < 2 {
					continue
				}
				status, _ := pArr[0].(string)
				rtt, _ := pArr[1].(float64)
				if strings.EqualFold(status, "OK") {
					okCount++
					totalSec += rtt
				}
			}
			if okCount > 0 {
				res.Success = true
				res.LatencyMs = max(int64((totalSec/float64(okCount))*1000), 1)
				res.Message = fmt.Sprintf("%d/%d packets OK", okCount, len(outer))
				return res
			}
			res.Message = "100% packet loss"
			return res
		}
	}
	res.Message = "Unreachable"
	return res
}
