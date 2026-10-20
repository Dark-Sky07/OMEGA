package job

import (
	"github.com/Dark-Sky07/phoenix/internal/web/service"
)

// ActivateBeforeJob expires delayed-start clients whose activation deadline
// passed without a first use. It is intentionally lightweight: when no client
// carries an activation deadline the sweep is a single indexed query.
type ActivateBeforeJob struct {
	clientService  service.ClientService
	inboundService service.InboundService
}

// NewActivateBeforeJob creates a new activation-deadline sweep job instance.
func NewActivateBeforeJob() *ActivateBeforeJob {
	return new(ActivateBeforeJob)
}

// Run sweeps expired activation deadlines.
func (j *ActivateBeforeJob) Run() {
	j.clientService.SweepActivateBefore(&j.inboundService)
}
