package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	Status    string `json:"status"` // "ok", "closed_port", "pending", "filtered"
	LatencyMs int64  `json:"latencyMs"`
	Message   string `json:"message,omitempty"`
}

// CheckHostReport is returned by CheckHostService.RunCheck.
type CheckHostReport struct {
	Target        string                `json:"target"`
	Mode          string                `json:"mode"`
	PermanentLink string                `json:"permanentLink,omitempty"`
	CheckedAt     int64                 `json:"checkedAt"`
	Nodes         []CheckHostNodeResult `json:"nodes"`
}

type checkHostNodeSpec struct {
	Node     string
	Location string
}

var defaultIranCheckHostNodes = []checkHostNodeSpec{
	{Node: "ir1.node.check-host.net", Location: "🇮🇷 Iran — Tehran (ir1)"},
	{Node: "ir2.node.check-host.net", Location: "🇮🇷 Iran — Isfahan (ir2)"},
	{Node: "ir3.node.check-host.net", Location: "🇮🇷 Iran — Shiraz (ir3)"},
	{Node: "ir4.node.check-host.net", Location: "🇮🇷 Iran — Shiraz (ir4)"},
	{Node: "ir5.node.check-host.net", Location: "🇮🇷 Iran — Tehran (ir5)"},
	{Node: "ir6.node.check-host.net", Location: "🇮🇷 Iran — Qom (ir6)"},
	{Node: "ir7.node.check-host.net", Location: "🇮🇷 Iran — Tehran (ir7)"},
	{Node: "de1.node.check-host.net", Location: "🇩🇪 Germany — Nuremberg (Ref)"},
	{Node: "nl1.node.check-host.net", Location: "🇳🇱 Netherlands — Amsterdam (Ref)"},
}

// CheckHostService queries check-host.net from Iranian & reference nodes to detect IP/port filtering.
type CheckHostService struct {
	BaseURL      string
	HTTPClient   *http.Client
	InitialDelay time.Duration
	PollInterval time.Duration
	MaxPolls     int
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

func normalizeCheckHostTarget(rawTarget, mode string) string {
	t := strings.TrimSpace(rawTarget)
	for _, prefix := range []string{"https://", "http://", "tcp://"} {
		if strings.HasPrefix(strings.ToLower(t), prefix) {
			t = t[len(prefix):]
			break
		}
	}
	if slash := strings.IndexByte(t, '/'); slash >= 0 {
		t = t[:slash]
	}
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}

	if mode == "ping" {
		if h, _, err := net.SplitHostPort(t); err == nil && h != "" {
			return h
		}
		if strings.Count(t, ":") == 1 {
			if idx := strings.LastIndexByte(t, ':'); idx > 0 {
				return t[:idx]
			}
		}
		return strings.Trim(t, "[]")
	}

	// TCP mode requires host:port. If no port was given, default to :443.
	if _, _, err := net.SplitHostPort(t); err == nil {
		return t
	}
	if strings.Contains(t, ":") && !strings.HasPrefix(t, "[") {
		// Unbracketed IPv6 literal
		return "[" + t + "]:443"
	}
	return t + ":443"
}

func isLocalOrSelfHost(host string) bool {
	h := strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if h == "" || h == "self" || h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()
	}
	return false
}

