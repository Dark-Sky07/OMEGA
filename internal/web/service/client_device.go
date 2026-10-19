package service

import (
	"errors"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var (
	// ErrDeviceLimitReached is returned when a subscription request carries a
	// new HWID and the client's device limit is already reached.
	ErrDeviceLimitReached = errors.New("device limit reached")
)

// ClientDeviceService tracks per-client device (HWID) registrations.
//
// Enforcement model:
//   - Clients with limitDevice <= 0 never track or restrict devices.
//   - A request carrying a known HWID just refreshes last_seen (zero-count update).
//   - A request carrying a new HWID registers it only when free slots remain.
//   - When the limit is reached, ErrDeviceLimitReached is returned and the
//     subscription request must be denied as a whole (link-layer denials are
//     ambiguous and device-unfriendly).
type ClientDeviceService struct{}

// RegisterDevice records a device access for the given client email and HWID
// and enforces the client's device limit. limitDevice <= 0 means unlimited:
// no rows are written at all.
func (s *ClientDeviceService) RegisterDevice(email string, hwid string, limitDevice int, ip string, userAgent string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	hwid = strings.TrimSpace(hwid)
	if email == "" || hwid == "" {
		return nil
	}
	db := database.GetDB()
	if limitDevice <= 0 {
		// Unlimited — clients that previously had a limit keep their rows (so
		// toggling the limit off and on again preserves registrations), we just
		// never gate on them.
		return nil
	}
	now := time.Now().UnixMilli()
	var dev model.ClientDevice
	err := db.Where("email = ? AND hwid = ?", email, hwid).First(&dev).Error
	if err == nil {
		return db.Model(&model.ClientDevice{}).
			Where("email = ? AND hwid = ?", email, hwid).
			Updates(map[string]any{"last_seen_at": now, "ip": ip, "user_agent": userAgent}).Error
	}
	var count int64
	if err := db.Model(&model.ClientDevice{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return err
	}
	if int(count) >= limitDevice {
		return ErrDeviceLimitReached
	}
	dev = model.ClientDevice{
		Email:      email,
		Hwid:       hwid,
		IP:         ip,
		UserAgent:  userAgent,
		CreatedAt:  now,
		LastSeenAt: now,
	}
	return db.Create(&dev).Error
}

// GetDevices lists the registered devices for a client, newest first.
func (s *ClientDeviceService) GetDevices(email string) ([]model.ClientDevice, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var devices []model.ClientDevice
	err := database.GetDB().
		Where("email = ?", email).
		Order("last_seen_at DESC").
		Find(&devices).Error
	return devices, err
}

// RemoveDevice unregisters one HWID of a client. Returns false when nothing
// matched (already removed or wrong email/hwid pair).
func (s *ClientDeviceService) RemoveDevice(email string, hwid string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	hwid = strings.TrimSpace(hwid)
	if email == "" || hwid == "" {
		return false, nil
	}
	res := database.GetDB().
		Where("email = ? AND hwid = ?", email, hwid).
		Delete(&model.ClientDevice{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// RemoveAllDevices clears every registered device of a client (used when the
// client's HWID limit is lowered or the client is recreated).
func (s *ClientDeviceService) RemoveAllDevices(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	return database.GetDB().
		Where("email = ?", email).
		Delete(&model.ClientDevice{}).Error
}
