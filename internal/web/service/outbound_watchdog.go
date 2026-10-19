package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/outbound"
)

var (
	watchdogMu      sync.Mutex
	watchdogLastRun time.Time
	watchdogState   = &OutboundWatchdogStatus{
		FailCounts:  make(map[string]int),
		LastResults: make(map[string]outbound.TestOutboundResult),
	}
)

// OutboundWatchdogStatus reports the current state of the Smart Outbound Auto-Failover Watchdog.
type OutboundWatchdogStatus struct {
	Enabled          bool                                   `json:"enabled"`
	ActiveTag        string                                 `json:"activeTag"`
	CandidateTags    []string                               `json:"candidateTags"`
	LastCheckedAt    int64                                  `json:"lastCheckedAt"`
	LastFailoverAt   int64                                  `json:"lastFailoverAt"`
	LastFailoverFrom string                                 `json:"lastFailoverFrom,omitempty"`
	LastFailoverTo   string                                 `json:"lastFailoverTo,omitempty"`
	FailCounts       map[string]int                         `json:"failCounts"`
	LastResults      map[string]outbound.TestOutboundResult `json:"lastResults"`
}

// OutboundWatchdogService monitors candidate Xray outbounds and automatically promotes a healthy backup outbound when the primary fails.
type OutboundWatchdogService struct {
	settingService     SettingService
	xraySettingService XraySettingService
	outboundService    outbound.OutboundService
	xrayService        XrayService
	discordService     DiscordService
	auditLogService    AuditLogService
}

// Status returns a snapshot of the current watchdog state and candidate tags.
func (s *OutboundWatchdogService) Status() *OutboundWatchdogStatus {
	enabled, _ := s.settingService.GetOutboundWatchdogEnable()
	configuredTags, _ := s.settingService.GetOutboundWatchdogTags()
	tpl, _ := s.settingService.GetXrayConfigTemplate()
	candidates, active := extractWatchdogCandidates(tpl, configuredTags)

	watchdogMu.Lock()
	defer watchdogMu.Unlock()

	cp := &OutboundWatchdogStatus{
		Enabled:          enabled,
		ActiveTag:        active,
		CandidateTags:    candidates,
		LastCheckedAt:    watchdogState.LastCheckedAt,
		LastFailoverAt:   watchdogState.LastFailoverAt,
		LastFailoverFrom: watchdogState.LastFailoverFrom,
		LastFailoverTo:   watchdogState.LastFailoverTo,
		FailCounts:       make(map[string]int, len(watchdogState.FailCounts)),
		LastResults:      make(map[string]outbound.TestOutboundResult, len(watchdogState.LastResults)),
	}
	maps.Copy(cp.FailCounts, watchdogState.FailCounts)
	maps.Copy(cp.LastResults, watchdogState.LastResults)
	return cp
}

// RunScheduledIfDue runs a watchdog check when enabled and the configured interval has elapsed.
func (s *OutboundWatchdogService) RunScheduledIfDue() {
	enabled, err := s.settingService.GetOutboundWatchdogEnable()
	if err != nil || !enabled {
		return
	}
	intervalSec, _ := s.settingService.GetOutboundWatchdogIntervalSec()
	if intervalSec < 15 {
		intervalSec = 120
	}
	interval := time.Duration(intervalSec) * time.Second

	watchdogMu.Lock()
	if !watchdogLastRun.IsZero() && time.Since(watchdogLastRun) < interval {
		watchdogMu.Unlock()
		return
	}
	watchdogLastRun = time.Now()
	watchdogMu.Unlock()

	if _, err := s.CheckAndFailover(false); err != nil {
		logger.Warningf("outbound watchdog check warning: %v", err)
	}
}

