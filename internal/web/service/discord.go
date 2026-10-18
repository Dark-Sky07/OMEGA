package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// DiscordService delivers panel notifications (periodic reports, login alerts,
// and approaching-limit warnings) to Discord via either an incoming Webhook
// URL or a Bot Token + Channel ID pair.
type DiscordService struct {
	settingService SettingService
	inboundService InboundService
	xrayService    XrayService
	// APIBaseURL overrides "https://discord.com/api/v10" in unit tests.
	APIBaseURL string
	// HTTPClient overrides the default 10s client in unit tests.
	HTTPClient *http.Client
}

// DiscordEmbedField is a single name/value pair inside a Discord rich embed.
type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// DiscordEmbed is the Discord rich embed payload.
type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type discordMessagePayload struct {
	Username string         `json:"username,omitempty"`
	Content  string         `json:"content,omitempty"`
	Embeds   []DiscordEmbed `json:"embeds,omitempty"`
}

const (
	discordColorInfo    = 0x6366F1 // Indigo (OMEGA brand)
	discordColorSuccess = 0x10B981 // Emerald
	discordColorWarn    = 0xF59E0B // Amber
	discordColorDanger  = 0xEF4444 // Red
)

func (d *DiscordService) client() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (d *DiscordService) apiBase() string {
	if d.APIBaseURL != "" {
		return strings.TrimRight(d.APIBaseURL, "/")
	}
	return "https://discord.com/api/v10"
}

// IsEnabled reports whether Discord notifications are turned on and at least
// one delivery target (Webhook URL or Bot Token + Channel ID) is configured.
func (d *DiscordService) IsEnabled() bool {
	enabled, err := d.settingService.GetDiscordEnable()
	if err != nil || !enabled {
		return false
	}
	webhook, _ := d.settingService.GetDiscordWebhookURL()
	if strings.TrimSpace(webhook) != "" {
		return true
	}
	token, _ := d.settingService.GetDiscordBotToken()
	channelID, _ := d.settingService.GetDiscordChannelId()
	return strings.TrimSpace(token) != "" && strings.TrimSpace(channelID) != ""
}

