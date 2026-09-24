package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

type SubLinkProvider interface {
	SubLinksForSubId(host, subId string) ([]string, error)
	LinksForClient(host string, inbound *model.Inbound, email string) []string
}

var registeredSubLinkProvider SubLinkProvider

func RegisterSubLinkProvider(p SubLinkProvider) {
	registeredSubLinkProvider = p
}

func (s *InboundService) GetSubLinks(host, subId string) ([]string, error) {
	if registeredSubLinkProvider == nil {
		return nil, common.NewError("sub link provider not registered")
	}
	return registeredSubLinkProvider.SubLinksForSubId(host, subId)
}

func (s *InboundService) GetAllClientLinks(host string, email string) ([]string, error) {
	return s.getAllClientLinks(host, email, nil)
}

// GetAllClientLinksForInbounds returns only links whose client association is
// attached to one of the supplied inbound IDs. A non-nil allow-list is used by
// reseller sessions so an explicitly owned client cannot be used to enumerate
// links for an admin-owned inbound it also happens to share.
func (s *InboundService) GetAllClientLinksForInbounds(host, email string, allowedInboundIDs map[int]struct{}) ([]string, error) {
	return s.getAllClientLinks(host, email, allowedInboundIDs)
}

func (s *InboundService) getAllClientLinks(host string, email string, allowedInboundIDs map[int]struct{}) ([]string, error) {
	if email == "" {
		return nil, common.NewError("client email is required")
	}
	if registeredSubLinkProvider == nil {
		return nil, common.NewError("sub link provider not registered")
	}
	rec, err := s.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return nil, err
	}
	inboundIds, err := s.clientService.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return nil, err
	}
	var links []string
	for _, ibId := range inboundIds {
		if allowedInboundIDs != nil {
			if _, allowed := allowedInboundIDs[ibId]; !allowed {
				continue
			}
		}
		inbound, getErr := s.GetInbound(ibId)
		if getErr != nil {
			return nil, getErr
		}
		links = append(links, registeredSubLinkProvider.LinksForClient(host, inbound, email)...)
	}
	return links, nil
}