// CheckAndFailover probes the active primary outbound and, if it exceeds the failure threshold,
// tests backup candidates and promotes the fastest healthy candidate.
func (s *OutboundWatchdogService) CheckAndFailover(force bool) (*OutboundWatchdogStatus, error) {
	tpl, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}
	tpl = UnwrapXrayTemplateConfig(tpl)
	configuredTags, _ := s.settingService.GetOutboundWatchdogTags()
	threshold, _ := s.settingService.GetOutboundWatchdogFailThreshold()
	if threshold <= 0 {
		threshold = 2
	}
	testURL, _ := s.settingService.GetXrayOutboundTestUrl()
	if strings.TrimSpace(testURL) == "" {
		testURL = "https://www.google.com/generate_204"
	}

	candidates, activeTag := extractWatchdogCandidates(tpl, configuredTags)
	if len(candidates) == 0 || activeTag == "" {
		return s.Status(), nil
	}

	outboundsByTag := extractOutboundRawByTag(tpl)
	activeRaw, ok := outboundsByTag[activeTag]
	if !ok {
		return s.Status(), nil
	}

	res, err := s.outboundService.TestOutbound(string(activeRaw), testURL, tpl, "http")
	nowMs := time.Now().UnixMilli()

	watchdogMu.Lock()
	watchdogState.LastCheckedAt = nowMs
	if res != nil {
		watchdogState.LastResults[activeTag] = *res
	}
	if err == nil && res != nil && res.Success {
		watchdogState.FailCounts[activeTag] = 0
		watchdogMu.Unlock()
		return s.Status(), nil
	}
	watchdogState.FailCounts[activeTag]++
	fails := watchdogState.FailCounts[activeTag]
	watchdogMu.Unlock()

	if (!force && fails < threshold) || len(candidates) < 2 {
		return s.Status(), nil
	}

	// Probe backup candidates to find the fastest healthy outbound.
	var backupList []json.RawMessage
	var backupTags []string
	for _, tag := range candidates {
		if tag == activeTag {
			continue
		}
		if raw, exists := outboundsByTag[tag]; exists {
			backupList = append(backupList, raw)
			backupTags = append(backupTags, tag)
		}
	}
	if len(backupList) == 0 {
		return s.Status(), nil
	}
	batchBytes, err := json.Marshal(backupList)
	if err != nil {
		return s.Status(), err
	}
	results, err := s.outboundService.TestOutbounds(string(batchBytes), testURL, tpl, "http")
	if err != nil {
		return s.Status(), err
	}

	bestIdx := -1
	var bestDelay int64 = 1<<62 - 1
	watchdogMu.Lock()
	for i, r := range results {
		if i < len(backupTags) {
			watchdogState.LastResults[backupTags[i]] = r
		}
		if r.Success && r.Delay > 0 && r.Delay < bestDelay {
			bestDelay = r.Delay
			bestIdx = i
		}
	}
	watchdogMu.Unlock()

	if bestIdx < 0 || bestIdx >= len(backupTags) {
		return s.Status(), nil
	}
	bestTag := backupTags[bestIdx]
	bestRes := results[bestIdx]

	updatedTpl, changed, err := promoteOutboundInTemplate(tpl, activeTag, bestTag)
	if err != nil || !changed {
		return s.Status(), err
	}
	if err := s.xraySettingService.SaveXraySetting(updatedTpl); err != nil {
		return s.Status(), err
	}
	if err := s.xrayService.RestartXray(false); err != nil {
		logger.Warningf("outbound watchdog restart xray warning: %v", err)
	}

	watchdogMu.Lock()
	watchdogState.FailCounts[activeTag] = 0
	watchdogState.FailCounts[bestTag] = 0
	watchdogState.LastFailoverAt = time.Now().UnixMilli()
	watchdogState.LastFailoverFrom = activeTag
	watchdogState.LastFailoverTo = bestTag
	watchdogMu.Unlock()

	s.auditLogService.Record(&model.AuditLog{
		ActorType: "system",
		ActorName: "OutboundWatchdog",
		Action:    "FAILOVER_OUTBOUND",
		Module:    "xray",
		Target:    fmt.Sprintf("%s -> %s (%d ms)", activeTag, bestTag, bestRes.Delay),
		Status:    200,
	})
	s.notifyFailover(activeTag, bestTag, bestRes)

	return s.Status(), nil
}

func (s *OutboundWatchdogService) notifyFailover(fromTag, toTag string, r outbound.TestOutboundResult) {
	geo := ""
	if r.CountryCode != "" || r.EgressIP != "" {
		geo = fmt.Sprintf(" [%s %s]", r.CountryCode, r.EgressIP)
	}
	msg := fmt.Sprintf("⚠️ OMEGA Outbound Auto-Failover\nFailed Outbound: %s\nPromoted Outbound: %s (%d ms)%s", fromTag, toTag, r.Delay, geo)

	if s.discordService.IsEnabled() {
		_ = s.discordService.SendEmbed(DiscordEmbed{
			Title:       "⚠️ Outbound Auto-Failover Triggered",
			Description: fmt.Sprintf("Primary outbound **%s** failed health checks and was automatically switched to **%s** (`%d ms`%s).", fromTag, toTag, r.Delay, geo),
			Color:       discordColorWarn,
		})
	}

	tgEnabled, _ := s.settingService.GetTgbotEnabled()
	tgToken, _ := s.settingService.GetTgBotToken()
	tgChats, _ := s.settingService.GetTgBotChatId()
	if tgEnabled && strings.TrimSpace(tgToken) != "" && strings.TrimSpace(tgChats) != "" {
		client := &http.Client{Timeout: 8 * time.Second}
		for _, rawChat := range strings.Split(tgChats, ",") {
			chatID := strings.TrimSpace(rawChat)
			if chatID == "" {
				continue
			}
			body, _ := json.Marshal(map[string]any{
				"chat_id": chatID,
				"text":    msg,
			})
			u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", strings.TrimSpace(tgToken))
			if resp, err := client.Post(u, "application/json", bytes.NewReader(body)); err == nil {
				resp.Body.Close()
			}
		}
	}
}