// SendEmbed delivers embed to the configured Discord Webhook and/or Bot channel.
func (d *DiscordService) SendEmbed(embed DiscordEmbed) error {
	if embed.Timestamp == "" {
		embed.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	body, err := json.Marshal(discordMessagePayload{
		Username: "OMEGA Panel",
		Embeds:   []DiscordEmbed{embed},
	})
	if err != nil {
		return err
	}

	webhook, _ := d.settingService.GetDiscordWebhookURL()
	webhook = strings.TrimSpace(webhook)
	token, _ := d.settingService.GetDiscordBotToken()
	token = strings.TrimSpace(token)
	channelID, _ := d.settingService.GetDiscordChannelId()
	channelID = strings.TrimSpace(channelID)

	if webhook == "" && (token == "" || channelID == "") {
		return common.NewError("Discord webhook URL or bot token + channel ID is not configured")
	}

	var firstErr error
	if webhook != "" {
		if err := d.postJSON(webhook, "", body); err != nil {
			firstErr = err
		}
	}
	if token != "" && channelID != "" {
		url := fmt.Sprintf("%s/channels/%s/messages", d.apiBase(), channelID)
		if err := d.postJSON(url, "Bot "+token, body); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *DiscordService) postJSON(url string, authHeader string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("discord HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// SendTestMessage sends a verification embed so admins can confirm their
// webhook or bot credentials straight from the Settings page.
func (d *DiscordService) SendTestMessage() error {
	host, _ := os.Hostname()
	if host == "" {
		host = "omega"
	}
	return d.SendEmbed(DiscordEmbed{
		Title:       "✅ OMEGA Discord Notification Test",
		Description: "Your Discord notification channel is connected and working.",
		Color:       discordColorSuccess,
		Fields: []DiscordEmbedField{
			{Name: "Host", Value: host, Inline: true},
			{Name: "Xray", Value: d.xrayService.GetXrayVersion(), Inline: true},
		},
	})
}

// SendLoginNotify posts a login success/failure alert when Discord login
// notifications are enabled.
func (d *DiscordService) SendLoginNotify(username, ip, when, reason string, success bool) {
	if !d.IsEnabled() {
		return
	}
	notify, err := d.settingService.GetDiscordLoginNotify()
	if err != nil || !notify {
		return
	}
	host, _ := os.Hostname()
	title := "🔐 Panel Login Succeeded"
	color := discordColorSuccess
	if !success {
		title = "🚨 Panel Login Failed"
		color = discordColorDanger
	}
	fields := []DiscordEmbedField{
		{Name: "Host", Value: nonEmpty(host, "-"), Inline: true},
		{Name: "User", Value: nonEmpty(username, "-"), Inline: true},
		{Name: "IP", Value: nonEmpty(ip, "-"), Inline: true},
		{Name: "Time", Value: nonEmpty(when, "-"), Inline: false},
	}
	if !success && reason != "" {
		fields = append(fields, DiscordEmbedField{Name: "Reason", Value: reason, Inline: false})
	}
	if err := d.SendEmbed(DiscordEmbed{
		Title:  title,
		Color:  color,
		Fields: fields,
	}); err != nil {
		logger.Warningf("Discord login notification failed: %v", err)
	}
}

// SendPeriodicReport posts a summary of inbound/client traffic and any clients
// approaching their expiry or quota threshold.
func (d *DiscordService) SendPeriodicReport() {
	if !d.IsEnabled() {
		return
	}
	inbounds, err := d.inboundService.GetAllInbounds()
	if err != nil {
		return
	}
	var totalUp, totalDown int64
	for _, ib := range inbounds {
		totalUp += ib.Up
		totalDown += ib.Down
	}
	traffics, _ := d.inboundService.GetAllClientTraffics()
	activeClients := 0
	depletedClients := 0
	approaching := 0

	exDiffDays, _ := d.settingService.GetExpireDiff()
	trDiffGB, _ := d.settingService.GetTrafficDiff()
	exDiffMs := int64(exDiffDays) * 86400000
	trDiffBytes := int64(trDiffGB) << 30
	nowMs := time.Now().UnixMilli()

	for _, t := range traffics {
		if t == nil {
			continue
		}
		used := t.Up + t.Down
		exhausted := t.Total > 0 && used >= t.Total
		expired := t.ExpiryTime > 0 && t.ExpiryTime <= nowMs
		if !t.Enable || exhausted || expired {
			depletedClients++
			continue
		}
		activeClients++
		nearExp := exDiffMs > 0 && t.ExpiryTime > 0 && (t.ExpiryTime-nowMs) <= exDiffMs
		nearTr := trDiffBytes > 0 && t.Total > 0 && (t.Total-used) < trDiffBytes
		if nearExp || nearTr {
			approaching++
		}
	}

	host, _ := os.Hostname()
	color := discordColorInfo
	if approaching > 0 {
		color = discordColorWarn
	}
	fields := []DiscordEmbedField{
		{Name: "Host", Value: nonEmpty(host, "-"), Inline: true},
		{Name: "Xray", Value: nonEmpty(d.xrayService.GetXrayVersion(), "-"), Inline: true},
		{Name: "Inbounds", Value: fmt.Sprintf("%d", len(inbounds)), Inline: true},
		{Name: "Active Clients", Value: fmt.Sprintf("%d", activeClients), Inline: true},
		{Name: "Depleted / Disabled", Value: fmt.Sprintf("%d", depletedClients), Inline: true},
		{Name: "Approaching Limit", Value: fmt.Sprintf("%d", approaching), Inline: true},
		{
			Name:   "Total Traffic",
			Value:  fmt.Sprintf("↑ %s · ↓ %s · Σ %s", common.FormatTraffic(totalUp), common.FormatTraffic(totalDown), common.FormatTraffic(totalUp+totalDown)),
			Inline: false,
		},
	}
	if err := d.SendEmbed(DiscordEmbed{
		Title:  "📊 OMEGA Periodic Report",
		Color:  color,
		Fields: fields,
	}); err != nil {
		logger.Warningf("Discord periodic report failed: %v", err)
	}
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
