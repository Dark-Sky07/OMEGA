package service

import (
	"errors"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

// validateResellerClientMutationScope is the service-side counterpart to the
// HTTP controller checks. The generic ClientService mutation methods are kept
// for administrator/internal callers; reseller routes must use the wrappers
// below so a direct service caller cannot bypass explicit client and inbound
// ownership checks.
func (s *ClientService) validateResellerClientMutationScope(resellerID int, emails []string, inboundIDs []int, rejectForeignAssociations bool) error {
	if resellerID <= 0 {
		return errors.New("reseller is required")
	}
	resellerService := &ResellerService{}
	reseller, err := resellerService.Get(resellerID)
	if err != nil {
		if database.IsNotFound(err) {
			return errors.New("reseller not found")
		}
		return err
	}
	if err := resellerService.EnsureActive(reseller); err != nil {
		return err
	}

	seenEmails := make(map[string]struct{}, len(emails))
	for _, raw := range emails {
		email := strings.TrimSpace(raw)
		key := transferEmailKey(email)
		if key == "" {
			return errors.New("client email is required")
		}
		if _, duplicate := seenEmails[key]; duplicate {
			continue
		}
		seenEmails[key] = struct{}{}
		owned, err := resellerService.OwnsClient(resellerID, email)
		if err != nil {
			return err
		}
		if !owned {
			return errors.New("client not found")
		}
		if rejectForeignAssociations {
			foreign, err := resellerService.HasForeignInboundAssociation(resellerID, email)
			if err != nil {
				return err
			}
			if foreign {
				return errors.New("client not found")
			}
		}
	}

	seenInboundIDs := make(map[int]struct{}, len(inboundIDs))
	for _, inboundID := range inboundIDs {
		if inboundID <= 0 {
			return errors.New("inbound id must be positive")
		}
		if _, duplicate := seenInboundIDs[inboundID]; duplicate {
			continue
		}
		seenInboundIDs[inboundID] = struct{}{}
		owned, err := resellerService.OwnsInbound(resellerID, inboundID)
		if err != nil {
			return err
		}
		if !owned {
			return errors.New("inbound not found")
		}
	}
	return nil
}

// AttachByEmailForReseller attaches an explicitly mapped client only to
// inbounds owned by the same reseller. A client with an out-of-scope
// association is rejected because its email-level credentials/traffic row
// cannot be partitioned safely across tenants.
func (s *ClientService) AttachByEmailForReseller(inboundSvc *InboundService, resellerID int, email string, inboundIDs []int) (bool, error) {
	resellerScopeMutationMu.RLock()
	defer resellerScopeMutationMu.RUnlock()
	if err := s.validateResellerClientMutationScope(resellerID, []string{email}, inboundIDs, true); err != nil {
		return false, err
	}
	return s.AttachByEmail(inboundSvc, email, inboundIDs)
}

// DetachByEmailManyForReseller detaches only an explicitly mapped client's
// attachments from inbounds owned by the reseller. It deliberately permits a
// shared client: the operation removes only the reseller's association and
// leaves the out-of-scope association and shared counters intact.
func (s *ClientService) DetachByEmailManyForReseller(inboundSvc *InboundService, resellerID int, email string, inboundIDs []int) (bool, error) {
	resellerScopeMutationMu.RLock()
	defer resellerScopeMutationMu.RUnlock()
	if err := s.validateResellerClientMutationScope(resellerID, []string{email}, inboundIDs, false); err != nil {
		return false, err
	}
	return s.DetachByEmailMany(inboundSvc, email, inboundIDs)
}

// BulkAttachForReseller performs the complete scope validation in the service
// before entering the optimized bulk mutation path.
func (s *ClientService) BulkAttachForReseller(inboundSvc *InboundService, resellerID int, emails []string, inboundIDs []int) (*BulkAttachResult, bool, error) {
	resellerScopeMutationMu.RLock()
	defer resellerScopeMutationMu.RUnlock()
	if err := s.validateResellerClientMutationScope(resellerID, emails, inboundIDs, true); err != nil {
		return nil, false, err
	}
	return s.BulkAttach(inboundSvc, emails, inboundIDs)
}

// BulkDetachForReseller performs the complete scope validation in the service
// before detaching only the requested reseller-owned associations.
func (s *ClientService) BulkDetachForReseller(inboundSvc *InboundService, resellerID int, emails []string, inboundIDs []int) (*BulkDetachResult, bool, error) {
	resellerScopeMutationMu.RLock()
	defer resellerScopeMutationMu.RUnlock()
	if err := s.validateResellerClientMutationScope(resellerID, emails, inboundIDs, false); err != nil {
		return nil, false, err
	}
	return s.BulkDetach(inboundSvc, emails, inboundIDs)
}

// DeleteByEmailForReseller is the service-level destructive boundary for a
// reseller. It validates ownership of both the mapped client and every target
// inbound, then delegates to the non-destructive scoped delete operation.
func (s *ClientService) DeleteByEmailForReseller(inboundSvc *InboundService, resellerID int, email string, keepTraffic bool, inboundIDs []int) (bool, error) {
	resellerScopeMutationMu.RLock()
	defer resellerScopeMutationMu.RUnlock()
	if err := s.validateResellerClientMutationScope(resellerID, []string{email}, inboundIDs, false); err != nil {
		return false, err
	}
	return s.DeleteByEmailForInbounds(inboundSvc, email, keepTraffic, inboundIDs)
}

// BulkDeleteForReseller validates all requested client/inbound pairs while the
// reseller namespace is stable, then uses the association-aware batch delete.
func (s *ClientService) BulkDeleteForReseller(inboundSvc *InboundService, resellerID int, emails []string, keepTraffic bool, inboundIDs []int) (BulkDeleteResult, bool, error) {
	resellerScopeMutationMu.RLock()
	defer resellerScopeMutationMu.RUnlock()
	if err := s.validateResellerClientMutationScope(resellerID, emails, inboundIDs, false); err != nil {
		return BulkDeleteResult{}, false, err
	}
	return s.BulkDeleteForInbounds(inboundSvc, emails, keepTraffic, inboundIDs)
}
