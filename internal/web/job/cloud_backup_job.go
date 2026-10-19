package job

import "github.com/mhsanaei/3x-ui/v3/internal/web/service"

// CloudBackupJob periodically checks if a scheduled cloud database backup is due.
type CloudBackupJob struct {
	cloudBackupService service.CloudBackupService
}

func NewCloudBackupJob() *CloudBackupJob {
	return &CloudBackupJob{}
}

func (j *CloudBackupJob) Run() {
	j.cloudBackupService.RunScheduledIfDue()
}
