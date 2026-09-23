package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
)

const (
	clientTransferFormat  = "omega-client-transfer"
	clientTransferVersion = 1
	maxTransferClients    = 5000
)

// ClientTransferEnvelope is the stable, versioned client backup format. The
// database primary key is deliberately absent: email is the client identity
// when an envelope is imported into another panel.
type ClientTransferEnvelope struct {
	Format              string                 `json:"format"`
	Version             int                    `json:"version"`
	ExportedAt          string                 `json:"exportedAt"`
	ReplaceAttachments  bool                   `json:"replaceAttachments,omitempty"`
	Clients             []ClientTransferEntry `json:"clients"`
}

// ClientTransferInboundRef is the portable identity for one client
// attachment. IDs are retained for same-panel/backward compatibility, while
// tag/protocol/remark/node identity allow imports to resolve a different
// panel's primary keys. FlowOverride is a pointer so an explicit empty flow is
// distinguishable from an omitted override.
type ClientTransferInboundRef struct {
	ID              int                         `json:"id,omitempty"`
	Tag             string                      `json:"tag,omitempty"`
	Remark          string                      `json:"remark,omitempty"`
	Protocol        model.Protocol              `json:"protocol,omitempty"`
	Port            int                         `json:"port,omitempty"`
	Listen          string                      `json:"listen,omitempty"`
	OriginNodeGuid  string                      `json:"originNodeGuid,omitempty"`
	FlowOverride    *string                     `json:"flowOverride,omitempty"`
	FallbackParent  *ClientTransferFallbackRef  `json:"fallbackParent,omitempty"`
}

type ClientTransferFallbackRef struct {
	MasterID        int            `json:"masterId,omitempty"`
	MasterTag       string         `json:"masterTag,omitempty"`
	MasterRemark    string         `json:"masterRemark,omitempty"`
	MasterProtocol  model.Protocol `json:"masterProtocol,omitempty"`
	MasterPort      int            `json:"masterPort,omitempty"`
	MasterListen    string         `json:"masterListen,omitempty"`
	Path            string         `json:"path,omitempty"`
}

type ClientTransferEntry struct {
	Client               model.Client                  `json:"client"`
	// InboundIds remains populated in exports for older panels. New imports
	// prefer InboundRefs and resolve these IDs against the destination.
	InboundIds           []int                         `json:"inboundIds"`
	InboundRefs          []ClientTransferInboundRef    `json:"inboundRefs,omitempty"`
	FlowOverrides        map[string]string             `json:"flowOverrides,omitempty"`
	FlowOverridesByRef   map[string]string             `json:"flowOverridesByRef,omitempty"`
}

// ClientTransferScope is nil for an administrator. For a reseller, both sets
// are mandatory: a transfer can never read or mutate an attachment outside the
// assigned inbound/client namespace.
type ClientTransferScope struct {
	InboundIDs map[int]struct{}
	Emails     map[string]struct{}
	Reseller   bool
	// ResellerID is set for HTTP imports performed by a reseller. It lets the
	// successful create path persist the explicit ResellerClient mapping before
	// the new client can disappear from the next scoped read.
	ResellerID int
}

type ClientTransferEntryError struct {
	Index int    `json:"index"`
	Email string `json:"email,omitempty"`
	Error string `json:"error"`
}

type ClientTransferReport struct {
	Total         int                       `json:"total"`
	Created       int                       `json:"created"`
	Updated       int                       `json:"updated"`
	Skipped       int                       `json:"skipped"`
	Failed        int                       `json:"failed"`
	Preflight     bool                      `json:"preflight"`
	NeedRestart   bool                      `json:"needRestart"`
	Errors        []ClientTransferEntryError `json:"errors,omitempty"`
}

type ClientTransferPreflight struct {
	NewClients       int                       `json:"newClients"`
	AdditionalQuota  int64                     `json:"additionalQuota"`
	Errors           []ClientTransferEntryError `json:"errors,omitempty"`
}

func transferEmailKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func scopeAllowsEmail(scope *ClientTransferScope, email string) bool {
	return scope == nil || !scope.Reseller || func() bool {
		_, ok := scope.Emails[transferEmailKey(email)]
		return ok
	}()
}

