package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
	"gorm.io/gorm"
)

// ResellerService implements the نمایندگی (reseller) feature: panel sub-accounts
// that own a subset of inbounds/clients, bounded by inbounds/clients/traffic
// quotas and billed through a prepaid balance.
type ResellerService struct {
	inboundService InboundService
	clientService  ClientService
	settingService SettingService
}

const bytesPerGB = 1024 * 1024 * 1024

// ResellerStat is one reseller's live usage snapshot. It is what both the
// admin's reseller table and the reseller's own dashboard render.
type ResellerStat struct {
	Reseller         *model.Reseller `json:"reseller"`
	InboundCount     int             `json:"inboundCount"`
	ClientCount      int             `json:"clientCount"`
	OnlineCount      int             `json:"onlineCount"`
	UsedTraffic      int64           `json:"usedTraffic"`
	AllocatedTraffic int64           `json:"allocatedTraffic"`
	RemainingTraffic int64           `json:"remainingTraffic"` // TrafficLimit - AllocatedTraffic (0 when unlimited)
	Cost             float64         `json:"cost"`             // UsedTraffic priced at PricePerGB
	Balance          float64         `json:"balance"`          // Deposit - Cost (negative = debt)
	Expired          bool            `json:"expired"`
	OverQuota        bool            `json:"overQuota"` // Any quota exceeded
	Disabled         bool            `json:"disabled"`  // Auto-disabled because a quota ran out
}

// ResellerClientRow is a single client inside a reseller report.
type ResellerClientRow struct {
	Email      string  `json:"email"`
	Comment    string  `json:"comment,omitempty"`
	Group      string  `json:"group,omitempty"`
	Enable     bool    `json:"enable"`
	TotalBytes int64   `json:"totalBytes"`
	ExpiryTime int64   `json:"expiryTime"`
	Up         int64   `json:"up"`
	Down       int64   `json:"down"`
	Used       int64   `json:"used"`
	Cost       float64 `json:"cost"`
	InboundIds []int   `json:"inboundIds"`
	SubID      string  `json:"subId,omitempty"`
}

