package job

import (
	"github.com/Dark-Sky07/phoenix/internal/logger"
	"github.com/Dark-Sky07/phoenix/internal/web/service"
	"github.com/Dark-Sky07/phoenix/internal/web/service/tgbot"
)

// ResellerNotifyJob periodically evaluates every reseller's quotas and expiry
// and writes in-panel notifications for the reseller's own dashboard. Each
// warning kind fires once per depletion episode and re-arms after recovery.
// When the Telegram bot is running and the reseller linked their account,
// fresh notifications are also pushed to their Telegram chat.
type ResellerNotifyJob struct {
	tgbotService tgbot.Tgbot
}

// NewResellerNotifyJob creates a new reseller notification job instance.
func NewResellerNotifyJob() *ResellerNotifyJob {
	return new(ResellerNotifyJob)
}

// Run evaluates every reseller and prunes old read notifications. Errors are
// logged and never panic the cron loop.
func (j *ResellerNotifyJob) Run() {
	resellerService := &service.ResellerService{}
	stats, err := resellerService.List()
	if err != nil {
		logger.Warning("reseller notify: list resellers failed:", err)
		return
	}
	notify := &service.ResellerNotifyService{}
	pushEnabled := j.tgbotService.IsRunning()
	for _, stat := range stats {
		if stat == nil || stat.Reseller == nil {
			continue
		}
		fresh := notify.Evaluate(stat)
		if !pushEnabled || stat.Reseller.TgID == 0 || !stat.Reseller.Enable {
			continue
		}
		for _, row := range fresh {
			if msg := j.tgbotService.FormatResellerNotification(stat.Reseller, row); msg != "" {
				j.tgbotService.SendMsgToTgbot(stat.Reseller.TgID, msg)
			}
		}
	}
	if err := notify.PurgeRead(30); err != nil {
		logger.Warning("reseller notify: purge read failed:", err)
	}
}
