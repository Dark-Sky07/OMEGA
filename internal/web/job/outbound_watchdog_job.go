package job

import "github.com/Dark-Sky07/phoenix/internal/web/service"

// OutboundWatchdogJob periodically checks candidate Xray outbounds and fails over when needed.
type OutboundWatchdogJob struct {
	watchdogService service.OutboundWatchdogService
}

func NewOutboundWatchdogJob() *OutboundWatchdogJob {
	return &OutboundWatchdogJob{}
}

func (j *OutboundWatchdogJob) Run() {
	j.watchdogService.RunScheduledIfDue()
}
