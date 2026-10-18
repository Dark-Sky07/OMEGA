package service

import (
	"errors"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"gorm.io/gorm"
)

// UserTemplateService manages reusable client plan presets (User Templates).
type UserTemplateService struct{}

func normalizeUserTemplate(tpl *model.UserTemplate) error {
	if tpl == nil {
		return common.NewError("template is required")
	}
	tpl.Name = strings.TrimSpace(tpl.Name)
	if tpl.Name == "" {
		return common.NewError("template name is required")
	}
	if len(tpl.Name) > 128 {
		return common.NewError("template name is too long")
	}
	tpl.Description = strings.TrimSpace(tpl.Description)
	tpl.Group = strings.TrimSpace(tpl.Group)
	if tpl.TotalGB < 0 {
		tpl.TotalGB = 0
	}
	if tpl.ExpireDurationDays < 0 {
		tpl.ExpireDurationDays = 0
	}
	if tpl.ActivateBeforeDays < 0 {
		tpl.ActivateBeforeDays = 0
	}
	if !tpl.DelayedStart {
		tpl.ActivateBeforeDays = 0
	}
	if tpl.LimitIP < 0 {
		tpl.LimitIP = 0
	}
	if tpl.LimitDevice < 0 {
		tpl.LimitDevice = 0
	}
	if tpl.Reset < 0 {
		tpl.Reset = 0
	}
	if tpl.RenewalMode < 0 || tpl.RenewalMode > 2 {
		tpl.RenewalMode = 0
	}
	if tpl.RenewalMode == 0 {
		tpl.RenewalDay = 0
	}
	if tpl.InboundIds == nil {
		tpl.InboundIds = []int{}
	} else {
		seen := make(map[int]struct{}, len(tpl.InboundIds))
		dedup := make([]int, 0, len(tpl.InboundIds))
		for _, id := range tpl.InboundIds {
			if id <= 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			dedup = append(dedup, id)
		}
		tpl.InboundIds = dedup
	}
	return nil
}

func (s *UserTemplateService) List() ([]model.UserTemplate, error) {
	db := database.GetDB()
	var rows []model.UserTemplate
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].InboundIds == nil {
			rows[i].InboundIds = []int{}
		}
	}
	return rows, nil
}

func (s *UserTemplateService) Create(tpl *model.UserTemplate) (*model.UserTemplate, error) {
	if err := normalizeUserTemplate(tpl); err != nil {
		return nil, err
	}
	db := database.GetDB()
	var count int64
	if err := db.Model(&model.UserTemplate{}).
		Where("LOWER(name) = LOWER(?)", tpl.Name).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, common.NewError("template name already exists:", tpl.Name)
	}
	tpl.Id = 0
	now := time.Now().UnixMilli()
	tpl.CreatedAt = now
	tpl.UpdatedAt = now
	if err := db.Create(tpl).Error; err != nil {
		return nil, err
	}
	return tpl, nil
}

func (s *UserTemplateService) Update(id int, tpl *model.UserTemplate) (*model.UserTemplate, error) {
	if id <= 0 {
		return nil, common.NewError("invalid template id")
	}
	if err := normalizeUserTemplate(tpl); err != nil {
		return nil, err
	}
	db := database.GetDB()
	var existing model.UserTemplate
	if err := db.First(&existing, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewError("template not found")
		}
		return nil, err
	}
	var dupCount int64
	if err := db.Model(&model.UserTemplate{}).
		Where("LOWER(name) = LOWER(?) AND id != ?", tpl.Name, id).
		Count(&dupCount).Error; err != nil {
		return nil, err
	}
	if dupCount > 0 {
		return nil, common.NewError("template name already exists:", tpl.Name)
	}

	existing.Name = tpl.Name
	existing.Description = tpl.Description
	existing.TotalGB = tpl.TotalGB
	existing.ExpireDurationDays = tpl.ExpireDurationDays
	existing.DelayedStart = tpl.DelayedStart
	existing.ActivateBeforeDays = tpl.ActivateBeforeDays
	existing.LimitIP = tpl.LimitIP
	existing.LimitDevice = tpl.LimitDevice
	existing.Reset = tpl.Reset
	existing.RenewalMode = tpl.RenewalMode
	existing.RenewalDay = tpl.RenewalDay
	existing.Group = tpl.Group
	existing.InboundIds = tpl.InboundIds
	existing.UpdatedAt = time.Now().UnixMilli()

	if err := db.Save(&existing).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *UserTemplateService) Delete(id int) error {
	if id <= 0 {
		return common.NewError("invalid template id")
	}
	db := database.GetDB()
	res := db.Delete(&model.UserTemplate{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return common.NewError("template not found")
	}
	return nil
}