// ResellerReport is the full billing/usage report for one reseller.
type ResellerReport struct {
	Stat         *ResellerStat                `json:"stat"`
	Clients      []ResellerClientRow          `json:"clients"`
	Transactions []*model.ResellerTransaction `json:"transactions"`
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// List returns every reseller together with its live usage numbers.
func (s *ResellerService) List() ([]*ResellerStat, error) {
	db := database.GetDB()
	var resellers []*model.Reseller
	if err := db.Model(model.Reseller{}).Order("id ASC").Find(&resellers).Error; err != nil {
		return nil, err
	}
	out := make([]*ResellerStat, 0, len(resellers))
	for _, r := range resellers {
		stat, err := s.Stat(r)
		if err != nil {
			return nil, err
		}
		out = append(out, stat)
	}
	return out, nil
}

// Get loads one reseller by id.
func (s *ResellerService) Get(id int) (*model.Reseller, error) {
	db := database.GetDB()
	reseller := &model.Reseller{}
	err := db.Model(model.Reseller{}).Where("id = ?", id).First(reseller).Error
	if err != nil {
		return nil, err
	}
	return reseller, nil
}

// GetByUsername loads one reseller by its login name.
func (s *ResellerService) GetByUsername(username string) (*model.Reseller, error) {
	db := database.GetDB()
	reseller := &model.Reseller{}
	err := db.Model(model.Reseller{}).Where("username = ?", username).First(reseller).Error
	if err != nil {
		return nil, err
	}
	return reseller, nil
}

func (s *ResellerService) StatFor(id int) (*ResellerStat, error) {
	reseller, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return s.Stat(reseller)
}

// Add creates a reseller. The password is stored as a bcrypt hash.
// Billing fields (pricePerGb, deposit) are intentionally ignored and forced to 0
// because the panel now uses attach-inbound instead of billing.
func (s *ResellerService) Add(reseller *model.Reseller, password string) error {
	if reseller == nil {
		return errors.New("reseller is required")
	}
	reseller.Username = strings.TrimSpace(reseller.Username)
	if reseller.Username == "" {
		return errors.New("username can not be empty")
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("password can not be empty")
	}
	if err := s.validateUsername(reseller.Username, 0); err != nil {
		return err
	}
	hashed, err := crypto.HashPasswordAsBcrypt(password)
	if err != nil {
		return err
	}
	reseller.Password = hashed
	if reseller.Name == "" {
		reseller.Name = reseller.Username
	}
	// Force billing to 0 - removed from UI per admin request
	reseller.PricePerGB = 0
	reseller.Deposit = 0
	reseller.LoginEpoch = 1
	reseller.CreatedAt = time.Now().UnixMilli()
	reseller.UpdatedAt = reseller.CreatedAt
	db := database.GetDB()
	return db.Model(model.Reseller{}).Create(reseller).Error
}

// Update saves the editable fields of a reseller. An empty password leaves the
// stored one untouched.
func (s *ResellerService) Update(reseller *model.Reseller, password string) error {
	if reseller == nil || reseller.Id <= 0 {
		return errors.New("reseller is required")
	}
	existing, err := s.Get(reseller.Id)
	if err != nil {
		return err
	}
	reseller.Username = strings.TrimSpace(reseller.Username)
	if reseller.Username == "" {
		return errors.New("username can not be empty")
	}
	if err := s.validateUsername(reseller.Username, reseller.Id); err != nil {
		return err
	}
	if reseller.Name == "" {
		reseller.Name = reseller.Username
	}

	updates := map[string]any{
		"username":      reseller.Username,
		"name":          reseller.Name,
		"comment":       reseller.Comment,
		"enable":        reseller.Enable,
		"traffic_limit": reseller.TrafficLimit,
		"client_limit":  reseller.ClientLimit,
		"expiry_time":   reseller.ExpiryTime,
		"updated_at":    time.Now().UnixMilli(),
	}
	if strings.TrimSpace(password) != "" {
		hashed, err := crypto.HashPasswordAsBcrypt(password)
		if err != nil {
			return err
		}
		updates["password"] = hashed
		updates["login_epoch"] = existing.LoginEpoch + 1
	}
	db := database.GetDB()
	return db.Model(model.Reseller{}).Where("id = ?", reseller.Id).Updates(updates).Error
}

// Delete removes a reseller, its ownership mappings and its ledger. The
// inbounds/clients themselves are kept: they simply fall back to the admin.
func (s *ResellerService) Delete(id int) error {
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		for _, mdl := range []any{&model.Reseller{}, &model.ResellerInbound{}, &model.ResellerClient{}, &model.ResellerTransaction{}} {
			q := tx
			if _, isReseller := mdl.(*model.Reseller); isReseller {
				if err := q.Where("id = ?", id).Delete(mdl).Error; err != nil {
					return err
				}
				continue
			}
			if err := q.Where("reseller_id = ?", id).Delete(mdl).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SetEnable enables/disables a reseller. Disabling also cuts off every inbound
// the reseller owns and invalidates its active sessions.
func (s *ResellerService) SetEnable(id int, enable bool) (bool, error) {
	reseller, err := s.Get(id)
	if err != nil {
		return false, err
	}
	if reseller.Enable == enable {
		return false, nil
	}
	db := database.GetDB()
	updates := map[string]any{
		"enable":      enable,
		"updated_at":  time.Now().UnixMilli(),
		"login_epoch": reseller.LoginEpoch + 1,
	}
	if err := db.Model(model.Reseller{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return false, err
	}
	needRestart := false
	ids, err := s.OwnedInboundIds(id)
	if err != nil {
		return false, err
	}
	for _, inboundId := range ids {
		changed, err := s.inboundService.SetInboundEnable(inboundId, enable)
		if err != nil {
			logger.Warningf("reseller %d: failed to set inbound %d enable=%v: %v", id, inboundId, enable, err)
			continue
		}
		needRestart = needRestart || changed
	}
	return needRestart, nil
}

// ChangePassword updates a reseller's login password and invalidates its other
// sessions.
func (s *ResellerService) ChangePassword(id int, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("password can not be empty")
	}
	hashed, err := crypto.HashPasswordAsBcrypt(newPassword)
	if err != nil {
		return err
	}
	db := database.GetDB()
	return db.Model(model.Reseller{}).Where("id = ?", id).Updates(map[string]any{
		"password":    hashed,
		"login_epoch": gorm.Expr("login_epoch + 1"),
		"updated_at":  time.Now().UnixMilli(),
	}).Error
}

// CheckReseller authenticates a reseller login.
func (s *ResellerService) CheckReseller(username string, password string) (*model.Reseller, error) {
	reseller, err := s.GetByUsername(strings.TrimSpace(username))
	if err != nil {
		if database.IsNotFound(err) {
			return nil, errors.New("invalid credentials")
		}
		return nil, err
	}
	if !crypto.CheckPasswordHash(reseller.Password, password) {
		return nil, errors.New("invalid credentials")
	}
	if !reseller.Enable {
		return nil, errors.New("reseller account is disabled")
	}
	if IsExpired(reseller.ExpiryTime) {
		return nil, errors.New("reseller account has expired")
	}
	return reseller, nil
}

// IsExpired reports whether a unix-ms expiry timestamp is in the past. Zero
// means "never expires".
func IsExpired(expiryTime int64) bool {
	return expiryTime > 0 && expiryTime <= time.Now().UnixMilli()
}

func (s *ResellerService) validateUsername(username string, selfId int) error {
	db := database.GetDB()
	var count int64
	if err := db.Model(model.Reseller{}).
		Where("username = ? AND id <> ?", username, selfId).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("username %q is already used by another reseller", username)
	}
	// The admin account always wins at login, so a clashing reseller username
	// would be unreachable. Reject it up-front instead.
	if err := db.Model(model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("username %q is already used by the panel admin", username)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ownership
// ---------------------------------------------------------------------------

// OwnedInboundIds returns the ids of the inbounds assigned to a reseller.
func (s *ResellerService) OwnedInboundIds(resellerId int) ([]int, error) {
	db := database.GetDB()
	var ids []int
	err := db.Model(model.ResellerInbound{}).
		Where("reseller_id = ?", resellerId).
		Order("inbound_id ASC").
		Pluck("inbound_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// OwnedInboundIdSet is the set-shaped variant of OwnedInboundIds.
func (s *ResellerService) OwnedInboundIdSet(resellerId int) (map[int]struct{}, error) {
	ids, err := s.OwnedInboundIds(resellerId)
	if err != nil {
		return nil, err
	}
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

// OwnedEmailSet returns only the client emails explicitly mapped to a
// reseller. Owning an inbound grants access to that inbound's configuration,
// but never implicitly grants access to every client attached to it.
func (s *ResellerService) OwnedEmailSet(resellerId int) (map[string]struct{}, error) {
	emails, err := s.OwnedEmails(resellerId)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		key := strings.ToLower(strings.TrimSpace(email))
		if key != "" {
			set[key] = struct{}{}
		}
	}
	return set, nil
}

// OwnedAssociatedEmailSet returns the explicitly mapped clients that are also
// attached to at least one inbound owned by the same reseller. It is useful
// for association-aware identity/configuration paths. Aggregate traffic,
// online state, and last-online callers must use
// OwnedIsolatedAssociatedEmailSet because ClientTraffic is email-level and a
// shared admin association cannot be partitioned safely.
func (s *ResellerService) OwnedAssociatedEmailSet(resellerId int) (map[string]struct{}, error) {
	var emails []string
	db := database.GetDB()
	err := db.Table("reseller_clients AS rc").
		Select("DISTINCT LOWER(TRIM(c.email))").
		Joins("JOIN clients AS c ON LOWER(TRIM(c.email)) = LOWER(TRIM(rc.email))").
		Joins("JOIN client_inbounds AS ci ON ci.client_id = c.id").
		Joins("JOIN reseller_inbounds AS ri ON ri.inbound_id = ci.inbound_id AND ri.reseller_id = rc.reseller_id").
		Where("rc.reseller_id = ?", resellerId).
		Scan(&emails).Error
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		if key := transferEmailKey(email); key != "" {
			set[key] = struct{}{}
		}
	}
	return set, nil
}

// OwnedIsolatedAssociatedEmailSet returns mapped clients whose every current
// inbound association is owned by this reseller and that have at least one
// such association. ClientTraffic is one email-level aggregate, so a client
// shared with an admin-owned inbound cannot safely expose or mutate traffic,
// online state, or last-online data from a reseller session.
func (s *ResellerService) OwnedIsolatedAssociatedEmailSet(resellerId int) (map[string]struct{}, error) {
	var emails []string
	db := database.GetDB()
	err := db.Table("reseller_clients AS rc").
		Select("DISTINCT LOWER(TRIM(c.email))").
		Joins("JOIN clients AS c ON LOWER(TRIM(c.email)) = LOWER(TRIM(rc.email))").
		Joins("JOIN client_inbounds AS ci ON ci.client_id = c.id").
		Joins("JOIN reseller_inbounds AS ri ON ri.inbound_id = ci.inbound_id AND ri.reseller_id = rc.reseller_id").
		Where("rc.reseller_id = ?", resellerId).
		Where(`NOT EXISTS (
			SELECT 1 FROM client_inbounds AS foreign_ci
			WHERE foreign_ci.client_id = c.id
			  AND NOT EXISTS (
				SELECT 1 FROM reseller_inbounds AS foreign_ri
				WHERE foreign_ri.reseller_id = rc.reseller_id
				  AND foreign_ri.inbound_id = foreign_ci.inbound_id
			)
		)`).
		Scan(&emails).Error
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		if key := transferEmailKey(email); key != "" {
			set[key] = struct{}{}
		}
	}
	return set, nil
}

// OwnedEmails lists only the client emails explicitly mapped to a reseller.
// Keep this query independent from reseller_inbounds: inbound ownership is a
// separate capability and must not become a transitive client grant.
func (s *ResellerService) OwnedEmails(resellerId int) ([]string, error) {
	db := database.GetDB()
	var mapped []string
	if err := db.Model(model.ResellerClient{}).
		Where("reseller_id = ?", resellerId).
		Order("email ASC").
		Pluck("email", &mapped).Error; err != nil {
		return nil, err
	}
	if len(mapped) == 0 {
		return []string{}, nil
	}

	// Older databases may contain a mapping with incidental whitespace or
	// different casing. Resolve it to the canonical client email so every
	// reseller representation (lists, reports, transfer scopes) uses the same
	// identity, while retaining a trimmed fallback for a dangling mapping.
	keys := make([]string, 0, len(mapped))
	seenKeys := make(map[string]struct{}, len(mapped))
	for _, email := range mapped {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" {
			continue
		}
		if _, seen := seenKeys[key]; !seen {
			seenKeys[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	canonical := make(map[string]string, len(keys))
	for _, batch := range chunkStrings(keys, sqlInChunk) {
		var clients []model.ClientRecord
		if err := db.Model(model.ClientRecord{}).Where("LOWER(TRIM(email)) IN ?", batch).Find(&clients).Error; err != nil {
			return nil, err
		}
		for i := range clients {
			canonical[strings.ToLower(strings.TrimSpace(clients[i].Email))] = clients[i].Email
		}
	}

	out := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, email := range mapped {
		trimmed := strings.TrimSpace(email)
		key := strings.ToLower(trimmed)
		if key == "" {
			continue
		}
		value := trimmed
		if canonicalEmail, ok := canonical[key]; ok {
			value = canonicalEmail
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

// OwnsInbound reports whether the inbound is assigned to the reseller.
func (s *ResellerService) OwnsInbound(resellerId, inboundId int) (bool, error) {
	if resellerId <= 0 || inboundId <= 0 {
		return false, nil
	}
	db := database.GetDB()
	var count int64
	if err := db.Model(model.ResellerInbound{}).
		Where("reseller_id = ? AND inbound_id = ?", resellerId, inboundId).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// OwnsClient reports whether the client (by email) has an explicit mapping
// to the reseller. Inbound ownership is deliberately not considered here.
func (s *ResellerService) OwnsClient(resellerId int, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if resellerId <= 0 || email == "" {
		return false, nil
	}
	set, err := s.OwnedEmailSet(resellerId)
	if err != nil {
		return false, err
	}
	_, ok := set[email]
	return ok, nil
}

// EmailBySubID resolves a subscription id to its client email so reseller-
// scoped endpoints can verify ownership before serving subscription links.
func (s *ResellerService) EmailBySubID(subId string) (string, error) {
	subId = strings.TrimSpace(subId)
	if subId == "" {
		return "", nil
	}
	db := database.GetDB()
	var email string
	err := db.Model(model.ClientRecord{}).Where("sub_id = ?", subId).Limit(1).Pluck("email", &email).Error
	if err != nil {
		if database.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return email, nil
}

func normalizedPositiveIDs(values []int) ([]int, error) {
	out := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, errors.New("ids must be positive")
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, errors.New("at least one id is required")
	}
	return out, nil
}

// AssignInbounds atomically assigns a set of inbounds to one reseller. An
// inbound has one administrative owner: if any requested inbound is assigned
// to another reseller, the whole operation is rejected before a row is
// written. Repeating the same assignment is idempotent.
func (s *ResellerService) AssignInbounds(resellerId int, inboundIds []int) error {
	resellerScopeMutationMu.Lock()
	defer resellerScopeMutationMu.Unlock()
	ids, err := normalizedPositiveIDs(inboundIds)
	if err != nil {
		return err
	}
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var reseller model.Reseller
		if err := tx.Where("id = ?", resellerId).First(&reseller).Error; err != nil {
			if database.IsNotFound(err) {
				return errors.New("reseller not found")
			}
			return err
		}
		var inbounds []model.Inbound
		if err := tx.Where("id IN ?", ids).Find(&inbounds).Error; err != nil {
			return err
		}
		found := make(map[int]struct{}, len(inbounds))
		for _, inbound := range inbounds {
			found[inbound.Id] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := found[id]; !ok {
				return fmt.Errorf("inbound %d not found", id)
			}
		}
		var conflicts []model.ResellerInbound
		if err := tx.Where("inbound_id IN ? AND reseller_id <> ?", ids, resellerId).
			Find(&conflicts).Error; err != nil {
			return err
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("inbound %d is already assigned to another reseller", conflicts[0].InboundId)
		}
		now := time.Now().UnixMilli()
		for _, id := range ids {
			row := &model.ResellerInbound{ResellerId: resellerId, InboundId: id, CreatedAt: now}
			if err := tx.Where("reseller_id = ? AND inbound_id = ?", resellerId, id).
				FirstOrCreate(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SetInbounds replaces this reseller's complete inbound assignment set in one
// transaction. It is used by the multi-select form: an empty desired set is
// valid and clears only this reseller's mappings. Existing mappings are not
// touched until every requested inbound and ownership conflict has passed
// validation.
func (s *ResellerService) SetInbounds(resellerId int, inboundIds []int) error {
	resellerScopeMutationMu.Lock()
	defer resellerScopeMutationMu.Unlock()
	ids := make([]int, 0, len(inboundIds))
	seen := make(map[int]struct{}, len(inboundIds))
	for _, id := range inboundIds {
		if id <= 0 {
			return errors.New("ids must be positive")
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var reseller model.Reseller
		if err := tx.Where("id = ?", resellerId).First(&reseller).Error; err != nil {
			if database.IsNotFound(err) {
				return errors.New("reseller not found")
			}
			return err
		}
		if len(ids) > 0 {
			var inbounds []model.Inbound
			if err := tx.Where("id IN ?", ids).Find(&inbounds).Error; err != nil {
				return err
			}
			found := make(map[int]struct{}, len(inbounds))
			for _, inbound := range inbounds {
				found[inbound.Id] = struct{}{}
			}
			for _, id := range ids {
				if _, ok := found[id]; !ok {
					return fmt.Errorf("inbound %d not found", id)
				}
			}
			var conflicts []model.ResellerInbound
			if err := tx.Where("inbound_id IN ? AND reseller_id <> ?", ids, resellerId).
				Find(&conflicts).Error; err != nil {
				return err
			}
			if len(conflicts) > 0 {
				return fmt.Errorf("inbound %d is already assigned to another reseller", conflicts[0].InboundId)
			}
		}
		deleteQuery := tx.Where("reseller_id = ?", resellerId)
		if len(ids) > 0 {
			deleteQuery = deleteQuery.Where("inbound_id NOT IN ?", ids)
		}
		if err := deleteQuery.Delete(&model.ResellerInbound{}).Error; err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		for _, id := range ids {
			row := &model.ResellerInbound{ResellerId: resellerId, InboundId: id, CreatedAt: now}
			if err := tx.Where("reseller_id = ? AND inbound_id = ?", resellerId, id).
				FirstOrCreate(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AssignInbound is the compatibility wrapper for the original single-item
// endpoint.
func (s *ResellerService) AssignInbound(resellerId, inboundId int) error {
	return s.AssignInbounds(resellerId, []int{inboundId})
}

// UnassignInbounds atomically removes only this reseller's inbound mappings.
// It never changes client records or inbound configuration.
func (s *ResellerService) UnassignInbounds(resellerId int, inboundIds []int) error {
	resellerScopeMutationMu.Lock()
	defer resellerScopeMutationMu.Unlock()
	ids, err := normalizedPositiveIDs(inboundIds)
	if err != nil {
		return err
	}
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var reseller model.Reseller
		if err := tx.Where("id = ?", resellerId).First(&reseller).Error; err != nil {
			if database.IsNotFound(err) {
				return errors.New("reseller not found")
			}
			return err
		}
		return tx.Where("reseller_id = ? AND inbound_id IN ?", resellerId, ids).
			Delete(&model.ResellerInbound{}).Error
	})
}

// UnassignInbound is the compatibility wrapper for the original single-item
// endpoint.
func (s *ResellerService) UnassignInbound(resellerId, inboundId int) error {
	return s.UnassignInbounds(resellerId, []int{inboundId})
}

// ExplicitAssignedEmails lists only the clients the admin assigned to a
// reseller by hand (inherited ones are not included).
func (s *ResellerService) ExplicitAssignedEmails(resellerId int) ([]string, error) {
	return s.OwnedEmails(resellerId)
}

// HasForeignInboundAssociation reports whether a canonical client is attached
// to any inbound not owned by this reseller. This is used before a reseller
// attaches an existing client: the legacy email-level traffic/limit row cannot
// safely be shared across reseller and admin-owned associations.
func (s *ResellerService) HasForeignInboundAssociation(resellerId int, email string) (bool, error) {
	var count int64
	err := database.GetDB().Table("client_inbounds AS ci").
		Joins("JOIN clients AS c ON c.id = ci.client_id").
		Where("LOWER(TRIM(c.email)) = LOWER(?)", strings.TrimSpace(email)).
		Where(`NOT EXISTS (
			SELECT 1 FROM reseller_inbounds AS ri
			WHERE ri.reseller_id = ? AND ri.inbound_id = ci.inbound_id
		)`, resellerId).
		Count(&count).Error
	return count > 0, err
}

// AssignClients atomically creates explicit client mappings for a reseller.
// Inbound ownership is intentionally not consulted: this is the only operation
// that grants client credentials/stats visibility.
func (s *ResellerService) AssignClients(resellerId int, emails []string) error {
	resellerScopeMutationMu.Lock()
	defer resellerScopeMutationMu.Unlock()
	keys := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, raw := range emails {
		key := transferEmailKey(raw)
		if key == "" {
			return errors.New("email can not be empty")
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return errors.New("at least one email is required")
	}
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var reseller model.Reseller
		if err := tx.Where("id = ?", resellerId).First(&reseller).Error; err != nil {
			if database.IsNotFound(err) {
				return errors.New("reseller not found")
			}
			return err
		}
		var clients []model.ClientRecord
		if err := tx.Where("LOWER(TRIM(email)) IN ?", keys).Find(&clients).Error; err != nil {
			return err
		}
		canonical := make(map[string]string, len(clients))
		for _, client := range clients {
			canonical[transferEmailKey(client.Email)] = client.Email
		}
		for _, key := range keys {
			if _, ok := canonical[key]; !ok {
				return fmt.Errorf("client %q not found", key)
			}
		}
		for _, key := range keys {
			email := canonical[key]
			row := &model.ResellerClient{
				ResellerId: resellerId,
				Email:      email,
				CreatedAt:  time.Now().UnixMilli(),
			}
			if err := tx.Where("reseller_id = ? AND LOWER(TRIM(email)) = LOWER(?)", resellerId, email).
				FirstOrCreate(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AssignClient is the compatibility wrapper for the original single-item
// endpoint.
func (s *ResellerService) AssignClient(resellerId int, email string) error {
	return s.AssignClients(resellerId, []string{email})
}

// UnassignClients atomically removes explicit mappings for the requested
// reseller. It does not delete the client or touch any inbound association.
func (s *ResellerService) UnassignClients(resellerId int, emails []string) error {
	resellerScopeMutationMu.Lock()
	defer resellerScopeMutationMu.Unlock()
	keys := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, raw := range emails {
		key := transferEmailKey(raw)
		if key == "" {
			return errors.New("email can not be empty")
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return errors.New("at least one email is required")
	}
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var reseller model.Reseller
		if err := tx.Where("id = ?", resellerId).First(&reseller).Error; err != nil {
			if database.IsNotFound(err) {
				return errors.New("reseller not found")
			}
			return err
		}
		return tx.Where("reseller_id = ? AND LOWER(TRIM(email)) IN ?", resellerId, keys).
			Delete(&model.ResellerClient{}).Error
	})
}

// UnassignClient is the compatibility wrapper for the original single-item
// endpoint.
func (s *ResellerService) UnassignClient(resellerId int, email string) error {
	return s.UnassignClients(resellerId, []string{email})
}

// ---------------------------------------------------------------------------
// Usage & billing
// ---------------------------------------------------------------------------

// ClientUsage returns per-email traffic totals (up+down) for the given emails.
func (s *ResellerService) ClientUsage(emails []string) (map[string]*xray.ClientTraffic, error) {
	usage := make(map[string]*xray.ClientTraffic, len(emails))
	if len(emails) == 0 {
		return usage, nil
	}
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
	db := database.GetDB()
	rows := make([]*xray.ClientTraffic, 0)
	for _, batch := range chunkStrings(keys, sqlInChunk) {
		var part []*xray.ClientTraffic
		if err := db.Model(xray.ClientTraffic{}).Where("LOWER(TRIM(email)) IN ?", batch).Find(&part).Error; err != nil {
			return nil, err
		}
		rows = append(rows, part...)
	}
	overlayGlobalTraffic(db, rows)
	for _, row := range rows {
		key := strings.ToLower(strings.TrimSpace(row.Email))
		agg, ok := usage[key]
		if !ok {
			agg = &xray.ClientTraffic{Email: row.Email}
			usage[key] = agg
		}
		agg.Up += row.Up
		agg.Down += row.Down
		if row.Enable {
			agg.Enable = true
		}
	}
	return usage, nil
}

// Stat builds the live usage snapshot of one reseller.
func (s *ResellerService) Stat(reseller *model.Reseller) (*ResellerStat, error) {
	if reseller == nil {
		return nil, errors.New("reseller is required")
	}
	db := database.GetDB()
	stat := &ResellerStat{Reseller: reseller}

	ids, err := s.OwnedInboundIds(reseller.Id)
	if err != nil {
		return nil, err
	}
	stat.InboundCount = len(ids)

	emails, err := s.OwnedEmails(reseller.Id)
	if err != nil {
		return nil, err
	}
	stat.ClientCount = len(emails)
	associatedEmails, err := s.OwnedIsolatedAssociatedEmailSet(reseller.Id)
	if err != nil {
		return nil, err
	}
	usageEmails := make([]string, 0, len(emails))
	for _, email := range emails {
		if _, ok := associatedEmails[transferEmailKey(email)]; ok {
			usageEmails = append(usageEmails, email)
		}
	}

	if len(emails) > 0 {
		for _, batch := range chunkStrings(emails, sqlInChunk) {
			var allocated struct {
				Total int64
			}
			if err := db.Model(model.ClientRecord{}).
				Where("LOWER(TRIM(email)) IN ?", batch).
				Select("COALESCE(SUM(total_gb), 0) AS total").
				Scan(&allocated).Error; err != nil {
				return nil, err
			}
			stat.AllocatedTraffic += allocated.Total
		}
	}

	usage, err := s.ClientUsage(usageEmails)
	if err != nil {
		return nil, err
	}
	for _, row := range usage {
		stat.UsedTraffic += row.Up + row.Down
	}

	online := s.inboundService.GetOnlineClients()
	onlineSet := make(map[string]struct{}, len(online))
	for _, email := range online {
		onlineSet[transferEmailKey(email)] = struct{}{}
	}
	for email := range associatedEmails {
		if _, ok := onlineSet[email]; ok {
			stat.OnlineCount++
		}
	}

	// Billing removed: cost/balance always 0, kept for backward compat
	stat.Cost = 0
	stat.Balance = 0
	stat.Expired = IsExpired(reseller.ExpiryTime)
	if reseller.TrafficLimit > 0 {
		stat.RemainingTraffic = reseller.TrafficLimit - stat.AllocatedTraffic
	}
	stat.OverQuota = stat.Expired || !reseller.Enable ||
		(reseller.TrafficLimit > 0 && (stat.AllocatedTraffic > reseller.TrafficLimit || stat.UsedTraffic >= reseller.TrafficLimit)) ||
		(reseller.ClientLimit > 0 && stat.ClientCount > reseller.ClientLimit)
	stat.Disabled = !reseller.Enable
	return stat, nil
}

// Report returns the detailed usage/billing report of one reseller.
func (s *ResellerService) Report(resellerId int) (*ResellerReport, error) {
	reseller, err := s.Get(resellerId)
	if err != nil {
		return nil, err
	}
	stat, err := s.Stat(reseller)
	if err != nil {
		return nil, err
	}
	emails, err := s.OwnedEmails(resellerId)
	if err != nil {
		return nil, err
	}
	ownedInboundIDs, err := s.OwnedInboundIdSet(resellerId)
	if err != nil {
		return nil, err
	}
	associatedEmails, err := s.OwnedIsolatedAssociatedEmailSet(resellerId)
	if err != nil {
		return nil, err
	}
	usageEmails := make([]string, 0, len(emails))
	for _, email := range emails {
		if _, ok := associatedEmails[transferEmailKey(email)]; ok {
			usageEmails = append(usageEmails, email)
		}
	}
	usage, err := s.ClientUsage(usageEmails)
	if err != nil {
		return nil, err
	}

	db := database.GetDB()
	records := make([]*model.ClientRecord, 0, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var part []*model.ClientRecord
		if err := db.Model(model.ClientRecord{}).Where("LOWER(TRIM(email)) IN ?", batch).Find(&part).Error; err != nil {
			return nil, err
		}
		records = append(records, part...)
	}

	inboundIdsByEmail := map[string][]int{}
	if len(emails) > 0 {
		type row struct {
			Email     string
			InboundId int
		}
		rows := make([]row, 0)
		for _, batch := range chunkStrings(emails, sqlInChunk) {
			var part []row
			if err := db.Raw(`
				SELECT c.email AS email, ci.inbound_id AS inbound_id
				FROM clients c JOIN client_inbounds ci ON ci.client_id = c.id
				WHERE LOWER(TRIM(c.email)) IN ?
			`, batch).Scan(&part).Error; err != nil {
				return nil, err
			}
			rows = append(rows, part...)
		}
		for _, r := range rows {
			if _, allowed := ownedInboundIDs[r.InboundId]; !allowed {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(r.Email))
			inboundIdsByEmail[key] = append(inboundIdsByEmail[key], r.InboundId)
		}
	}

	rows := make([]ResellerClientRow, 0, len(records))
	for _, rec := range records {
		up, down, used := int64(0), int64(0), int64(0)
		if t, ok := usage[strings.ToLower(strings.TrimSpace(rec.Email))]; ok && t != nil {
			up, down = t.Up, t.Down
			used = up + down
		}
		ids := inboundIdsByEmail[strings.ToLower(strings.TrimSpace(rec.Email))]
		sort.Ints(ids)
		rows = append(rows, ResellerClientRow{
			Email:      rec.Email,
			Comment:    rec.Comment,
			Group:      rec.Group,
			Enable:     rec.Enable,
			TotalBytes: rec.TotalGB,
			ExpiryTime: rec.ExpiryTime,
			Up:         up,
			Down:       down,
			Used:       used,
			Cost:       0,
			InboundIds: ids,
			SubID:      rec.SubID,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Email < rows[j].Email })

	transactions, err := s.Transactions(resellerId)
	if err != nil {
		return nil, err
	}
	return &ResellerReport{Stat: stat, Clients: rows, Transactions: transactions}, nil
}

// Transactions lists a reseller's ledger, newest first.
func (s *ResellerService) Transactions(resellerId int) ([]*model.ResellerTransaction, error) {
	db := database.GetDB()
	rows := make([]*model.ResellerTransaction, 0)
	err := db.Model(model.ResellerTransaction{}).
		Where("reseller_id = ?", resellerId).
		Order("id DESC").
		Limit(500).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// AdjustBalance moves money on a reseller's account and records the movement.
// A negative amount withdraws (settlement), a positive amount tops the account
// up.
func (s *ResellerService) AdjustBalance(resellerId int, amount float64, comment string) (*model.Reseller, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount == 0 {
		return nil, errors.New("amount must be a non-zero number")
	}
	reseller, err := s.Get(resellerId)
	if err != nil {
		return nil, err
	}
	if reseller.Deposit+amount < 0 {
		return nil, fmt.Errorf("balance can not go below zero (current: %s)", FormatNumber(reseller.Deposit))
	}
	newBalance := Round2(reseller.Deposit + amount)
	txType := "deposit"
	if amount < 0 {
		txType = "withdraw"
	}
	db := database.GetDB()
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(model.Reseller{}).Where("id = ?", resellerId).Updates(map[string]any{
			"deposit":    newBalance,
			"updated_at": time.Now().UnixMilli(),
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.ResellerTransaction{
			ResellerId: resellerId,
			Type:       txType,
			Amount:     Round2(amount),
			Balance:    newBalance,
			Comment:    strings.TrimSpace(comment),
			CreatedAt:  time.Now().UnixMilli(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.Get(resellerId)
}

// ---------------------------------------------------------------------------
// Quota checks (used by the reseller-scoped write paths)
// ---------------------------------------------------------------------------

// EnsureActive rejects writes from disabled/expired resellers.
func (s *ResellerService) EnsureActive(reseller *model.Reseller) error {
	if reseller == nil {
		return errors.New("reseller is required")
	}
	if !reseller.Enable {
		return errors.New("your reseller account is disabled, please contact the administrator")
	}
	if IsExpired(reseller.ExpiryTime) {
		return errors.New("your reseller account has expired, please contact the administrator")
	}
	return nil
}

// CheckTrafficQuota verifies that allocating addBytes more client quota keeps
// the reseller inside its purchased volume (and that the volume is not already
// used up).
func (s *ResellerService) CheckTrafficQuota(reseller *model.Reseller, addBytes int64) error {
	if reseller.TrafficLimit <= 0 {
		return nil
	}
	stat, err := s.Stat(reseller)
	if err != nil {
		return err
	}
	if stat.UsedTraffic >= reseller.TrafficLimit {
		return fmt.Errorf("traffic quota exhausted: %s of %s used", FormatBytes(stat.UsedTraffic), FormatBytes(reseller.TrafficLimit))
	}
	if addBytes > 0 && stat.AllocatedTraffic+addBytes > reseller.TrafficLimit {
		return fmt.Errorf("traffic quota exceeded: only %s left out of %s (you are allocating %s)",
			FormatBytes(reseller.TrafficLimit-stat.AllocatedTraffic), FormatBytes(reseller.TrafficLimit), FormatBytes(addBytes))
	}
	return nil
}

// CheckClientQuota verifies that adding `adding` clients is allowed and that
// their quota fits into the traffic volume.
func (s *ResellerService) CheckClientQuota(reseller *model.Reseller, adding int, addBytes int64) error {
	if adding > 0 && reseller.ClientLimit > 0 {
		stat, err := s.Stat(reseller)
		if err != nil {
			return err
		}
		if stat.ClientCount+adding > reseller.ClientLimit {
			return fmt.Errorf("client limit reached: %d of %d used", stat.ClientCount, reseller.ClientLimit)
		}
	}
	return s.CheckTrafficQuota(reseller, addBytes)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// Round2 rounds a money amount to two decimals.
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// FormatBytes renders a byte count using binary units.
func FormatBytes(v int64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(v)
	idx := -1
	for value >= unit && idx < len(units)-1 {
		value /= unit
		idx++
	}
	return fmt.Sprintf("%.2f %s", value, units[idx])
}

// FormatNumber renders a money amount with two decimals.
func FormatNumber(v float64) string {
	return fmt.Sprintf("%.2f", v)
}