func scopeAllowsInbound(scope *ClientTransferScope, id int) bool {
	return scope == nil || !scope.Reseller || func() bool {
		_, ok := scope.InboundIDs[id]
		return ok
	}()
}

func newTransferError(index int, email, message string) ClientTransferEntryError {
	return ClientTransferEntryError{Index: index, Email: strings.TrimSpace(email), Error: message}
}

func transferInboundRefKey(ref ClientTransferInboundRef) string {
	if strings.TrimSpace(ref.Tag) != "" {
		return "tag:" + strings.TrimSpace(ref.Tag)
	}
	return fmt.Sprintf("%s|%s|%d|%s|%s",
		ref.Protocol, strings.TrimSpace(ref.Remark), ref.Port,
		strings.TrimSpace(ref.Listen), strings.TrimSpace(ref.OriginNodeGuid))
}

func buildTransferInboundRef(db *gorm.DB, inboundID int, flowOverride string) (ClientTransferInboundRef, error) {
	var inbound model.Inbound
	if err := db.Model(model.Inbound{}).First(&inbound, inboundID).Error; err != nil {
		return ClientTransferInboundRef{}, err
	}
	ref := ClientTransferInboundRef{
		ID:             inbound.Id,
		Tag:            inbound.Tag,
		Remark:         inbound.Remark,
		Protocol:       inbound.Protocol,
		Port:           inbound.Port,
		Listen:         inbound.Listen,
		OriginNodeGuid: inbound.OriginNodeGuid,
	}
	ref.FlowOverride = &flowOverride

	var fallback model.InboundFallback
	if err := db.Where("child_id = ?", inbound.Id).
		Order("sort_order ASC, id ASC").First(&fallback).Error; err == nil {
		var master model.Inbound
		if masterErr := db.Model(model.Inbound{}).First(&master, fallback.MasterId).Error; masterErr == nil {
			ref.FallbackParent = &ClientTransferFallbackRef{
				MasterID:       master.Id,
				MasterTag:      master.Tag,
				MasterRemark:   master.Remark,
				MasterProtocol: master.Protocol,
				MasterPort:     master.Port,
				MasterListen:   master.Listen,
				Path:           fallback.Path,
			}
		}
	}
	return ref, nil
}

// ExportClients returns only configuration and attachment metadata. Current
// traffic counters, IP history, and database IDs are intentionally excluded:
// importing a backup must not overwrite usage accumulated on the destination.
// Each attachment also carries a portable inbound identity and fallback-parent
// hint so imports do not silently attach to an unrelated destination ID.
func (s *ClientService) ExportClients(scope *ClientTransferScope) (*ClientTransferEnvelope, error) {
	db := database.GetDB()
	var rows []model.ClientRecord
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	out := &ClientTransferEnvelope{
		Format:     clientTransferFormat,
		Version:    clientTransferVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Clients:    make([]ClientTransferEntry, 0, len(rows)),
	}
	for _, row := range rows {
		if !scopeAllowsEmail(scope, row.Email) {
			continue
		}
		var links []model.ClientInbound
		if err := db.Where("client_id = ?", row.Id).Order("inbound_id ASC").Find(&links).Error; err != nil {
			return nil, err
		}
		entry := ClientTransferEntry{
			Client:             *row.ToClient(),
			InboundIds:         make([]int, 0, len(links)),
			InboundRefs:        make([]ClientTransferInboundRef, 0, len(links)),
			FlowOverrides:      make(map[string]string, len(links)),
			FlowOverridesByRef: make(map[string]string, len(links)),
		}
		for _, link := range links {
			// Resellers may export a client they explicitly own, but never leak
			// an attachment to an inbound owned by another tenant.
			if !scopeAllowsInbound(scope, link.InboundId) {
				continue
			}
			ref, refErr := buildTransferInboundRef(db, link.InboundId, link.FlowOverride)
			if refErr != nil {
				return nil, refErr
			}
			entry.InboundIds = append(entry.InboundIds, link.InboundId)
			entry.InboundRefs = append(entry.InboundRefs, ref)
			entry.FlowOverrides[strconv.Itoa(link.InboundId)] = link.FlowOverride
			entry.FlowOverridesByRef[transferInboundRefKey(ref)] = link.FlowOverride
		}
		if len(entry.FlowOverrides) == 0 {
			entry.FlowOverrides = nil
		}
		if len(entry.FlowOverridesByRef) == 0 {
			entry.FlowOverridesByRef = nil
		}
		out.Clients = append(out.Clients, entry)
	}
	return out, nil
}

