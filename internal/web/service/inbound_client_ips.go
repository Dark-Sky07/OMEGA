package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *InboundService) GetAllInboundClientIps() ([]model.InboundClientIps, error) {
	db := database.GetDB()
	var ips []model.InboundClientIps
	err := db.Model(&model.InboundClientIps{}).Find(&ips).Error
	return ips, err
}

// GetInboundClientIpsForEmails returns IP history only for a caller-provided
// client set. IP rows are keyed by email and can exist without a current
// inbound; reseller callers must supply clients whose complete association set
// is inside their tenant because IP history is not partitioned by inbound.
func (s *InboundService) GetInboundClientIpsForEmails(emails []string) ([]model.InboundClientIps, error) {
	keys := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return []model.InboundClientIps{}, nil
	}
	var ips []model.InboundClientIps
	err := database.GetDB().Where("LOWER(TRIM(client_email)) IN ?", keys).Find(&ips).Error
	return ips, err
}

// clientIpStaleAfterSeconds mirrors job.ipStaleAfterSeconds: client IPs older than
// 30 minutes are evicted. Applying the same cutoff inside the cross-node merge keeps
// the synced blob bounded and stops the master's push-back from resurrecting IPs that
// a node has already pruned (otherwise the merge defeats the eviction cluster-wide).
const clientIpStaleAfterSeconds = int64(30 * 60)

// clientIpEntry is the on-disk shape of each element of InboundClientIps.Ips. Tags
// match job.IPWithTimestamp so the blob round-trips with the access.log scanner.
type clientIpEntry struct {
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
}

// mergeClientIpEntries unions old and incoming IP observations, dropping anything
// older than cutoff, keeping the most recent timestamp per IP, and returning the
// result sorted newest-first.
func mergeClientIpEntries(old, incoming []clientIpEntry, cutoff int64) []clientIpEntry {
	ipMap := make(map[string]int64, len(old)+len(incoming))
	for _, e := range old {
		if e.Timestamp < cutoff {
			continue
		}
		ipMap[e.IP] = e.Timestamp
	}
	for _, e := range incoming {
		if e.Timestamp < cutoff {
			continue
		}
		if cur, ok := ipMap[e.IP]; !ok || e.Timestamp > cur {
			ipMap[e.IP] = e.Timestamp
		}
	}
	out := make([]clientIpEntry, 0, len(ipMap))
	for ip, ts := range ipMap {
		out = append(out, clientIpEntry{IP: ip, Timestamp: ts})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out
}

// MergeInboundClientIps folds client IPs synced from another node into the local
// inbound_client_ips table without double-counting an IP seen on multiple nodes and
// without resurrecting stale entries. Existing rows are updated in place; brand-new
// clients (typically node-only clients with no local row) are created with a fresh
// local id.
func (s *InboundService) MergeInboundClientIps(incomingIps []model.InboundClientIps) error {
	db := database.GetDB()
	var currentIps []model.InboundClientIps
	if err := db.Model(&model.InboundClientIps{}).Find(&currentIps).Error; err != nil {
		return err
	}

	currentMap := make(map[string]*model.InboundClientIps, len(currentIps))
	for i := range currentIps {
		currentMap[strings.ToLower(strings.TrimSpace(currentIps[i].ClientEmail))] = &currentIps[i]
	}

	now := time.Now().Unix()
	cutoff := now - clientIpStaleAfterSeconds

	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, incoming := range incomingIps {
		incoming.ClientEmail = strings.TrimSpace(incoming.ClientEmail)
		if incoming.ClientEmail == "" || incoming.Ips == "" {
			continue
		}

		var incomingEntries []clientIpEntry
		_ = json.Unmarshal([]byte(incoming.Ips), &incomingEntries)

		current, exists := currentMap[strings.ToLower(incoming.ClientEmail)]
		if !exists {
			// New client we've never seen locally. Drop stale entries up front and
			// skip the row entirely if nothing is fresh, so we don't persist a row
			// that is dead on arrival.
			fresh := mergeClientIpEntries(nil, incomingEntries, cutoff)
			if len(fresh) == 0 {
				continue
			}
			b, _ := json.Marshal(fresh)
			incoming.Ips = string(b)
			// Never carry the remote node's primary key into the local table: id
			// spaces are independent across nodes and the remote id would collide
			// with an unrelated local row. OnConflict guards the race where
			// check_client_ip_job creates the same brand-new email between the
			// snapshot above and this insert.
			incoming.Id = 0
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "client_email"}},
				DoNothing: true,
			}).Create(&incoming).Error; err != nil {
				tx.Rollback()
				return err
			}
			continue
		}

		var oldEntries []clientIpEntry
		if current.Ips != "" {
			_ = json.Unmarshal([]byte(current.Ips), &oldEntries)
		}

		merged := mergeClientIpEntries(oldEntries, incomingEntries, cutoff)
		b, _ := json.Marshal(merged)
		mergedStr := string(b)

		// A concurrent check_client_ip_job db.Save on the same row can interleave
		// with this update (benign last-writer-wins; any dropped IP reappears on the
		// next scan/sync), so only write when the blob actually changed.
		if current.Ips != mergedStr {
			if err := tx.Model(&model.InboundClientIps{}).Where("id = ?", current.Id).Update("ips", mergedStr).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit().Error
}

