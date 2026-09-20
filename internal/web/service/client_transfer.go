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
	Format     string                 `json:"format"`
	Version    int                    `json:"version"`
	ExportedAt string                 `json:"exportedAt"`
	Clients    []ClientTransferEntry `json:"clients"`
}

type ClientTransferEntry struct {
	Client        model.Client       `json:"client"`
	InboundIds    []int              `json:"inboundIds"`
	FlowOverrides map[string]string  `json:"flowOverrides,omitempty"`
}

// ClientTransferScope is nil for an administrator. For a reseller, both sets
// are mandatory: a transfer can never read or mutate an attachment outside the
// assigned inbound/client namespace.
type ClientTransferScope struct {
	InboundIDs map[int]struct{}
	Emails     map[string]struct{}
	Reseller   bool
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

// ExportClients returns only configuration and attachment metadata. Current
// traffic counters, IP history, and database IDs are intentionally excluded:
// importing a backup must not overwrite usage accumulated on the destination.
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
			Client:        *row.ToClient(),
			InboundIds:    make([]int, 0, len(links)),
			FlowOverrides: make(map[string]string, len(links)),
		}
		for _, link := range links {
			// Resellers may export a client they explicitly own, but never leak
			// an attachment to an inbound owned by another tenant.
			if !scopeAllowsInbound(scope, link.InboundId) {
				continue
			}
			entry.InboundIds = append(entry.InboundIds, link.InboundId)
			entry.FlowOverrides[strconv.Itoa(link.InboundId)] = link.FlowOverride
		}
		if len(entry.FlowOverrides) == 0 {
			entry.FlowOverrides = nil
		}
		out.Clients = append(out.Clients, entry)
	}
	return out, nil
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
		if !scopeAllowsEmail(scope, email) {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, "client is outside reseller scope"))
		}

		desired := make(map[int]struct{}, len(entry.InboundIds))
		for _, inboundID := range entry.InboundIds {
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
		if len(entry.InboundIds) == 0 {
			preflight.Errors = append(preflight.Errors, newTransferError(index, email, "at least one inbound is required"))
		}
		for rawID := range entry.FlowOverrides {
			inboundID, err := strconv.Atoi(strings.TrimSpace(rawID))
			if err != nil || inboundID <= 0 {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "flow override contains an invalid inbound id"))
				continue
			}
			if _, ok := desired[inboundID]; !ok {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email,
					fmt.Sprintf("flow override %d is not one of the attached inbounds", inboundID)))
			}
			if len(entry.FlowOverrides[rawID]) > 512 {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "flow override is too long"))
			}
		}

		rec, exists := existingByEmail[key]
		if !exists {
			preflight.NewClients++
			preflight.AdditionalQuota += entry.Client.TotalGB
		} else {
			if rec.Email != email {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "email differs only by case from an existing client"))
			}
			if scope != nil && scope.Reseller && !scopeAllowsEmail(scope, rec.Email) {
				preflight.Errors = append(preflight.Errors, newTransferError(index, email, "existing client is outside reseller scope"))
			}
			if entry.Client.TotalGB > rec.TotalGB {
				preflight.AdditionalQuota += entry.Client.TotalGB - rec.TotalGB
			}
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

// ImportClients applies a validated envelope as an email-keyed upsert. Existing
// reseller-owned attachments outside the file's owned target set are retained;
// administrator imports reconcile attachments exactly.
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
		rec, findErr := s.GetRecordByEmail(nil, email)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			nr, createErr := s.Create(inboundSvc, &ClientCreatePayload{
				Client:     entry.Client,
				InboundIds: uniqueInts(entry.InboundIds),
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
				if nr2, updateErr := s.Update(inboundSvc, rec.Id, entry.Client, uniqueInts(entry.InboundIds)...); updateErr != nil {
					report.Failed++
					report.Errors = append(report.Errors, newTransferError(index, email, updateErr.Error()))
					continue
				} else if nr2 {
					report.NeedRestart = true
				}
			}
			if flowErr := s.applyTransferFlowOverrides(inboundSvc, email, entry, scope); flowErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, flowErr.Error()))
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

		currentIDs, idsErr := s.GetInboundIdsForRecord(rec.Id)
		if idsErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, newTransferError(index, email, idsErr.Error()))
			continue
		}
		ownedCurrent := filterTransferInboundIDs(currentIDs, scope)
		desired := uniqueInts(entry.InboundIds)
		desiredSet := intSet(desired)
		toAttach := differenceInts(desired, currentIDs)
		toDetach := make([]int, 0)
		for _, id := range ownedCurrent {
			if _, keep := desiredSet[id]; !keep {
				toDetach = append(toDetach, id)
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
		updateIDs := desired
		if scope != nil && scope.Reseller {
			updateIDs = filterTransferInboundIDs(desired, scope)
		}
		if len(updateIDs) > 0 {
			if nr, updateErr := s.Update(inboundSvc, rec.Id, entry.Client, updateIDs...); updateErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, updateErr.Error()))
				continue
			} else if nr {
				report.NeedRestart = true
			}
		}
		if len(toDetach) > 0 {
			if nr, detachErr := s.Detach(inboundSvc, rec.Id, toDetach); detachErr != nil {
				report.Failed++
				report.Errors = append(report.Errors, newTransferError(index, email, detachErr.Error()))
				continue
			} else if nr {
				report.NeedRestart = true
			}
		}
		if flowErr := s.applyTransferFlowOverrides(inboundSvc, email, entry, scope); flowErr != nil {
			report.Failed++
			report.Errors = append(report.Errors, newTransferError(index, email, flowErr.Error()))
			continue
		}
		report.Updated++
	}
	if report.Failed == 0 && report.Created == 0 && report.Updated == 0 {
		report.Skipped = report.Total
	}
	return report, nil
}

func uniqueInts(values []int) []int {
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

func intSet(values []int) map[int]struct{} {
	out := make(map[int]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func differenceInts(want, have []int) []int {
	haveSet := intSet(have)
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
func (s *ClientService) applyTransferFlowOverrides(inboundSvc *InboundService, email string, entry ClientTransferEntry, scope *ClientTransferScope) error {
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return err
	}
	db := database.GetDB()
	for _, inboundID := range uniqueInts(entry.InboundIds) {
		if !scopeAllowsInbound(scope, inboundID) {
			continue
		}
		flow := entry.Client.Flow
		if override, ok := entry.FlowOverrides[strconv.Itoa(inboundID)]; ok {
			flow = override
		}
		if err := db.Model(&model.ClientInbound{}).
			Where("client_id = ? AND inbound_id = ?", rec.Id, inboundID).
			Update("flow_override", flow).Error; err != nil {
			return err
		}
		inbound, err := inboundSvc.GetInbound(inboundID)
		if err != nil {
			return err
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return err
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			continue
		}
		changed := false
		for _, raw := range clients {
			client, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			candidate, _ := client["email"].(string)
			if transferEmailKey(candidate) == transferEmailKey(email) {
				client["flow"] = flow
				changed = true
			}
		}
		if changed {
			encoded, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return err
			}
			if err := db.Model(&model.Inbound{}).Where("id = ?", inboundID).Update("settings", string(encoded)).Error; err != nil {
				return err
			}
		}
	}
	return db.Model(&model.ClientRecord{}).Where("id = ?", rec.Id).Update("flow", entry.Client.Flow).Error
}