func transferInboundIdentityProvided(ref ClientTransferInboundRef) bool {
	return ref.Tag != "" || ref.Remark != "" || ref.Protocol != "" || ref.Port > 0 || ref.Listen != "" || ref.OriginNodeGuid != ""
}

func transferInboundIdentityMatches(ref ClientTransferInboundRef, inbound model.Inbound) bool {
	if ref.Tag != "" && inbound.Tag != ref.Tag {
		return false
	}
	if ref.Remark != "" && inbound.Remark != ref.Remark {
		return false
	}
	if ref.Protocol != "" && inbound.Protocol != ref.Protocol {
		return false
	}
	if ref.Port > 0 && inbound.Port != ref.Port {
		return false
	}
	if ref.Listen != "" && inbound.Listen != ref.Listen {
		return false
	}
	if ref.OriginNodeGuid != "" && inbound.OriginNodeGuid != ref.OriginNodeGuid {
		return false
	}
	return transferInboundIdentityProvided(ref)
}

func resolveTransferIdentity(ref ClientTransferInboundRef, inbounds []model.Inbound) (int, error) {
	matches := make([]int, 0, 2)
	for _, inbound := range inbounds {
		if transferInboundIdentityMatches(ref, inbound) {
			matches = append(matches, inbound.Id)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("inbound identity is ambiguous (%d matches)", len(matches))
	}
	return 0, nil
}

func resolveTransferFallbackParent(ref ClientTransferInboundRef, inbounds []model.Inbound, fallbacks []model.InboundFallback) (int, error) {
	if ref.FallbackParent == nil {
		return 0, nil
	}
	parent := ref.FallbackParent
	masterRef := ClientTransferInboundRef{
		ID:       parent.MasterID,
		Tag:      parent.MasterTag,
		Remark:   parent.MasterRemark,
		Protocol: parent.MasterProtocol,
		Port:     parent.MasterPort,
		Listen:   parent.MasterListen,
	}
	masterID, err := resolveTransferIdentity(masterRef, inbounds)
	if err != nil {
		return 0, err
	}
	if masterID == 0 && parent.MasterID > 0 && !transferInboundIdentityProvided(masterRef) {
		for _, inbound := range inbounds {
			if inbound.Id == parent.MasterID {
				masterID = inbound.Id
				break
			}
		}
	}
	if masterID == 0 {
		return 0, fmt.Errorf("fallback master %q was not found", parent.MasterTag)
	}
	matches := make([]int, 0, 2)
	for _, fallback := range fallbacks {
		if fallback.MasterId == masterID && fallback.Path == parent.Path {
			matches = append(matches, fallback.ChildId)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("fallback path %q is ambiguous", parent.Path)
	}
	return 0, fmt.Errorf("fallback path %q was not found on destination master", parent.Path)
}

// resolveTransferEntry maps source-panel attachment identities to destination
// inbound IDs and rewrites flow-override keys at the same time. Legacy files
// without InboundRefs retain their original ID behavior.
func resolveTransferEntry(entry ClientTransferEntry) ([]int, map[string]string, error) {
	db := database.GetDB()
	if len(entry.InboundRefs) == 0 {
		return transferUniqueInts(entry.InboundIds), cloneTransferFlowOverrides(entry.FlowOverrides), nil
	}
	var inbounds []model.Inbound
	if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil {
		return nil, nil, err
	}
	var fallbacks []model.InboundFallback
	if err := db.Model(model.InboundFallback{}).Find(&fallbacks).Error; err != nil {
		return nil, nil, err
	}
	ids := make([]int, 0, len(entry.InboundRefs))
	flows := make(map[string]string, len(entry.InboundRefs))
	seen := make(map[int]struct{}, len(entry.InboundRefs))
	for _, ref := range entry.InboundRefs {
		id, err := resolveTransferIdentity(ref, inbounds)
		if err != nil {
			return nil, nil, err
		}
		if id == 0 {
			id, err = resolveTransferFallbackParent(ref, inbounds, fallbacks)
			if err != nil {
				return nil, nil, err
			}
		}
			if id == 0 && ref.ID > 0 && !transferInboundIdentityProvided(ref) {
				for _, inbound := range inbounds {
					if inbound.Id == ref.ID {
						id = inbound.Id
						break
					}
				}
			}
		if id == 0 {
			return nil, nil, fmt.Errorf("inbound %q was not found on destination", ref.Tag)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, nil, fmt.Errorf("duplicate destination inbound %d", id)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)

		flow, hasFlow := "", false
		if ref.FlowOverride != nil {
			flow, hasFlow = *ref.FlowOverride, true
		} else if value, ok := entry.FlowOverridesByRef[transferInboundRefKey(ref)]; ok {
			flow, hasFlow = value, true
		} else if value, ok := entry.FlowOverrides[strconv.Itoa(ref.ID)]; ok {
			flow, hasFlow = value, true
		} else if value, ok := entry.FlowOverrides[strconv.Itoa(id)]; ok {
			flow, hasFlow = value, true
		}
		if hasFlow {
			flows[strconv.Itoa(id)] = flow
		}
	}
	return ids, flows, nil
}

func cloneTransferFlowOverrides(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// ValidateClientTransfer performs every deterministic check before any DB or
// daemon mutation. It is also called again by ImportClients to protect callers
// that use the service directly instead of the HTTP controller.
func (s *ClientService) ValidateClientTransfer(inboundSvc *InboundService, envelope ClientTransferEnvelope, scope *ClientTransferScope) (*ClientTransferPreflight, error) {
	if inboundSvc == nil {
		return nil, common.NewError("inbound service is required")
	}
	if envelope.Format != clientTransferFormat {
		return nil, common.NewError("unsupported client transfer format")
	}
	if envelope.Version != clientTransferVersion {
		return nil, common.NewError("unsupported client transfer version")
	}
	if len(envelope.Clients) > maxTransferClients {
		return nil, common.NewError("client transfer contains too many clients")
	}
	if scope != nil && scope.Reseller && (scope.InboundIDs == nil || scope.Emails == nil) {
		return nil, common.NewError("incomplete reseller transfer scope")
	}

	preflight := &ClientTransferPreflight{}
	seenEmails := make(map[string]int, len(envelope.Clients))
	db := database.GetDB()
	var existing []model.ClientRecord
	if err := db.Find(&existing).Error; err != nil {
		return nil, err
	}
	existingByEmail := make(map[string]*model.ClientRecord, len(existing))
	existingBySubID := make(map[string]string, len(existing))
	existingByUUID := make(map[string]string, len(existing))
	for i := range existing {
		rec := &existing[i]
		existingByEmail[transferEmailKey(rec.Email)] = rec
		if rec.SubID != "" {
			existingBySubID[rec.SubID] = rec.Email
		}
		if rec.UUID != "" {
			existingByUUID[rec.UUID] = rec.Email
		}
	}

	for index, entry := range envelope.Clients {
		email := strings.TrimSpace(entry.Client.Email)
		key := transferEmailKey(email)
		if email == "" {
			preflight.Errors = append(preflight.Errors, newTransferError(index, "", "client email is required"))
			continue
		}
		if previous, exists := seenEmails[key]; exists {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email,
				fmt.Sprintf("duplicate email in transfer file (also entry %d)", previous)))
			continue
		}
		seenEmails[key] = index
		if err := validateClientEmail(email); err != nil {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, err.Error()))
		}
		if err := validateClientSubID(entry.Client.SubID); err != nil {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, err.Error()))
		}
		// A negative expiry is the panel's supported delayed-start form, so
		// preserve it. Quota/IP/reset counters themselves cannot be negative.
		if entry.Client.TotalGB < 0 || entry.Client.LimitIP < 0 || entry.Client.Reset < 0 {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, "client quota, IP limit, and reset fields cannot be negative"))
		}
		// A reseller may import a genuinely new client, provided every target
		// inbound is in its scope. Existing clients still require an explicit
		// ResellerClient mapping; this is what makes reseller-originated imports
		// possible without reopening implicit inbound ownership.
		_, existingEmail := existingByEmail[key]
		if !scopeAllowsEmail(scope, email) && existingEmail {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, "client is outside reseller scope"))
		}

		resolvedIDs, resolvedFlows, resolveErr := resolveTransferEntry(entry)
		if resolveErr != nil {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, resolveErr.Error()))
			resolvedIDs = nil
			resolvedFlows = nil
		}
		desired := make(map[int]struct{}, len(resolvedIDs))
		for _, inboundID := range resolvedIDs {
			if inboundID <= 0 {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "inbound id must be positive"))
				continue
			}
			if _, duplicate := desired[inboundID]; duplicate {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email,
					fmt.Sprintf("duplicate inbound id %d", inboundID)))
				continue
			}
			desired[inboundID] = struct{}{}
			if !scopeAllowsInbound(scope, inboundID) {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email,
					fmt.Sprintf("inbound %d is outside reseller scope", inboundID)))
			}
			if _, err := inboundSvc.GetInbound(inboundID); err != nil {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email,
					fmt.Sprintf("inbound %d does not exist", inboundID)))
			}
		}
		if len(resolvedIDs) == 0 {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, "at least one inbound is required"))
		}
		if len(entry.InboundRefs) == 0 {
			for rawID, flow := range entry.FlowOverrides {
				inboundID, err := strconv.Atoi(strings.TrimSpace(rawID))
				if err != nil || inboundID <= 0 {
					preflight.Errors = append(preflight.Errors, newTransferError(index, email, "flow override contains an invalid inbound id"))
					continue
				}
				if _, ok := desired[inboundID]; !ok {
					preflight.Errors = append(preflight.Errors, newTransferError(index, email,
						fmt.Sprintf("flow override %d is not one of the attached inbounds", inboundID)))
				}
				if len(flow) > 512 {
					preflight.Errors = append(preflight.Errors, newTransferError(index, email, "flow override is too long"))
				}
			}
		} else {
			for rawID, flow := range resolvedFlows {
				inboundID, _ := strconv.Atoi(rawID)
				if _, ok := desired[inboundID]; !ok {
					preflight.Errors = append(preflight.Errors, newTransferError(index, email,
						fmt.Sprintf("flow override %d is not one of the attached inbounds", inboundID)))
				}
				if len(flow) > 512 {
					preflight.Errors = append(preflight.Errors, newTransferError(index, email, "flow override is too long"))
				}
			}
		}

			rec, exists := existingByEmail[key]
			if !exists {
				preflight.NewClients++
				preflight.AdditionalQuota += entry.Client.TotalGB
			} else if scope != nil && scope.Reseller && !scopeAllowsEmail(scope, rec.Email) {
				// Existing destination data is authoritative and will not be
				// replaced, so an imported quota difference is intentionally not
				// charged. The explicit mapping check still applies.
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "existing client is outside reseller scope"))
			}
		if entry.Client.SubID != "" {
			if owner, taken := existingBySubID[entry.Client.SubID]; taken && transferEmailKey(owner) != key {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "subId is already used by another client"))
			}
			for otherIndex, other := range envelope.Clients {
				if otherIndex != index && other.Client.SubID != "" && other.Client.SubID == entry.Client.SubID && transferEmailKey(other.Client.Email) != key {
					preflight.Errors = append(preflight.Errors, newTransferError(index, email, "subId collides with another imported client"))
					break
				}
			}
		}
		if entry.Client.ID != "" {
			if owner, taken := existingByUUID[entry.Client.ID]; taken && transferEmailKey(owner) != key {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "client credential ID collides with another client"))
			}
		}
	}
	return preflight, nil
}

