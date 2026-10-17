package sub

import (
	"strconv"
	"strings"
)

// SubOutlineService renders Outline-compatible subscriptions: plain-text,
// newline-separated SIP002 `ss://` access keys. Outline clients only speak
// Shadowsocks, so non-SS links of the same subscription are omitted rather
// than breaking the import.
type SubOutlineService struct {
	SubService *SubService
}

// NewSubOutlineService creates an Outline subscription service.
func NewSubOutlineService(subService *SubService) *SubOutlineService {
	return &SubOutlineService{SubService: subService}
}

// GetOutline returns the Outline access-key list and the traffic header.
// An empty body with nil error means the subscription has no Shadowsocks
// entries (controller turns that into 404, matching GetJson/GetClash).
func (s *SubOutlineService) GetOutline(subId string, host string) (string, string, error) {
	subs, _, _, traffic, err := s.SubService.GetSubs(subId, host)
	if err != nil {
		return "", "", err
	}
	var lines []string
	for _, sub := range subs {
		for _, line := range strings.Split(sub, "\n") {
			line = strings.TrimSpace(line)
			// SIP002 keys start with ss:// — ssconf:// and plugin variants the
			// Outline client cannot import are skipped deliberately.
			if strings.HasPrefix(line, "ss://") {
				lines = append(lines, line)
			}
		}
	}
	if len(lines) == 0 {
		return "", "", nil
	}
	header := "upload=" + strconv.FormatInt(traffic.Up, 10) +
		"; download=" + strconv.FormatInt(traffic.Down, 10) +
		"; total=" + strconv.FormatInt(traffic.Total, 10) +
		"; expire=" + strconv.FormatInt(traffic.ExpiryTime/1000, 10)
	return strings.Join(lines, "\n"), header, nil
}