func isSystemOutboundProtocol(proto string) bool {
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "freedom", "blackhole", "dns", "loopback":
		return true
	default:
		return false
	}
}

func extractOutboundRawByTag(templateJSON string) map[string]json.RawMessage {
	res := make(map[string]json.RawMessage)
	var root map[string]any
	if err := json.Unmarshal([]byte(templateJSON), &root); err != nil {
		return res
	}
	rawList, _ := root["outbounds"].([]any)
	for _, item := range rawList {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag == "" {
			continue
		}
		if b, err := json.Marshal(m); err == nil {
			res[tag] = b
		}
	}
	return res
}

func extractWatchdogCandidates(templateJSON, configuredTags string) ([]string, string) {
	var root map[string]any
	if err := json.Unmarshal([]byte(templateJSON), &root); err != nil {
		return nil, ""
	}
	rawList, _ := root["outbounds"].([]any)
	var allOrdered []string
	allSet := make(map[string]bool)
	nonSystemSet := make(map[string]bool)
	for _, item := range rawList {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if enabled, ok := m["enabled"].(bool); ok && !enabled {
			continue
		}
		tag, _ := m["tag"].(string)
		proto, _ := m["protocol"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == "api" {
			continue
		}
		allOrdered = append(allOrdered, tag)
		allSet[tag] = true
		if !isSystemOutboundProtocol(proto) {
			nonSystemSet[tag] = true
		}
	}

	var candidates []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(configuredTags, ",") {
		t := strings.TrimSpace(part)
		if t != "" && allSet[t] && !seen[t] {
			candidates = append(candidates, t)
			seen[t] = true
		}
	}
	if len(candidates) == 0 {
		for _, t := range allOrdered {
			if nonSystemSet[t] && !seen[t] {
				candidates = append(candidates, t)
				seen[t] = true
			}
		}
	}
	if len(candidates) == 0 {
		return nil, ""
	}
	// Active primary is whichever candidate appears first in the current Xray outbounds array.
	candSet := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		candSet[c] = true
	}
	active := candidates[0]
	for _, t := range allOrdered {
		if candSet[t] {
			active = t
			break
		}
	}
	return candidates, active
}

// promoteOutboundInTemplate swaps the positions of failedTag and healthyTag in outbounds
// and updates any routing rules pointing at failedTag to point at healthyTag.
func promoteOutboundInTemplate(templateJSON, failedTag, healthyTag string) (string, bool, error) {
	if failedTag == "" || healthyTag == "" || failedTag == healthyTag {
		return templateJSON, false, nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(templateJSON), &root); err != nil {
		return "", false, err
	}
	rawList, ok := root["outbounds"].([]any)
	if !ok {
		return templateJSON, false, nil
	}
	idxFailed, idxHealthy := -1, -1
	for i, item := range rawList {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag == failedTag && idxFailed < 0 {
			idxFailed = i
		} else if tag == healthyTag && idxHealthy < 0 {
			idxHealthy = i
		}
	}
	if idxFailed < 0 || idxHealthy < 0 {
		return templateJSON, false, nil
	}
	rawList[idxFailed], rawList[idxHealthy] = rawList[idxHealthy], rawList[idxFailed]
	root["outbounds"] = rawList

	if routing, ok := root["routing"].(map[string]any); ok {
		if rules, ok := routing["rules"].([]any); ok {
			for i, r := range rules {
				rm, ok := r.(map[string]any)
				if !ok {
					continue
				}
				if ot, _ := rm["outboundTag"].(string); ot == failedTag {
					rm["outboundTag"] = healthyTag
					rules[i] = rm
				}
			}
			routing["rules"] = rules
			root["routing"] = routing
		}
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}
