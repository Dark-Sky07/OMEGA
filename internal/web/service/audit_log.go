package service

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

const maxAuditLogRows = 5000

var auditWriteCount uint64

// AuditLogService manages security audit log entries for admin, reseller, and system actions.
type AuditLogService struct{}

// Record persists an AuditLog entry and periodically trims rows beyond maxAuditLogRows.
func (s *AuditLogService) Record(entry *model.AuditLog) {
	if entry == nil {
		return
	}
	db := database.GetDB()
	if db == nil {
		return
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().UnixMilli()
	}
	if len(entry.Target) > 500 {
		entry.Target = entry.Target[:500]
	}
	if err := db.Create(entry).Error; err != nil {
		return
	}
	if atomic.AddUint64(&auditWriteCount, 1)%100 == 0 {
		s.pruneOldEntries()
	}
}

func (s *AuditLogService) pruneOldEntries() {
	db := database.GetDB()
	if db == nil {
		return
	}
	var cutoff model.AuditLog
	if err := db.Order("id DESC").Offset(maxAuditLogRows).Limit(1).Find(&cutoff).Error; err == nil && cutoff.Id > 0 {
		_ = db.Where("id <= ?", cutoff.Id).Delete(&model.AuditLog{}).Error
	}
}

// List retrieves recent audit log entries matching optional filters.
func (s *AuditLogService) List(limit int, actorType, module, search string) ([]model.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	db := database.GetDB()
	q := db.Model(&model.AuditLog{})
	if at := strings.TrimSpace(actorType); at != "" {
		q = q.Where("actor_type = ?", at)
	}
	if mod := strings.TrimSpace(module); mod != "" {
		q = q.Where("module = ?", mod)
	}
	if kw := strings.TrimSpace(search); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("actor_name LIKE ? OR action LIKE ? OR target LIKE ? OR ip LIKE ?", like, like, like, like)
	}
	var list []model.AuditLog
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

// Clear removes all audit log entries.
func (s *AuditLogService) Clear() error {
	db := database.GetDB()
	return db.Where("1 = 1").Delete(&model.AuditLog{}).Error
}
