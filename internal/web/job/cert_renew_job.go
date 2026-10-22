package job

import (
	"context"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/web/service"
)

// CertRenewJob checks the configured panel certificate once a day and renews
// it via Let's Encrypt when fewer than CertRenewBeforeDays remain.
type CertRenewJob struct {
	certService service.CertService
}

func NewCertRenewJob() *CertRenewJob {
	return &CertRenewJob{}
}

func (j *CertRenewJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	_, _ = j.certService.RenewIfDue(ctx)
	_ = j.certService.RenewResellerDomainsIfDue(ctx)
}
