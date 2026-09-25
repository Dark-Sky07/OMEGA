package controller

import (
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
	// errResellerInboundReadOnly is reported when a reseller session reaches
	// an inbound write endpoint. Inbounds are assigned by the admin and are
	// read-only for resellers — unlike a wrong-owner access this is not a
	// probe-able fact, so the message says so plainly.
	errResellerInboundReadOnly = "inbounds are read-only for reseller accounts"
)

// rejectResellerInboundWrite is the guard every inbound POST handler calls
// first, before parsing any parameter or body. Reseller sessions are answered
// with the 403 contract ({success:false, msg}) and the handler must stop;
// admin sessions always proceed. It returns false when the caller must stop.
func rejectResellerInboundWrite(c *gin.Context) bool {
	if resellerSession(c) != nil {
		abortForbidden(c, errResellerInboundReadOnly)
		return false
	}
	return true
}

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
		if _, ok := owned[strings.ToLower(strings.TrimSpace(email))]; !ok {
			abortForbidden(c, errNotYourClient)
			return false
		}
	}
	return true
}

// filterOwnedEmails keeps only the explicitly mapped client emails. It keeps
// the canonical spelling returned by the traffic source in the response.
func filterOwnedEmails(owned map[string]struct{}, emails []string) []string {
	out := make([]string, 0, len(emails))
	for _, email := range emails {
		if _, ok := owned[strings.ToLower(strings.TrimSpace(email))]; ok {
			out = append(out, email)
		}
	}
	return out
}

func filterInboundIDs(ids []int, owned map[int]struct{}) []int {
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, ok := owned[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