func ensureTransferResellerClient(scope *ClientTransferScope, email string) error {
	if scope == nil || !scope.Reseller || scope.ResellerID <= 0 {
		return nil
	}
	return (&ResellerService{}).AssignClient(scope.ResellerID, email)
}

// ImportClients applies a validated envelope by email. New clients receive
// the exported configuration; existing destination client records remain
// authoritative while missing attachments are added by default. An explicit
// replaceAttachments flag is required before any destination attachment is
// detached. Successful new reseller imports also receive an explicit
// ResellerClient mapping.
func (s *ClientService) ImportClients(inboundSvc *InboundService, envelope ClientTransferEnvelope, scope *ClientTransferScope) (*ClientTransferReport, error) {
	preflight, err := s.ValidateClientTransfer(inboundSvc, envelope, scope)
	if err != nil {
		return nil, err
	}
	report := &ClientTransferReport{Total: len(envelope.Clients), Errors: preflight.Errors}
	if len(preflight.Errors) > 0 {
		report.Preflight = true
		report.Failed = len(envelope.Clients)
		return report, nil
	}

	for index, entry := range envelope.Clients {
		email := strings.TrimSpace(entry.Client.Email)
		// Keep the canonical identity consistent across Create, mapping, and
		// subsequent case-insensitive lookups. Otherwise an export containing
		// incidental surrounding whitespace could create a record that the
		// reseller mapping step cannot find.
		entry.Client.Email = email
		resolvedIDs, resolvedFlows, resolveErr := resolveTransferEntry(entry)
		if resolveErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, newTransferError(index, email, resolveErr.Error()))
			continue
		}
		entry.InboundIds = resolvedIDs
		entry.FlowOverrides = resolvedFlows
		rec, findErr := s.GetRecordByEmail(nil, email)
		if database.IsNotFound(findErr) || errors.Is(findErr, gorm.ErrRecordNotFound) {
			candidate := &model.ClientRecord{}
			lookupErr := database.GetDB().Where("LOWER(email) = LOWER(?)", email).First(candidate).Error
			if lookupErr == nil {
				rec = candidate
				findErr = nil
			} else if !database.IsNotFound(lookupErr) && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, lookupErr.Error()))
				continue
			}
		}
		if database.IsNotFound(findErr) || errors.Is(findErr, gorm.ErrRecordNotFound) {
			nr, createErr := s.Create(inboundSvc, &ClientCreatePayload{
				Client:     entry.Client,
				InboundIds: transferUniqueInts(entry.InboundIds),
			})
			if createErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, createErr.Error()))
				continue
			}
			if nr {
				report.NeedRestart = true
			}
			// Create historically normalizes Enable=true. Apply the imported
			// value and credentials once more across the exact targets.
			rec, findErr = s.GetRecordByEmail(nil, email)
			if findErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, findErr.Error()))
				continue
			}
			if findErr == nil {
				if nr2, updateErr := s.Update(inboundSvc, rec.Id, entry.Client, transferUniqueInts(entry.InboundIds)...); updateErr != nil {
					report.Failed++
					report.Errors = append(report.Errors, newTransferError(index, email, updateErr.Error()))
					continue
				} else if nr2 {
					report.NeedRestart = true
				}
			}
			if flowChanged, flowErr := s.applyTransferFlowOverrides(inboundSvc, email, entry, scope); flowErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, flowErr.Error()))
				continue
			} else if flowChanged {
				report.NeedRestart = true
			}
			if mappingErr := ensureTransferResellerClient(scope, email); mappingErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, mappingErr.Error()))
				continue
			}
			report.Created++
			continue
		}
		if findErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, newTransferError(index, email, findErr.Error()))
			continue
		}
		// Use the destination's canonical spelling for all subsequent service
		// calls, including flow-setting and reseller mapping.
		email = rec.Email

		currentIDs, idsErr := s.GetInboundIdsForRecord(rec.Id)
		if idsErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, newTransferError(index, email, idsErr.Error()))
			continue
		}
		ownedCurrent := filterTransferInboundIDs(currentIDs, scope)
		desired := transferUniqueInts(entry.InboundIds)
		desiredSet := transferIntSet(desired)
		toAttach := transferDifferenceInts(desired, currentIDs)
		toDetach := make([]int, 0)
		// Imports are additive by default. Detaching an existing attachment is
		// only allowed when the caller explicitly sets replaceAttachments, and
		// reseller imports can still affect only their owned inbound subset.
		if envelope.ReplaceAttachments {
			for _, id := range ownedCurrent {
				if _, keep := desiredSet[id]; !keep {
					toDetach = append(toDetach, id)
				}
			}
		}

		if len(toAttach) > 0 {
			if nr, attachErr := s.Attach(inboundSvc, rec.Id, toAttach); attachErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, attachErr.Error()))
				continue
			} else if nr {
				report.NeedRestart = true
			}
		}
		// Existing destination client records are authoritative. Attach uses
		// the canonical credentials/limits already stored on the destination;
		// an import must not silently rotate them or overwrite quota, enable,
		// protocol fields, traffic-related timestamps, or comments.
		if len(toDetach) > 0 {
			if nr, detachErr := s.Detach(inboundSvc, rec.Id, toDetach); detachErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, detachErr.Error()))
				continue
			} else if nr {
				report.NeedRestart = true
			}
		}
		if flowChanged, flowErr := s.applyTransferFlowOverrides(inboundSvc, email, entry, scope); flowErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, newTransferError(index, email, flowErr.Error()))
			continue
		} else if flowChanged {
			report.NeedRestart = true
		}
		if mappingErr := ensureTransferResellerClient(scope, email); mappingErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, newTransferError(index, email, mappingErr.Error()))
			continue
		}
		report.Updated++
	}
	if report.Failed == 0 && report.Created == 0 && report.Updated == 0 {
		report.Skipped = report.Total
	}
	return report, nil
}

