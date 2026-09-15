package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// This file holds the helpers that keep reseller (نمایندگی) sessions inside
// their own data. Admin sessions are never affected: every helper is a no-op
// (or a straight pass-through) when the request is not a reseller session.

// resellerSession returns the reseller bound to the request, or nil when the
// caller is the panel admin.
func resellerSession(c *gin.Context) *model.Reseller {
	return session.GetLoginReseller(c)
}

// newResellerService builds a ready-to-use reseller service. The service is a
// stateless struct of sub-services, so this is cheap.
func newResellerService() *service.ResellerService {
	return &service.ResellerService{}
}

// abortForbidden answers a scoped request that reached data the reseller does
// not own. The object is reported as missing rather than forbidden so a
// reseller cannot probe for the existence of other tenants' inbounds/clients.
func abortForbidden(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "msg": msg})
}

const (
	errNotYourInbound = "inbound not found"
	errNotYourClient  = "client not found"
)

// ensureInboundOwned verifies that a reseller owns an inbound. It returns true
// when the caller may proceed (admin sessions always may).
func ensureInboundOwned(c *gin.Context, reseller *model.Reseller, inboundId int) bool {
	ok, err := newResellerService().OwnsInbound(reseller.Id, inboundId)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return false
	}
	if !ok {
		abortForbidden(c, errNotYourInbound)
		return false
	}
	return true
}

// ensureClientOwned verifies that a reseller owns a client (by email).
func ensureClientOwned(c *gin.Context, reseller *model.Reseller, email string) bool {
	ok, err := newResellerService().OwnsClient(reseller.Id, email)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return false
	}
	if !ok {
		abortForbidden(c, errNotYourClient)
		return false
	}
	return true
}

// scopeClientPageEmails narrows a client page response to the reseller's own
// clients. Used by the list endpoints the clients page calls.
func scopeClientPageEmails(reseller *model.Reseller) (*[]string, error) {
	emails, err := newResellerService().OwnedEmails(reseller.Id)
	if err != nil {
		return nil, err
	}
	if emails == nil {
		emails = []string{}
	}
	return &emails, nil
}

// inboundClients extracts settings.clients[] from an inbound payload.
func inboundClients(inbound *model.Inbound) []model.Client {
	if inbound == nil || strings.TrimSpace(inbound.Settings) == "" {
		return nil
	}
	settings := map[string][]model.Client{}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return nil
	}
	return settings["clients"]
}

// inboundClientList is the nil-safe variant of inboundClients.
func inboundClientList(inbound *model.Inbound) []model.Client {
	clients := inboundClients(inbound)
	if clients == nil {
		return []model.Client{}
	}
	return clients
}

// inboundClientBytes is the total quota (in bytes) carried by an inbound's
// settings.clients[].
func inboundClientBytes(inbound *model.Inbound) int64 {
	var total int64
	for _, client := range inboundClients(inbound) {
		total += client.TotalGB
	}
	return total
}

// checkResellerInboundUpdate keeps an inbound update inside the reseller quota:
// only the difference between the stored client list and the submitted one is
// charged, so shrinking an inbound is always allowed.
func (a *InboundController) checkResellerInboundUpdate(reseller *model.Reseller, inboundId int, payload *model.Inbound) error {
	current, err := a.inboundService.GetInbound(inboundId)
	if err != nil {
		return err
	}
	oldClients, err := a.inboundService.GetClients(current)
	if err != nil {
		oldClients = nil
	}
	newClients := inboundClients(payload)
	if newClients == nil {
		// The update did not carry a client list (the panel's inbound form
		// never does) — nothing to re-charge.
		return nil
	}
	var oldBytes, newBytes int64
	for _, client := range oldClients {
		oldBytes += client.TotalGB
	}
	for _, client := range newClients {
		newBytes += client.TotalGB
	}
	return a.resellerService.CheckClientQuota(reseller, len(newClients)-len(oldClients), newBytes-oldBytes)
}

// resellerEmailSet returns the reseller's owned client emails as a set.
func resellerEmailSet(reseller *model.Reseller) (map[string]struct{}, error) {
	return newResellerService().OwnedEmailSet(reseller.Id)
}

// ensureInboundsOwned verifies that every given inbound belongs to the
// reseller. It is used by the client endpoints that move clients between
// inbounds.
func ensureInboundsOwned(c *gin.Context, reseller *model.Reseller, inboundIds []int) bool {
	svc := newResellerService()
	for _, id := range inboundIds {
		ok, err := svc.OwnsInbound(reseller.Id, id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return false
		}
		if !ok {
			abortForbidden(c, errNotYourInbound)
			return false
		}
	}
	return true
}

// ensureEmailsOwned verifies that every given client email belongs to the
// reseller.
func ensureEmailsOwned(c *gin.Context, reseller *model.Reseller, emails []string) bool {
	owned, err := resellerEmailSet(reseller)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return false
	}
	for _, email := range emails {
		if _, ok := owned[email]; !ok {
			abortForbidden(c, errNotYourClient)
			return false
		}
	}
	return true
}

// filterOwnedEmails keeps only the emails the reseller owns.
func filterOwnedEmails(owned map[string]struct{}, emails []string) []string {
	out := make([]string, 0, len(emails))
	for _, email := range emails {
		if _, ok := owned[email]; ok {
			out = append(out, email)
		}
	}
	return out
}
