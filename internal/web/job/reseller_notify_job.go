package job

import (
	"github.com/Dark-Sky07/phoenix/internal/logger"
	"github.com/Dark-Sky07/phoenix/internal/web/service"
)

// ResellerNotifyJob periodically evaluates every reseller's quotas and expiry
// and writes in-panel notifications for the reseller's own dashboard. Each
// warning kind fires once per depletion episode and re-arms after recovery.
type ResellerNotifyJob struct{}

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
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		notify.Evaluate(stat)
	}
	if err := notify.PurgeRead(30); err != nil {
		logger.Warning("reseller notify: purge read failed:", err)
	}
}
