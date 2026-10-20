package job

import (
	"github.com/Dark-Sky07/phoenix/internal/web/service"
	"github.com/Dark-Sky07/phoenix/internal/web/service/tgbot"
)

// LoginStatus represents the status of a login attempt.
type LoginStatus byte

const (
	LoginSuccess LoginStatus = 1 // Successful login
	LoginFail    LoginStatus = 0 // Failed login attempt
)

// StatsNotifyJob sends periodic statistics reports via Telegram and Discord.
type StatsNotifyJob struct {
	xrayService    service.XrayService
	tgbotService   tgbot.Tgbot
	discordService service.DiscordService
}

// NewStatsNotifyJob creates a new statistics notification job instance.
func NewStatsNotifyJob() *StatsNotifyJob {
	return new(StatsNotifyJob)
}

// Run sends a statistics report via Telegram / Discord if Xray is running.
func (j *StatsNotifyJob) Run() {
	if !j.xrayService.IsXrayRunning() {
		return
	}
	if j.tgbotService.IsRunning() {
		j.tgbotService.SendReport()
	}
	j.discordService.SendPeriodicReport()
}