func (s *InboundService) UpdateClientIPs(tx *gorm.DB, oldEmail string, newEmail string) error {
	return tx.Model(model.InboundClientIps{}).
		Where("LOWER(TRIM(client_email)) = LOWER(?)", strings.TrimSpace(oldEmail)).
		Update("client_email", strings.TrimSpace(newEmail)).Error
}

func (s *InboundService) DelClientIPs(tx *gorm.DB, email string) error {
	return tx.Where("LOWER(TRIM(client_email)) = LOWER(?)", strings.TrimSpace(email)).Delete(model.InboundClientIps{}).Error
}

func (s *InboundService) delClientIPsByEmails(tx *gorm.DB, emails []string) error {
	const chunk = 400
	keys := make([]string, 0, len(emails))
	for _, email := range emails {
		if key := strings.ToLower(strings.TrimSpace(email)); key != "" {
			keys = append(keys, key)
		}
	}
	for start := 0; start < len(keys); start += chunk {
		end := min(start+chunk, len(keys))
		if err := tx.Where("LOWER(TRIM(client_email)) IN ?", keys[start:end]).Delete(model.InboundClientIps{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *InboundService) getInboundClientIps(clientEmail string) (string, error) {
	db := database.GetDB()
	inboundClientIps := &model.InboundClientIps{}
	err := db.Model(model.InboundClientIps{}).Where("LOWER(TRIM(client_email)) = LOWER(?)", strings.TrimSpace(clientEmail)).First(inboundClientIps).Error
	if err != nil {
		return "", err
	}

	if inboundClientIps.Ips == "" {
		return "", nil
	}

	// Try to parse as new format (with timestamps)
	type IPWithTimestamp struct {
		IP        string `json:"ip"`
		Timestamp int64  `json:"timestamp"`
	}

	var ipsWithTime []IPWithTimestamp
	err = json.Unmarshal([]byte(inboundClientIps.Ips), &ipsWithTime)

	// If successfully parsed as new format, return with timestamps
	if err == nil && len(ipsWithTime) > 0 {
		return inboundClientIps.Ips, nil
	}

	// Otherwise, assume it's old format (simple string array)
	// Try to parse as simple array and convert to new format
	var oldIps []string
	err = json.Unmarshal([]byte(inboundClientIps.Ips), &oldIps)
	if err == nil && len(oldIps) > 0 {
		// Convert old format to new format with current timestamp
		newIpsWithTime := make([]IPWithTimestamp, len(oldIps))
		for i, ip := range oldIps {
			newIpsWithTime[i] = IPWithTimestamp{
				IP:        ip,
				Timestamp: time.Now().Unix(),
			}
		}
		result, _ := json.Marshal(newIpsWithTime)
		return string(result), nil
	}

	// Return as-is if parsing fails
	return inboundClientIps.Ips, nil
}

func (s *InboundService) GetInboundClientIps(clientEmail string) (string, error) {
	return s.getInboundClientIps(clientEmail)
}

// GetInboundClientIpsForInbounds returns email-keyed IP history only after
// proving that all current associations are in the caller's inbound scope.
// A shared client returns no history because the legacy row cannot be split by
// inbound without attributing another tenant's observations incorrectly.
func (s *InboundService) GetInboundClientIpsForInbounds(clientEmail string, allowedInboundIDs map[int]struct{}) (string, error) {
	ok, err := s.emailAssociationsWithinScope(clientEmail, allowedInboundIDs)
	if err != nil || !ok {
		return "", err
	}
	return s.getInboundClientIps(clientEmail)
}

func (s *InboundService) ClearClientIps(clientEmail string) error {
	db := database.GetDB()

	result := db.Model(model.InboundClientIps{}).
		Where("LOWER(TRIM(client_email)) = LOWER(?)", strings.TrimSpace(clientEmail)).
		Update("ips", "")
	err := result.Error
	if err != nil {
		return err
	}
	return nil
}

// ClearClientIpsForInbounds clears the email-keyed history only for clients
// whose complete current association set is inside the supplied scope. A
// client outside the scope is treated as a no-op so direct service callers do
// not get an existence oracle through this destructive operation.
func (s *InboundService) ClearClientIpsForInbounds(clientEmail string, allowedInboundIDs map[int]struct{}) error {
	ok, err := s.emailAssociationsWithinScope(clientEmail, allowedInboundIDs)
	if err != nil || !ok {
		return err
	}
	return s.ClearClientIps(clientEmail)
}