func (s *CheckHostService) resolvePublicIP(ctx context.Context) string {
	for _, ep := range []string{"https://api.ipify.org", "https://icanhazip.com"} {
		rCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, err := http.NewRequestWithContext(rCtx, http.MethodGet, ep, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := s.client().Do(req)
		if err != nil {
			cancel()
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
		resp.Body.Close()
		cancel()
		candidate := strings.TrimSpace(string(b))
		if ip := net.ParseIP(candidate); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
			return candidate
		}
	}
	return ""
}

func (s *CheckHostService) RunCheck(ctx context.Context, target string, mode string) (*CheckHostReport, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "ping" {
		mode = "tcp"
	}
	target = normalizeCheckHostTarget(target, mode)
	if target == "" {
		return nil, errors.New("target host or host:port is required")
	}
	if s.BaseURL == "" {
		if mode == "ping" {
			if isLocalOrSelfHost(target) {
				if pubIP := s.resolvePublicIP(ctx); pubIP != "" {
					target = pubIP
				}
			}
		} else if h, p, err := net.SplitHostPort(target); err == nil && isLocalOrSelfHost(h) {
			if pubIP := s.resolvePublicIP(ctx); pubIP != "" {
				target = net.JoinHostPort(pubIP, p)
			}
		}
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PHOENIX-Panel/5.0; +https://github.com/Dark-Sky07/OMEGA)")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	resp.Body.Close()

	var initPayload struct {
		Ok            int                 `json:"ok"`
		RequestID     string              `json:"request_id"`
		PermanentLink string              `json:"permanent_link"`
		Error         string              `json:"error"`
		Nodes         map[string][]string `json:"nodes"`
	}
	if err := json.Unmarshal(body, &initPayload); err != nil {
		return nil, fmt.Errorf("check-host response error (rate limit or invalid target): %w", err)
	}
	if initPayload.RequestID == "" {
		if initPayload.Error != "" {
			return nil, errors.New(initPayload.Error)
		}
		return nil, errors.New("check-host rate limit reached for this target — please wait a minute and retry")
	}

	// Build active node list in deterministic order from defaultIranCheckHostNodes,
	// filtering to nodes actually scheduled by check-host.net when initPayload.Nodes is present.
	activeNodes := make([]checkHostNodeSpec, 0, len(defaultIranCheckHostNodes))
	if len(initPayload.Nodes) > 0 {
		for _, spec := range defaultIranCheckHostNodes {
			if meta, ok := initPayload.Nodes[spec.Node]; ok {
				loc := spec.Location
				if len(meta) >= 3 && meta[2] != "" {
					flag := "🌐"
					switch strings.ToLower(meta[0]) {
					case "ir":
						flag = "🇮🇷"
					case "de":
						flag = "🇩🇪"
					case "nl":
						flag = "🇳🇱"
					}
					loc = fmt.Sprintf("%s %s — %s", flag, meta[1], meta[2])
					if len(meta) >= 5 && meta[4] != "" {
						loc += fmt.Sprintf(" (%s)", meta[4])
					}
				}
				activeNodes = append(activeNodes, checkHostNodeSpec{Node: spec.Node, Location: loc})
			}
		}
	}
	if len(activeNodes) == 0 {
		activeNodes = defaultIranCheckHostNodes
	}

	resURL := fmt.Sprintf("%s/check-result/%s", s.baseURL(), url.PathEscape(initPayload.RequestID))
	rawResults := make(map[string]any, len(activeNodes))

	initDelay := s.InitialDelay
	if initDelay == 0 {
		if s.BaseURL != "" {
			initDelay = 5 * time.Millisecond
		} else {
			initDelay = 2000 * time.Millisecond
		}
	}
	pollInterval := s.PollInterval
	if pollInterval == 0 {
		if s.BaseURL != "" {
			pollInterval = 10 * time.Millisecond
		} else {
			pollInterval = 1400 * time.Millisecond
		}
	}
	maxPolls := s.MaxPolls
	if maxPolls <= 0 {
		maxPolls = 8
	}

	for attempt := range maxPolls {
		waitDur := pollInterval
		if attempt == 0 {
			waitDur = initDelay
		}
		select {
		case <-ctx.Done():
			break
		case <-time.After(waitDur):
		}

		rReq, err := http.NewRequestWithContext(ctx, http.MethodGet, resURL, nil)
		if err != nil {
			break
		}
		rReq.Header.Set("Accept", "application/json")
		rReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PHOENIX-Panel/5.0; +https://github.com/Dark-Sky07/OMEGA)")
		rResp, err := s.client().Do(rReq)
		if err != nil {
			continue
		}
		rBody, _ := io.ReadAll(io.LimitReader(rResp.Body, 512*1024))
		rResp.Body.Close()

		var pollMap map[string]any
		if err := json.Unmarshal(rBody, &pollMap); err != nil {
			continue
		}
		for k, v := range pollMap {
			if v != nil {
				rawResults[k] = v
			}
		}

		allReady := true
		for _, n := range activeNodes {
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
		Target:        target,
		Mode:          mode,
		PermanentLink: initPayload.PermanentLink,
		CheckedAt:     time.Now().UnixMilli(),
		Nodes:         make([]CheckHostNodeResult, 0, len(activeNodes)),
	}
	for _, n := range activeNodes {
		nr := parseCheckHostNodeEntry(n.Node, n.Location, mode, rawResults[n.Node])
		report.Nodes = append(report.Nodes, nr)
	}
	return report, nil
}

func parseCheckHostNodeEntry(node, location, mode string, raw any) CheckHostNodeResult {
	res := CheckHostNodeResult{
		Node:     node,
		Location: location,
		Status:   "pending",
	}
	if raw == nil {
		res.Message = "Node busy / still checking"
		return res
	}
	if obj, ok := raw.(map[string]any); ok {
		raw = []any{obj}
	}
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		res.Message = "Node busy / still checking"
		return res
	}
	if len(arr) == 1 && arr[0] == nil {
		res.Success = false
		res.Status = "filtered"
		res.Message = "DNS / host resolution failed"
		return res
	}
	if mode == "tcp" {
		if first, ok := arr[0].(map[string]any); ok {
			if errStr, hasErr := first["error"].(string); hasErr && errStr != "" {
				lowerErr := strings.ToLower(errStr)
				if strings.Contains(lowerErr, "refused") {
					// TCP RST received from target host: IP is reachable (not filtered),
					// only the tested TCP port is closed on the server.
					res.Success = true
					res.Status = "closed_port"
					res.Message = "IP Reachable (" + errStr + " — port closed)"
					return res
				}
				res.Success = false
				res.Status = "filtered"
				res.Message = errStr
				return res
			}
			if tVal, ok := first["time"].(float64); ok {
				res.Success = true
				res.Status = "ok"
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
			if len(outer) == 1 && outer[0] == nil {
				res.Success = false
				res.Status = "filtered"
				res.Message = "DNS / host resolution failed"
				return res
			}
			var totalSec float64
			var okCount int
			var resolvedIP string
			for _, pkt := range outer {
				pArr, ok := pkt.([]any)
				if !ok || len(pArr) < 2 {
					continue
				}
				status, _ := pArr[0].(string)
				rtt, _ := pArr[1].(float64)
				if len(pArr) >= 3 && resolvedIP == "" {
					if ipStr, ok := pArr[2].(string); ok {
						resolvedIP = ipStr
					}
				}
				if strings.EqualFold(status, "OK") {
					okCount++
					totalSec += rtt
				}
			}
			if okCount > 0 {
				res.Success = true
				res.Status = "ok"
				res.LatencyMs = max(int64((totalSec/float64(okCount))*1000), 1)
				msg := fmt.Sprintf("%d/%d packets OK", okCount, len(outer))
				if resolvedIP != "" {
					msg += " (" + resolvedIP + ")"
				}
				res.Message = msg
				return res
			}
			res.Success = false
			res.Status = "filtered"
			res.Message = "100% packet loss (ICMP blocked or host filtered)"
			return res
		}
	}
	res.Success = false
	res.Status = "filtered"
	res.Message = "Unreachable"
	return res
}
