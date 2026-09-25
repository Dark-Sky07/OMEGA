package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

func hasForbiddenClientChar(s string) bool {
	for _, r := range s {
		if r == '/' || r == '\\' || r == ' ' || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func validateClientEmail(email string) error {
	if hasForbiddenClientChar(email) {
		return common.NewError("client email contains an invalid character:", email)
	}
	return nil
}

func validateClientSubID(subID string) error {
	if hasForbiddenClientChar(subID) {
		return common.NewError("client subId contains an invalid character:", subID)
	}
	return nil
}

func (s *ClientService) Create(inboundSvc *InboundService, payload *ClientCreatePayload) (bool, error) {
	if payload == nil {
		return false, common.NewError("empty payload")
	}
	client := payload.Client
	client.Email = strings.TrimSpace(client.Email)
	payload.Client.Email = client.Email
	if client.Email == "" {
		return false, common.NewError("client email is required")
	}
	if err := validateClientEmail(client.Email); err != nil {
		return false, err
	}
	if err := validateClientSubID(client.SubID); err != nil {
		return false, err
	}
	if len(payload.InboundIds) == 0 {
		return false, common.NewError("at least one inbound is required")
	}

	if client.SubID == "" {
		client.SubID = uuid.NewString()
	}
	if !client.Enable {
		client.Enable = true
	}
	now := time.Now().UnixMilli()
	if client.CreatedAt == 0 {
		client.CreatedAt = now
	}
	client.UpdatedAt = now

	existing, err := s.GetRecordByEmail(nil, client.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !database.IsNotFound(err) {
		return false, err
	}
	emailTaken := err == nil
	if !emailTaken {
		existing = &model.ClientRecord{}
	}
	if emailTaken {
		if existing.SubID == "" || existing.SubID != client.SubID {
			return false, common.NewError("email already in use:", client.Email)
		}
	}

	if client.SubID != "" {
		var subTaken int64
		if err := database.GetDB().Model(&model.ClientRecord{}).
			Where("sub_id = ? AND LOWER(TRIM(email)) <> LOWER(TRIM(?))", client.SubID, client.Email).
			Count(&subTaken).Error; err != nil {
			return false, err
		}
		if subTaken > 0 {
			return false, common.NewError("subId already in use:", client.SubID)
		}
	}

	needRestart := false
	for _, ibId := range payload.InboundIds {
		inbound, getErr := inboundSvc.GetInbound(ibId)
		if getErr != nil {
			return needRestart, getErr
		}
		if err := s.fillProtocolDefaults(&client, inbound); err != nil {
			return needRestart, err
		}
		clientForInbound := client
		if ips, ok := client.AllowedIPsByInbound[ibId]; ok {
			clientForInbound.AllowedIPs = ips
		} else if !addressesFitAmneziaWGInbound(clientForInbound.AllowedIPs, inbound) {
			// The shared AllowedIPs value (e.g. from a single-field legacy
			// caller) came from a different subnet than this inbound's own --
			// clear it so defaultAmneziaWGClients allocates a fresh, correct
			// address for THIS inbound instead of persisting an unroutable
			// peer. Same reasoning as addressesFitAmneziaWGInbound's own doc
			// comment on the Attach path.
			clientForInbound.AllowedIPs = nil
		}
		settingsPayload, mErr := json.Marshal(map[string][]model.Client{"clients": {clientWithInboundFlow(clientForInbound, inbound)}})
		if mErr != nil {
			return needRestart, mErr
		}
		nr, addErr := s.AddInboundClient(inboundSvc, &model.Inbound{
			Id:       ibId,
			Settings: string(settingsPayload),
		})
		if addErr != nil {
			return needRestart, addErr
		}
		if nr {
			needRestart = true
		}
	}
	return needRestart, nil
}

func (s *ClientService) fillProtocolDefaults(c *model.Client, ib *model.Inbound) error {
	switch ib.Protocol {
	case model.VMESS, model.VLESS:
		if c.ID == "" {
			c.ID = uuid.NewString()
		}
	case model.Trojan:
		if c.Password == "" {
			c.Password = strings.ReplaceAll(uuid.NewString(), "-", "")
		}
	case model.Shadowsocks:
		method := shadowsocksMethodFromSettings(ib.Settings)
		if c.Password == "" || !validShadowsocksClientKey(method, c.Password) {
			c.Password = randomShadowsocksClientKey(method)
		}
	case model.Hysteria:
		if c.Auth == "" {
			c.Auth = strings.ReplaceAll(uuid.NewString(), "-", "")
		}
	}
	return nil
}

func clientWithInboundFlow(c model.Client, ib *model.Inbound) model.Client {
	if !inboundCanEnableTlsFlow(string(ib.Protocol), ib.StreamSettings, ib.Settings) {
		c.Flow = ""
	}
	return c
}

func shadowsocksMethodFromSettings(settings string) string {
	if settings == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(settings), &m); err != nil {
		return ""
	}
	method, _ := m["method"].(string)
	return method
}

func randomShadowsocksClientKey(method string) string {
	if n := shadowsocksKeyBytes(method); n > 0 {
		return random.Base64Bytes(n)
	}
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func validShadowsocksClientKey(method, key string) bool {
	n := shadowsocksKeyBytes(method)
	if n == 0 {
		return key != ""
	}
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return false
	}
	return len(decoded) == n
}

func shadowsocksKeyBytes(method string) int {
	switch method {
	case "2022-blake3-aes-128-gcm":
		return 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return 32
	}
	return 0
}

func applyShadowsocksClientMethod(clients []any, settings map[string]any) {
	method, _ := settings["method"].(string)
	is2022 := strings.HasPrefix(method, "2022-blake3-")
	for i := range clients {
		cm, ok := clients[i].(map[string]any)
		if !ok {
			continue
		}
		if is2022 {
			if _, hasKey := cm["method"]; hasKey {
				delete(cm, "method")
				clients[i] = cm
			}
			continue
		}
		if method == "" {
			continue
		}
		if existing, _ := cm["method"].(string); existing != "" {
			continue
		}
		cm["method"] = method
		clients[i] = cm
	}
}

func (s *ClientService) Update(inboundSvc *InboundService, id int, updated model.Client, inboundFilter ...int) (bool, error) {
	existing, err := s.GetByID(id)
	if err != nil {
		return false, err
	}
	inboundIds, err := s.GetInboundIdsForRecord(id)
	if err != nil {
		return false, err
	}
	if len(inboundFilter) > 0 {
		// Older callers pass a zero sentinel for the unscoped/admin update
		// path. Treat it as omitted; only positive ids form an actual filter.
		allow := make(map[int]struct{}, len(inboundFilter))
		for _, fid := range inboundFilter {
			if fid > 0 {
				allow[fid] = struct{}{}
			}
		}
		if len(allow) > 0 {
			filtered := inboundIds[:0:0]
			for _, ibId := range inboundIds {
				if _, ok := allow[ibId]; ok {
					filtered = append(filtered, ibId)
				}
			}
			inboundIds = filtered
		}
	}

	updated.Email = strings.TrimSpace(updated.Email)
	if updated.Email == "" {
		return false, common.NewError("client email is required")
	}
	if transferEmailKey(updated.Email) == transferEmailKey(existing.Email) {
		updated.Email = existing.Email
	}
	if err := validateClientEmail(updated.Email); err != nil {
		return false, err
	}
	if err := validateClientSubID(updated.SubID); err != nil {
		return false, err
	}
	if updated.SubID == "" {
		updated.SubID = existing.SubID
	}
	if updated.SubID == "" {
		updated.SubID = uuid.NewString()
	}
	updated.UpdatedAt = time.Now().UnixMilli()
	if updated.CreatedAt == 0 {
		updated.CreatedAt = existing.CreatedAt
	}

	// Preserve existing credentials when the caller omits them, so a partial
	// update (e.g. only changing traffic/expiry) doesn't silently rotate the
	// client's UUID/password/auth via fillProtocolDefaults. Supplying a new
	// value still rotates it intentionally.
	if updated.ID == "" {
		updated.ID = existing.UUID
	}
	if updated.Password == "" {
		updated.Password = existing.Password
	}
	if updated.Auth == "" {
		updated.Auth = existing.Auth
	}

	if updated.Email != existing.Email {
		var collisionCount int64
		if err := database.GetDB().Model(&model.ClientRecord{}).
			Where("LOWER(TRIM(email)) = LOWER(?) AND id <> ?", updated.Email, id).
			Count(&collisionCount).Error; err != nil {
			return false, err
		}
		if collisionCount > 0 {
			return false, common.NewError("Duplicate email:", updated.Email)
		}
		if err := database.GetDB().Model(&model.ClientRecord{}).
			Where("id = ?", id).
			Update("email", updated.Email).Error; err != nil {
			return false, err
		}
		// Reseller visibility follows the canonical email identity. Preserve
		// every explicit mapping when an administrator renames a client; do
		// not leave an orphaned ResellerClient row under the old email.
		if err := database.GetDB().Model(&model.ResellerClient{}).
			Where("LOWER(TRIM(email)) = LOWER(?)", existing.Email).
			Update("email", updated.Email).Error; err != nil {
			return false, err
		}
	}

	if updated.SubID != "" {
		var subCollision int64
		if err := database.GetDB().Model(&model.ClientRecord{}).
			Where("sub_id = ? AND id <> ?", updated.SubID, id).
			Count(&subCollision).Error; err != nil {
			return false, err
		}
		if subCollision > 0 {
			return false, common.NewError("Duplicate subId:", updated.SubID)
		}
	}

	needRestart := false
	for _, ibId := range inboundIds {
		inbound, getErr := inboundSvc.GetInbound(ibId)
		if getErr != nil {
			if errors.Is(getErr, gorm.ErrRecordNotFound) {
				if err := database.GetDB().
					Where("client_id = ? AND inbound_id = ?", id, ibId).
					Delete(&model.ClientInbound{}).Error; err != nil {
					return needRestart, err
				}
				continue
			}
			return needRestart, getErr
		}
		if existing.Email == "" {
			continue
		}
		if err := s.fillProtocolDefaults(&updated, inbound); err != nil {
			return needRestart, err
		}
		clientForInbound := updated
		if ips, ok := updated.AllowedIPsByInbound[ibId]; ok {
			clientForInbound.AllowedIPs = ips
		} else if !addressesFitAmneziaWGInbound(clientForInbound.AllowedIPs, inbound) {
			// A single shared AllowedIPs field (the common case for a caller
			// that never sends AllowedIPsByInbound) must never overwrite an
			// inbound it doesn't belong to -- e.g. a client attached to both
			// wg and awg saving its wg-labeled address would otherwise get
			// that same address silently written into the awg peer config
			// too. Clearing it here makes UpdateInboundClient's own
			// empty-AllowedIPs carry-forward (see its WireGuard/AmneziaWG
			// branch) preserve THIS inbound's existing, correct value
			// instead.
			clientForInbound.AllowedIPs = nil
		}
		settingsPayload, mErr := json.Marshal(map[string][]model.Client{"clients": {clientWithInboundFlow(clientForInbound, inbound)}})
		if mErr != nil {
			return needRestart, mErr
		}
		nr, upErr := s.UpdateInboundClient(inboundSvc, &model.Inbound{
			Id:       ibId,
			Settings: string(settingsPayload),
		}, existing.Email)
		if upErr != nil {
			return needRestart, upErr
		}
		if nr {
			needRestart = true
		}
		if ips, ok := updated.AllowedIPsByInbound[ibId]; ok &&
			(inbound.Protocol == model.WireGuard || inbound.Protocol == model.AmneziaWG) {
			if err := s.persistTunnelAllowedIPs(inboundSvc, ibId, existing.Email, ips); err != nil {
				return needRestart, err
			}
		}
	}

	reverseStr := ""
	if updated.Reverse != nil && strings.TrimSpace(updated.Reverse.Tag) != "" {
		if b, mErr := json.Marshal(updated.Reverse); mErr == nil {
			reverseStr = string(b)
		}
	}
	if err := database.GetDB().Model(&model.ClientRecord{}).
		Where("id = ?", id).
		Update("reverse", reverseStr).Error; err != nil {
		return needRestart, err
	}

	if err := database.GetDB().Model(&model.ClientRecord{}).
		Where("id = ?", id).
		UpdateColumn("updated_at", time.Now().UnixMilli()).Error; err != nil {
		return needRestart, err
	}
	return needRestart, nil
}

func (s *ClientService) Delete(inboundSvc *InboundService, id int, keepTraffic bool) (bool, error) {
	existing, err := s.GetByID(id)
	if err != nil {
		return false, err
	}
	tombstoneClientEmail(existing.Email)

	inboundIds, err := s.GetInboundIdsForRecord(id)
	if err != nil {
		return false, err
	}

	needRestart := false
	for _, ibId := range inboundIds {
		if _, getErr := inboundSvc.GetInbound(ibId); getErr != nil {
			if errors.Is(getErr, gorm.ErrRecordNotFound) {
				continue
			}
			return needRestart, getErr
		}

		// Always delete by email — the client's stable identity. This removes
		// every matching entry from the inbound's settings even when the stored
		// credential (UUID/password/auth) drifted from the inbound JSON, or a
		// duplicate entry with the same email exists.
		if existing.Email == "" {
			continue
		}
		nr, delErr := s.DelInboundClientByEmail(inboundSvc, ibId, existing.Email, false)
		if delErr != nil {
			// The client is already absent from this inbound (data drift or a
			// retried delete). Skip it — deletion stays idempotent.
			if errors.Is(delErr, ErrClientNotInInbound) {
				continue
			}
			return needRestart, delErr
		}
		if nr {
			needRestart = true
		}
	}

	db := database.GetDB()
	if err := db.Where("client_id = ?", id).Delete(&model.ClientInbound{}).Error; err != nil {
		return needRestart, err
	}
	if !keepTraffic && existing.Email != "" {
		if err := db.Where("LOWER(TRIM(email)) = LOWER(?)", existing.Email).Delete(&xray.ClientTraffic{}).Error; err != nil {
			return needRestart, err
		}
		if err := clearGlobalTraffic(db, existing.Email); err != nil {
			return needRestart, err
		}
		if err := db.Where("LOWER(TRIM(client_email)) = LOWER(?)", existing.Email).Delete(&model.InboundClientIps{}).Error; err != nil {
			return needRestart, err
		}
	}
	if err := db.Delete(&model.ClientRecord{}, id).Error; err != nil {
		return needRestart, err
	}
	if err := db.Where("LOWER(TRIM(email)) = LOWER(?)", existing.Email).Delete(&model.ResellerClient{}).Error; err != nil {
		return needRestart, err
	}
	return needRestart, nil
}

// hasTunnelAttachment reports whether any of inboundIds is a currently
// existing WireGuard or AmneziaWG inbound. Inbounds that fail to load are
// skipped rather than treated as an error -- Attach's own loop already
// surfaces a real error for any inbound it can't load when it gets there.
func (s *ClientService) hasTunnelAttachment(inboundSvc *InboundService, inboundIds []int) bool {
	for _, ibId := range inboundIds {
		inbound, err := inboundSvc.GetInbound(ibId)
		if err != nil {
			continue
		}
		if inbound.Protocol == model.WireGuard || inbound.Protocol == model.AmneziaWG {
			return true
		}
	}
	return false
}

// addressesFitAmneziaWGInbound reports whether every entry in addrs falls
// inside ib's own configured subnet(s). AmneziaWG only: its kernel interface
// Address is exactly that subnet, so an address inherited from elsewhere (an
// identity attached to a WireGuard inbound first, say) produces a peer that
// can never connect -- Attach allocates fresh instead.
func addressesFitAmneziaWGInbound(addrs []string, ib *model.Inbound) bool {
	if ib.Protocol != model.AmneziaWG || len(addrs) == 0 {
		return true
	}
	v4Base, v6Base, err := defaultAmneziaWGSubnetBases(ib.Settings)
	if err != nil {
		return false
	}
	bases := make([]netip.Prefix, 0, 2)
	for _, base := range []string{v4Base, v6Base} {
		if base == "" {
			continue
		}
		prefix, pErr := netip.ParsePrefix(base)
		if pErr != nil {
			return false
		}
		bases = append(bases, prefix)
	}
	for _, a := range addrs {
		host := wireguardHostAddr(a)
		if !host.IsValid() {
			return false
		}
		fits := false
		for _, prefix := range bases {
			if prefix.Contains(host) {
				fits = true
				break
			}
		}
		if !fits {
			return false
		}
	}
	return true
}

func (s *ClientService) Attach(inboundSvc *InboundService, id int, inboundIds []int) (bool, error) {
	existing, err := s.GetByID(id)
	if err != nil {
		return false, err
	}
	currentIds, err := s.GetInboundIdsForRecord(id)
	if err != nil {
		return false, err
	}
	have := make(map[int]struct{}, len(currentIds))
	for _, x := range currentIds {
		have[x] = struct{}{}
	}

	clientWire := existing.ToClient()
	flow, ffErr := s.EffectiveFlow(nil, id)
	if ffErr != nil {
		return false, ffErr
	}
	clientWire.Flow = flow
	clientWire.UpdatedAt = time.Now().UnixMilli()

	// If this identity has no CURRENT WireGuard/AmneziaWG attachment,
	// clientWire.AllowedIPs (from the ClientRecord) is a leftover from
	// whenever it last had one -- nothing reserves it anymore. Clear it so
	// attaching to a tunnel inbound now allocates a fresh address instead
	// of resurrecting the old one, which may no longer even be the lowest
	// free slot. Left untouched when the identity already has an active
	// tunnel elsewhere, so extending it to a second protocol still keeps
	// the same address on both.
	if !s.hasTunnelAttachment(inboundSvc, currentIds) {
		clientWire.AllowedIPs = nil
	}

	needRestart := false
	for _, ibId := range inboundIds {
		if _, attached := have[ibId]; attached {
			continue
		}
		inbound, getErr := inboundSvc.GetInbound(ibId)
		if getErr != nil {
			return needRestart, getErr
		}
		copyClient := *clientWire
		if !addressesFitAmneziaWGInbound(copyClient.AllowedIPs, inbound) {
			copyClient.AllowedIPs = nil
		}
		if err := s.fillProtocolDefaults(&copyClient, inbound); err != nil {
			return needRestart, err
		}
		settingsPayload, mErr := json.Marshal(map[string][]model.Client{"clients": {clientWithInboundFlow(copyClient, inbound)}})
		if mErr != nil {
			return needRestart, mErr
		}
		nr, addErr := s.AddInboundClient(inboundSvc, &model.Inbound{
			Id:       ibId,
			Settings: string(settingsPayload),
		})
		if addErr != nil {
			return needRestart, addErr
		}
		if nr {
			needRestart = true
		}
	}
	return needRestart, nil
}

func (s *ClientService) CreateOne(inboundSvc *InboundService, inboundId int, client model.Client) (bool, error) {
	return s.Create(inboundSvc, &ClientCreatePayload{
		Client:     client,
		InboundIds: []int{inboundId},
	})
}

func (s *ClientService) DetachByEmail(inboundSvc *InboundService, inboundId int, email string) (bool, error) {
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	return s.Detach(inboundSvc, rec.Id, []int{inboundId})
}

func (s *ClientService) AttachByEmail(inboundSvc *InboundService, email string, inboundIds []int) (bool, error) {
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	return s.Attach(inboundSvc, rec.Id, inboundIds)
}

func (s *ClientService) DetachByEmailMany(inboundSvc *InboundService, email string, inboundIds []int) (bool, error) {
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	return s.Detach(inboundSvc, rec.Id, inboundIds)
}

func (s *ClientService) DeleteByEmail(inboundSvc *InboundService, email string, keepTraffic bool) (bool, error) {
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err == nil {
		return s.Delete(inboundSvc, rec.Id, keepTraffic)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	inboundIds, idsErr := s.findInboundIdsByClientEmail(email)
	if idsErr != nil {
		return false, idsErr
	}
	if len(inboundIds) == 0 {
		return false, common.NewError(fmt.Sprintf("client %q not found in any inbound or client record", email))
	}
	needRestart := false
	for _, ibId := range inboundIds {
		nr, delErr := s.DelInboundClientByEmail(inboundSvc, ibId, email, false)
		if delErr != nil {
			if errors.Is(delErr, ErrClientNotInInbound) {
				continue
			}
			return needRestart, delErr
		}
		if nr {
			needRestart = true
		}
	}
	db := database.GetDB()
	if !keepTraffic {
		if err := db.Where("LOWER(TRIM(email)) = LOWER(?)", email).Delete(&xray.ClientTraffic{}).Error; err != nil {
			return needRestart, err
		}
		if err := clearGlobalTraffic(db, email); err != nil {
			return needRestart, err
		}
		if err := db.Where("LOWER(TRIM(client_email)) = LOWER(?)", email).Delete(&model.InboundClientIps{}).Error; err != nil {
			return needRestart, err
		}
	}
	if err := db.Where("LOWER(TRIM(email)) = LOWER(?)", email).Delete(&model.ResellerClient{}).Error; err != nil {
		return needRestart, err
	}
	return needRestart, nil
}

// DeleteByEmailForInbounds removes a client's attachments only from the
// supplied inbounds. It preserves the canonical client, credentials, stats,
// IP history, and unrelated inbound associations until no association remains.
// This is the destructive-operation boundary used by reseller routes.
func (s *ClientService) DeleteByEmailForInbounds(inboundSvc *InboundService, email string, keepTraffic bool, inboundIDs []int) (bool, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.NewError(fmt.Sprintf("client %q not found in any inbound or client record", email))
		}
		return false, err
	}
	if len(inboundIDs) == 0 {
		return false, nil
	}

	allowed := make(map[int]struct{}, len(inboundIDs))
	for _, id := range inboundIDs {
		if id > 0 {
			allowed[id] = struct{}{}
		}
	}
	currentIDs, err := s.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return false, err
	}
	needRestart := false
	removed := false
	for _, inboundID := range currentIDs {
		if _, ok := allowed[inboundID]; !ok {
			continue
		}
		if _, getErr := inboundSvc.GetInbound(inboundID); getErr != nil {
			if errors.Is(getErr, gorm.ErrRecordNotFound) {
				if err := database.GetDB().Where("client_id = ? AND inbound_id = ?", rec.Id, inboundID).Delete(&model.ClientInbound{}).Error; err != nil {
					return needRestart, err
				}
				removed = true
				continue
			}
			return needRestart, getErr
		}
		// Keep shared traffic while the client still belongs to any other
		// inbound. The cleanup below handles the final-association case.
		nr, delErr := s.DelInboundClientByEmail(inboundSvc, inboundID, rec.Email, true)
		if delErr != nil {
			if errors.Is(delErr, ErrClientNotInInbound) {
				continue
			}
			return needRestart, delErr
		}
		removed = true
		needRestart = needRestart || nr
		// SyncInbound normally removes this edge. Make the scoped contract
		// explicit even when the inbound was legacy/partially normalized.
		if err := database.GetDB().Where("client_id = ? AND inbound_id = ?", rec.Id, inboundID).Delete(&model.ClientInbound{}).Error; err != nil {
			return needRestart, err
		}
	}
	if !removed {
		return needRestart, nil
	}

	remainingIDs, err := s.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return needRestart, err
	}
	if len(remainingIDs) > 0 {
		// An admin-owned or another reseller-owned association remains. Never
		// delete its shared stats or canonical credentials.
		return needRestart, nil
	}

	db := database.GetDB()
	if !keepTraffic {
		if err := db.Where("LOWER(TRIM(email)) = LOWER(?)", rec.Email).Delete(&xray.ClientTraffic{}).Error; err != nil {
			return needRestart, err
		}
		if err := clearGlobalTraffic(db, rec.Email); err != nil {
			return needRestart, err
		}
		if err := db.Where("LOWER(TRIM(client_email)) = LOWER(?)", rec.Email).Delete(&model.InboundClientIps{}).Error; err != nil {
			return needRestart, err
		}
	}
	if err := db.Delete(&model.ClientRecord{}, rec.Id).Error; err != nil {
		return needRestart, err
	}
	if err := db.Where("LOWER(TRIM(email)) = LOWER(?)", rec.Email).Delete(&model.ResellerClient{}).Error; err != nil {
		return needRestart, err
	}
	return needRestart, nil
}