func transferUniqueInts(values []int) []int {
	out := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func transferIntSet(values []int) map[int]struct{} {
	out := make(map[int]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func transferDifferenceInts(want, have []int) []int {
	haveSet := transferIntSet(have)
	out := make([]int, 0, len(want))
	for _, id := range want {
		if _, ok := haveSet[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

func filterTransferInboundIDs(ids []int, scope *ClientTransferScope) []int {
	if scope == nil || !scope.Reseller {
		return append([]int(nil), ids...)
	}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if scopeAllowsInbound(scope, id) {
			out = append(out, id)
		}
	}
	return out
}

// applyTransferFlowOverrides updates both the persisted per-inbound override
// and the protocol settings consumed by Xray. It never logs the client's
// credentials; only the selected flow string is written to storage.
func (s *ClientService) applyTransferFlowOverrides(inboundSvc *InboundService, email string, entry ClientTransferEntry, scope *ClientTransferScope) (bool, error) {
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	db := database.GetDB()
	changedAny := false
	for _, inboundID := range transferUniqueInts(entry.InboundIds) {
		if !scopeAllowsInbound(scope, inboundID) {
			continue
		}
		// Only an explicit per-inbound value may change an existing
		// destination association. The client-level Flow field is a
		// legacy/default value and is already handled when creating a new
		// client; applying it here would overwrite destination data.
		flow, ok := entry.FlowOverrides[strconv.Itoa(inboundID)]
		if !ok {
			continue
		}

		var link model.ClientInbound
		if err := db.Where("client_id = ? AND inbound_id = ?", rec.Id, inboundID).First(&link).Error; err != nil {
			return changedAny, err
		}
		inboundChanged := link.FlowOverride != flow
		if inboundChanged {
			if err := db.Model(&model.ClientInbound{}).
				Where("client_id = ? AND inbound_id = ?", rec.Id, inboundID).
				Update("flow_override", flow).Error; err != nil {
				return changedAny, err
			}
		}

		inbound, err := inboundSvc.GetInbound(inboundID)
		if err != nil {
			return changedAny, err
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return changedAny, err
		}
		clients, ok := settings["clients"].([]any)
		settingsChanged := false
		if ok {
			for _, raw := range clients {
				client, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				candidate, _ := client["email"].(string)
				if transferEmailKey(candidate) == transferEmailKey(email) {
					current, _ := client["flow"].(string)
					if current != flow {
						client["flow"] = flow
						settingsChanged = true
					}
				}
			}
		}
		if settingsChanged {
			encoded, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return changedAny, err
			}
			if err := db.Model(&model.Inbound{}).Where("id = ?", inboundID).Update("settings", string(encoded)).Error; err != nil {
				return changedAny, err
			}
		}
		changedAny = changedAny || inboundChanged || settingsChanged
	}
	return changedAny, nil
}
