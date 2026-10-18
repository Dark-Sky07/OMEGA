package job

import (
	"context"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_, _ = j.certService.RenewIfDue(ctx)
}