func (s *ClientService) UpdateByEmail(inboundSvc *InboundService, email string, updated model.Client, inboundFilter ...int) (bool, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return false, common.NewError("client email is required")
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	return s.Update(inboundSvc, rec.Id, updated, inboundFilter...)
}

// UpdateByEmailForInbounds limits the settings/runtime mutation to the supplied
// inbound IDs. An empty list means "no inbounds", unlike UpdateByEmail's
// omitted optional filter, which retains the admin-wide behavior for legacy
// callers. This distinction is required by reseller routes: a reseller with an
// explicitly mapped client must not update that client's attachment on an
// unrelated admin-owned inbound merely because the email is shared.
func (s *ClientService) UpdateByEmailForInbounds(inboundSvc *InboundService, email string, updated model.Client, inboundIDs []int) (bool, error) {
	if len(inboundIDs) == 0 {
		// A scoped reseller update with no owned association must not even
		// touch canonical fields such as reverse or updated_at.
		return false, nil
	}
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	currentIDs, err := s.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return false, err
	}
	if !allInboundIDsInScope(currentIDs, inboundIDs) {
		// Canonical credentials, limits, and protocol fields are shared by
		// email. A scoped update cannot safely mutate them for a client that
		// is also attached outside the requested tenant.
		return false, nil
	}
	filter := append([]int(nil), inboundIDs...)
	return s.UpdateByEmail(inboundSvc, email, updated, filter...)
}

func (s *ClientService) Detach(inboundSvc *InboundService, id int, inboundIds []int) (bool, error) {
	existing, err := s.GetByID(id)
	if err != nil {
		return false, err
	}
	currentIds, err := s.GetInboundIdsForRecord(id)
	if err != nil {
		return false, err
	}
	have := make(map[int]struct{}, len(currentIds))
	for _, x := range currentIds {
		have[x] = struct{}{}
	}

	needRestart := false
	for _, ibId := range inboundIds {
		if _, attached := have[ibId]; !attached {
			continue
		}
		if _, getErr := inboundSvc.GetInbound(ibId); getErr != nil {
			return needRestart, getErr
		}
		// Detach by email — the client's stable identity (see Delete).
		if existing.Email == "" {
			continue
		}
		nr, delErr := s.DelInboundClientByEmail(inboundSvc, ibId, existing.Email, true)
		if delErr != nil {
			if errors.Is(delErr, ErrClientNotInInbound) {
				continue
			}
			return needRestart, delErr
		}
		if nr {
			needRestart = true
		}
	}
	return needRestart, nil
}
