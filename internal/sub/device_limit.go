package sub

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// EnforceDeviceLimit registers the request's device (HWID) against every
// client matched by the subscription and enforces each client's device limit.
//
// Rules (mirroring ClientDeviceService.RegisterDevice):
//   - No HWID in the request → nothing is registered, request passes. Devices
//     without HWID support keep working exactly as before.
//   - Clients with limitDevice <= 0 are unlimited and never tracked.
//   - Known HWIDs only refresh last-seen.
//   - A new HWID on a client whose slots are full fails the whole request with
//     service.ErrDeviceLimitReached.
//
// A subscription may aggregate several clients; every client with a limit
// must accept the device for the request to pass.
func (s *SubService) EnforceDeviceLimit(subId string, hwid string, ip string, userAgent string) error {
	hwid = strings.TrimSpace(hwid)
	if hwid == "" {
		return nil
	}
	inbounds, err := s.getInboundsBySubId(subId)
	if err != nil || len(inbounds) == 0 {
		return nil
	}
	var deviceService service.ClientDeviceService
	seen := make(map[string]struct{})
	for _, inbound := range inbounds {
		for _, client := range s.matchingClients(inbound, subId) {
			key := strings.ToLower(strings.TrimSpace(client.Email))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if err := deviceService.RegisterDevice(client.Email, hwid, client.LimitDevice, ip, userAgent); err != nil {
				return err
			}
		}
	}
	return nil
}
