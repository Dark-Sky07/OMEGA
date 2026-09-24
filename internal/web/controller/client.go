package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/websocket"

	"github.com/gin-gonic/gin"
)

func notifyClientsChanged() {
	websocket.BroadcastInvalidate(websocket.MessageTypeClients)
}

func parseInboundIdsQuery(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		if id, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

type ClientController struct {
	clientService   service.ClientService
	inboundService  service.InboundService
	xrayService     service.XrayService
	settingService  service.SettingService
	resellerService service.ResellerService
}

func NewClientController(g *gin.RouterGroup) *ClientController {
	a := &ClientController{}
	a.initRouter(g)
	return a
}

// resellerOwnedClientInboundIDs intersects the canonical client associations
// with the reseller's inbound capability. Explicit ResellerClient ownership
// is checked by the route separately; this intersection prevents a by-email
// operation from silently traversing an unrelated admin-owned association.
func (a *ClientController) resellerOwnedClientInboundIDs(reseller *model.Reseller, email string) ([]int, error) {
	owned, err := a.resellerService.OwnedInboundIdSet(reseller.Id)
	if err != nil {
		return nil, err
	}
	ids, err := a.clientService.GetInboundIdsForEmail(nil, email)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, ok := owned[id]; ok {
			out = append(out, id)
		}
	}
	return out, nil
}

func (a *ClientController) resellerClientIsolated(reseller *model.Reseller, email string) (bool, error) {
	isolated, err := a.resellerService.OwnedIsolatedAssociatedEmailSet(reseller.Id)
	if err != nil {
		return false, err
	}
	_, ok := isolated[strings.ToLower(strings.TrimSpace(email))]
	return ok, nil
}

func (a *ClientController) resellerOwnedEmails(reseller *model.Reseller, emails []string) ([]string, error) {
	isolated, err := a.resellerService.OwnedIsolatedAssociatedEmailSet(reseller.Id)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		if _, ok := isolated[key]; !ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, email)
	}
	return out, nil
}

func (a *ClientController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.GET("/list/paged", a.listPaged)
	g.GET("/get/:email", a.get)
	g.GET("/traffic/:email", a.getTrafficByEmail)
	g.GET("/subLinks/:subId", a.getSubLinks)
	g.GET("/links/:email", a.getClientLinks)
	g.GET("/openvpn/:email", a.getOpenvpnProfile)
	g.GET("/export", a.exportClients)

	g.POST("/add", a.create)
	g.POST("/import", a.importClients)
	g.POST("/update/:email", a.update)
	g.POST("/del/:email", a.delete)
	g.POST("/:email/attach", a.attach)
	g.POST("/:email/detach", a.detach)
	g.POST("/resetAllTraffics", a.resetAllTraffics)
	g.POST("/delDepleted", a.delDepleted)
	g.POST("/bulkAdjust", a.bulkAdjust)
	g.POST("/bulkDel", a.bulkDelete)
	g.POST("/bulkCreate", a.bulkCreate)
	g.POST("/bulkAttach", a.bulkAttach)
	g.POST("/bulkDetach", a.bulkDetach)
	g.POST("/bulkResetTraffic", a.bulkResetTraffic)
	g.POST("/resetTraffic/:email", a.resetTrafficByEmail)
	g.POST("/updateTraffic/:email", a.updateTrafficByEmail)
	g.POST("/ips/:email", a.getIps)
	g.POST("/clearIps/:email", a.clearIps)
	g.POST("/onlines", a.onlines)
	g.POST("/onlinesByGuid", a.onlinesByGuid)
	g.POST("/activeInbounds", a.activeInbounds)
	g.POST("/lastOnline", a.lastOnline)
}

func (a *ClientController) list(c *gin.Context) {
	rows, err := a.clientService.List()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		owned, err := resellerEmailSet(reseller)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
			return
		}
		ownedInbounds, err := newResellerService().OwnedInboundIdSet(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
			return
		}
		filtered := make([]service.ClientWithAttachments, 0, len(rows))
		for _, row := range rows {
			if _, ok := owned[strings.ToLower(strings.TrimSpace(row.Email))]; !ok {
				continue
			}
			row.InboundIds = filterInboundIDs(row.InboundIds, ownedInbounds)
			// ResellerClient is the explicit client authorization boundary.
			// The traffic row is an email aggregate, so preserve its counters
			// for an explicitly mapped client but never expose its legacy
			// inbound identifier.
			if row.Traffic != nil {
				traffic := *row.Traffic
				traffic.InboundId = 0
				row.Traffic = &traffic
			}
			filtered = append(filtered, row)
		}
		jsonObj(c, filtered, nil)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ClientController) listPaged(c *gin.Context) {
	var params service.ClientPageParams
	if err := c.ShouldBindQuery(&params); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		scope, err := scopeClientPageEmails(reseller)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
			return
		}
		ownedInbounds, err := newResellerService().OwnedInboundIdSet(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
			return
		}
		params.ScopeEmails = scope
		params.ScopeInboundIDs = &ownedInbounds
	}
	resp, err := a.clientService.ListPaged(&a.inboundService, &a.settingService, params)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	jsonObj(c, resp, nil)
}

func (a *ClientController) clientTransferScope(c *gin.Context) (*service.ClientTransferScope, *model.Reseller, error) {
	reseller := resellerSession(c)
	if reseller == nil {
		return nil, nil, nil
	}
	if err := a.resellerService.EnsureActive(reseller); err != nil {
		return nil, reseller, err
	}
	inbounds, err := a.resellerService.OwnedInboundIdSet(reseller.Id)
	if err != nil {
		return nil, reseller, err
	}
	emails, err := a.resellerService.OwnedEmailSet(reseller.Id)
	if err != nil {
		return nil, reseller, err
	}
	normalizedEmails := make(map[string]struct{}, len(emails))
	for email := range emails {
		normalizedEmails[strings.ToLower(strings.TrimSpace(email))] = struct{}{}
	}
	return &service.ClientTransferScope{
		InboundIDs: inbounds,
		Emails:     normalizedEmails,
		Reseller:   true,
		ResellerID: reseller.Id,
	}, reseller, nil
}

func (a *ClientController) exportClients(c *gin.Context) {
	scope, _, err := a.clientTransferScope(c)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	envelope, err := a.clientService.ExportClients(scope)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, envelope, nil)
}

func (a *ClientController) importClients(c *gin.Context) {
	var envelope service.ClientTransferEnvelope
	if err := c.ShouldBindJSON(&envelope); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	scope, reseller, err := a.clientTransferScope(c)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	preflight, err := a.clientService.ValidateClientTransfer(&a.inboundService, envelope, scope)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if len(preflight.Errors) > 0 {
		jsonMsgObj(c, "Client transfer validation failed", preflight, errors.New("preflight validation failed"))
		return
	}
	if reseller != nil {
		if err := a.resellerService.CheckClientQuota(reseller, preflight.NewClients, preflight.AdditionalQuota); err != nil {
			jsonMsgObj(c, "Client transfer quota validation failed", preflight, err)
			return
		}
	}
	report, err := a.clientService.ImportClients(&a.inboundService, envelope, scope)
	if err != nil {
		if report != nil && report.NeedRestart {
			a.xrayService.SetToNeedRestart()
		}
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if report.NeedRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
	jsonObj(c, report, nil)
}

func (a *ClientController) get(c *gin.Context) {
	email := c.Param("email")
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	rec, err := a.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	inboundIds, err := a.clientService.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	var scopedInbounds map[int]struct{}
	if reseller := resellerSession(c); reseller != nil {
		ownedInbounds, ownedErr := newResellerService().OwnedInboundIdSet(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "get"), ownedErr)
			return
		}
		scopedInbounds = ownedInbounds
		inboundIds = filterInboundIDs(inboundIds, ownedInbounds)
	}
	var flow string
	if scopedInbounds != nil {
		flow, err = a.clientService.EffectiveFlowForInbounds(nil, rec.Id, scopedInbounds)
	} else {
		flow, err = a.clientService.EffectiveFlow(nil, rec.Id)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	rec.Flow = flow
	// Consumed bytes (up+down, including cross-node global overlay) so API
	// consumers can pair usage with the client's totalGB quota (#4973).
	// Best-effort: a traffic lookup failure must not break the client fetch.
	var usedTraffic int64
	if scopedInbounds != nil {
		if traffic, trafficErr := a.inboundService.GetClientTrafficByEmailForInbounds(email, scopedInbounds); trafficErr == nil && traffic != nil {
			usedTraffic = traffic.Up + traffic.Down
		}
	} else if traffic, trafficErr := a.inboundService.GetClientTrafficByEmail(email); trafficErr == nil && traffic != nil {
		usedTraffic = traffic.Up + traffic.Down
	}
	// Best-effort: a traffic lookup failure must not break the client fetch.
	jsonObj(c, gin.H{"client": rec, "inboundIds": inboundIds, "usedTraffic": usedTraffic}, nil)
}

func (a *ClientController) create(c *gin.Context) {
	var payload service.ClientCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	newResellerClient := false
	if reseller := resellerSession(c); reseller != nil {
		if err := a.scopeResellerClientWrite(reseller, []service.ClientCreatePayload{payload}); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		_, lookupErr := a.clientService.GetRecordByEmail(nil, strings.TrimSpace(payload.Client.Email))
		newResellerClient = database.IsNotFound(lookupErr)
		if lookupErr != nil && !newResellerClient {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), lookupErr)
			return
		}
	}
	needRestart, err := a.clientService.Create(&a.inboundService, &payload)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		// Reseller-created clients must remain visible under explicit-only
		// ownership, even when the target inbound is also reseller-owned.
		if err := a.resellerService.AssignClient(reseller.Id, strings.TrimSpace(payload.Client.Email)); err != nil {
			// Create and ownership are separate service layers because the
			// daemon/runtime mutation cannot run inside the reseller mapping
			// transaction. If this was a new client, compensate the persisted
			// client/attachment mutation so a mapping failure cannot leave an
			// invisible orphan behind. Existing clients are never deleted.
			if newResellerClient {
				if _, rollbackErr := a.clientService.DeleteByEmail(&a.inboundService, payload.Client.Email, false); rollbackErr != nil {
					jsonMsg(c, "client ownership failed; rollback also failed", fmt.Errorf("%w (rollback: %v)", err, rollbackErr))
					return
				}
			}
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientAddSuccess"), pendingNodeObj(a.inboundService.AnyNodePending(payload.InboundIds)), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

func (a *ClientController) update(c *gin.Context) {
	email := c.Param("email")
	var updated model.Client
	if err := c.ShouldBindJSON(&updated); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	inboundFilter := parseInboundIdsQuery(c.Query("inboundIds"))
	var resellerInboundFilter []int
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		if err := a.resellerService.EnsureActive(reseller); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		attachedOwned, attachedErr := a.resellerOwnedClientInboundIDs(reseller, email)
		if attachedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), attachedErr)
			return
		}
		isolated, isolatedErr := a.resellerClientIsolated(reseller, email)
		if isolatedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), isolatedErr)
			return
		}
		if !isolated {
			abortForbidden(c, errNotYourClient)
			return
		}
		if len(inboundFilter) > 0 {
			if !ensureInboundsOwned(c, reseller, inboundFilter) {
				return
			}
			requested := make(map[int]struct{}, len(inboundFilter))
			for _, id := range inboundFilter {
				requested[id] = struct{}{}
			}
			resellerInboundFilter = filterInboundIDs(attachedOwned, requested)
		} else {
			// The browser normally omits inboundIds. Never let that omission
			// fall through to the admin-wide UpdateByEmail behavior, and never
			// update a mapped client that has no association in this tenant.
			resellerInboundFilter = attachedOwned
		}
		if len(resellerInboundFilter) == 0 {
			abortForbidden(c, errNotYourClient)
			return
		}
		if err := a.checkResellerClientQuotaDelta(reseller, email, updated.TotalGB); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		if strings.TrimSpace(updated.Email) != "" && !strings.EqualFold(strings.TrimSpace(updated.Email), strings.TrimSpace(email)) {
			// The ResellerClient row is the explicit visibility boundary. Do
			// not let a reseller rename a canonical client and strand or
			// accidentally retarget that mapping.
			abortForbidden(c, errNotYourClient)
			return
		}
		updated.Email = email
	}
	var (
		needRestart bool
		err         error
	)
	if resellerSession(c) != nil {
		needRestart, err = a.clientService.UpdateByEmailForInbounds(&a.inboundService, email, updated, resellerInboundFilter)
	} else {
		needRestart, err = a.clientService.UpdateByEmail(&a.inboundService, email, updated, inboundFilter...)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientUpdateSuccess"), pendingNodeObj(a.clientService.HasPendingNode(&a.inboundService, email)), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

func (a *ClientController) delete(c *gin.Context) {
	email := c.Param("email")
	keepTraffic := c.Query("keepTraffic") == "1"
	var needRestart bool
	var err error
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		owned, ownedErr := a.resellerService.OwnedInboundIds(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		needRestart, err = a.clientService.DeleteByEmailForReseller(&a.inboundService, reseller.Id, email, keepTraffic, owned)
		if err == nil {
			// Deleting from the reseller's visible scope also revokes the
			// explicit mapping. Any remaining admin-owned association is left
			// intact by DeleteByEmailForInbounds.
			err = a.resellerService.UnassignClient(reseller.Id, email)
		}
	} else {
		needRestart, err = a.clientService.DeleteByEmail(&a.inboundService, email, keepTraffic)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientDeleteSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

type attachDetachBody struct {
	InboundIds []int `json:"inboundIds"`
}

func (a *ClientController) attach(c *gin.Context) {
	email := c.Param("email")
	var body attachDetachBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) || !ensureInboundsOwned(c, reseller, body.InboundIds) {
			return
		}
		foreign, foreignErr := a.resellerService.HasForeignInboundAssociation(reseller.Id, email)
		if foreignErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), foreignErr)
			return
		}
		if foreign {
			abortForbidden(c, errNotYourClient)
			return
		}
	}
	var needRestart bool
	var err error
	if reseller := resellerSession(c); reseller != nil {
		needRestart, err = a.clientService.AttachByEmailForReseller(&a.inboundService, reseller.Id, email, body.InboundIds)
	} else {
		needRestart, err = a.clientService.AttachByEmail(&a.inboundService, email, body.InboundIds)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientAddSuccess"), pendingNodeObj(a.inboundService.AnyNodePending(body.InboundIds)), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

func (a *ClientController) resetAllTraffics(c *gin.Context) {
	if reseller := resellerSession(c); reseller != nil {
		mapped, err := newResellerService().OwnedEmails(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		owned, err := a.resellerOwnedEmails(reseller, mapped)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		ownedInboundIDs, err := newResellerService().OwnedInboundIds(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		if len(owned) == 0 || len(ownedInboundIDs) == 0 {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetAllClientTrafficSuccess"), nil)
			return
		}
		_, needRestart, err := a.clientService.BulkResetTrafficForInbounds(&a.inboundService, owned, ownedInboundIDs)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		if needRestart {
			a.xrayService.SetToNeedRestart()
		}
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetAllClientTrafficSuccess"), nil)
		notifyClientsChanged()
		return
	}
	needRestart, err := a.clientService.ResetAllTraffics()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetAllClientTrafficSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

type bulkAdjustRequest struct {
	Emails   []string `json:"emails"`
	AddDays  int      `json:"addDays"`
	AddBytes int64    `json:"addBytes"`
}

func (a *ClientController) bulkAdjust(c *gin.Context) {
	var req bulkAdjustRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureEmailsOwned(c, reseller, req.Emails) {
			return
		}
		if err := a.resellerService.EnsureActive(reseller); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		isolated, isolatedErr := a.resellerService.OwnedIsolatedAssociatedEmailSet(reseller.Id)
		if isolatedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), isolatedErr)
			return
		}
		for _, email := range req.Emails {
			if _, ok := isolated[strings.ToLower(strings.TrimSpace(email))]; !ok {
				abortForbidden(c, errNotYourClient)
				return
			}
		}
		if err := a.resellerService.CheckTrafficQuota(reseller, int64(len(req.Emails))*req.AddBytes); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	var result service.BulkAdjustResult
	var needRestart bool
	var err error
	if reseller := resellerSession(c); reseller != nil {
		owned, ownedErr := a.resellerService.OwnedInboundIds(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		result, needRestart, err = a.clientService.BulkAdjustForInbounds(&a.inboundService, req.Emails, req.AddDays, req.AddBytes, owned)
	} else {
		result, needRestart, err = a.clientService.BulkAdjust(&a.inboundService, req.Emails, req.AddDays, req.AddBytes)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, result, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

type bulkDeleteRequest struct {
	Emails      []string `json:"emails"`
	KeepTraffic bool     `json:"keepTraffic"`
}

type bulkAttachRequest struct {
	Emails     []string `json:"emails"`
	InboundIds []int    `json:"inboundIds"`
}

func (a *ClientController) bulkAttach(c *gin.Context) {
	var req bulkAttachRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureEmailsOwned(c, reseller, req.Emails) || !ensureInboundsOwned(c, reseller, req.InboundIds) {
			return
		}
		for _, email := range req.Emails {
			foreign, foreignErr := a.resellerService.HasForeignInboundAssociation(reseller.Id, email)
			if foreignErr != nil {
				jsonMsg(c, I18nWeb(c, "somethingWentWrong"), foreignErr)
				return
			}
			if foreign {
				abortForbidden(c, errNotYourClient)
				return
			}
		}
	}
	var result *service.BulkAttachResult
	var needRestart bool
	var err error
	if reseller := resellerSession(c); reseller != nil {
		result, needRestart, err = a.clientService.BulkAttachForReseller(&a.inboundService, reseller.Id, req.Emails, req.InboundIds)
	} else {
		result, needRestart, err = a.clientService.BulkAttach(&a.inboundService, req.Emails, req.InboundIds)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, result, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

type bulkDetachRequest struct {
	Emails     []string `json:"emails"`
	InboundIds []int    `json:"inboundIds"`
}

func (a *ClientController) bulkDetach(c *gin.Context) {
	var req bulkDetachRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureEmailsOwned(c, reseller, req.Emails) || !ensureInboundsOwned(c, reseller, req.InboundIds) {
			return
		}
	}
	var result *service.BulkDetachResult
	var needRestart bool
	var err error
	if reseller := resellerSession(c); reseller != nil {
		result, needRestart, err = a.clientService.BulkDetachForReseller(&a.inboundService, reseller.Id, req.Emails, req.InboundIds)
	} else {
		result, needRestart, err = a.clientService.BulkDetach(&a.inboundService, req.Emails, req.InboundIds)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, result, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

func (a *ClientController) bulkDelete(c *gin.Context) {
	var req bulkDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var (
		result      service.BulkDeleteResult
		needRestart bool
		err         error
	)
	if reseller := resellerSession(c); reseller != nil {
		if !ensureEmailsOwned(c, reseller, req.Emails) {
			return
		}
		owned, ownedErr := a.resellerService.OwnedInboundIds(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		result, needRestart, err = a.clientService.BulkDeleteForReseller(&a.inboundService, reseller.Id, req.Emails, req.KeepTraffic, owned)
		if err == nil {
			mappingEmails := append(append([]string(nil), result.DeletedEmails...), result.UnassignedEmails...)
			if len(mappingEmails) > 0 {
				err = a.resellerService.UnassignClients(reseller.Id, mappingEmails)
			}
		}
	} else {
		result, needRestart, err = a.clientService.BulkDelete(&a.inboundService, req.Emails, req.KeepTraffic)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, result, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

func (a *ClientController) bulkCreate(c *gin.Context) {
	var payloads []service.ClientCreatePayload
	if err := c.ShouldBindJSON(&payloads); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	newResellerClients := map[string]struct{}{}
	if reseller := resellerSession(c); reseller != nil {
		if err := a.scopeResellerClientWrite(reseller, payloads); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		for _, payload := range payloads {
			email := strings.TrimSpace(payload.Client.Email)
			if email == "" {
				continue
			}
			if _, lookupErr := a.clientService.GetRecordByEmail(nil, email); database.IsNotFound(lookupErr) {
				newResellerClients[strings.ToLower(email)] = struct{}{}
			}
		}
	}
	result, needRestart, err := a.clientService.BulkCreate(&a.inboundService, payloads)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil && len(result.CreatedEmails) > 0 {
		// Persist all explicit visibility mappings in one transaction. Do not
		// loop over one-request assignments: a late failure must not leave a
		// partially visible bulk create.
		if err := a.resellerService.AssignClients(reseller.Id, result.CreatedEmails); err != nil {
			var rollbackErr error
			for _, email := range result.CreatedEmails {
				if _, isNew := newResellerClients[strings.ToLower(strings.TrimSpace(email))]; !isNew {
					continue
				}
				if _, err := a.clientService.DeleteByEmail(&a.inboundService, email, false); err != nil && rollbackErr == nil {
					rollbackErr = err
				}
			}
			if rollbackErr != nil {
				jsonMsg(c, "client ownership failed; rollback also failed", fmt.Errorf("%w (rollback: %v)", err, rollbackErr))
				return
			}
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	jsonObj(c, result, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

func (a *ClientController) delDepleted(c *gin.Context) {
	if reseller := resellerSession(c); reseller != nil {
		deleted, needRestart, err := a.delDepletedForReseller(reseller)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		jsonObj(c, gin.H{"deleted": deleted}, nil)
		if needRestart {
			a.xrayService.SetToNeedRestart()
		}
		notifyClientsChanged()
		return
	}
	deleted, needRestart, err := a.clientService.DelDepleted(&a.inboundService)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"deleted": deleted}, nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

func (a *ClientController) resetTrafficByEmail(c *gin.Context) {
	email := c.Param("email")
	var needRestart bool
	var err error
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		owned, ownedErr := a.resellerOwnedClientInboundIDs(reseller, email)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		isolated, isolatedErr := a.resellerClientIsolated(reseller, email)
		if isolatedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), isolatedErr)
			return
		}
		if len(owned) == 0 || !isolated {
			abortForbidden(c, errNotYourClient)
			return
		}
		needRestart, err = a.clientService.ResetTrafficByEmailForInbounds(&a.inboundService, email, owned)
	} else {
		needRestart, err = a.clientService.ResetTrafficByEmail(&a.inboundService, email)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetInboundClientTrafficSuccess"), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

type trafficUpdateRequest struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

func (a *ClientController) updateTrafficByEmail(c *gin.Context) {
	email := c.Param("email")
	var req trafficUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var scopedInboundIDs map[int]struct{}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		var ownedErr error
		scopedInboundIDs, ownedErr = a.resellerService.OwnedInboundIdSet(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		isolated, isolatedErr := a.resellerClientIsolated(reseller, email)
		if isolatedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), isolatedErr)
			return
		}
		if len(scopedInboundIDs) == 0 || !isolated {
			abortForbidden(c, errNotYourClient)
			return
		}
	}
	var trafficErr error
	if scopedInboundIDs != nil {
		trafficErr = a.inboundService.UpdateClientTrafficByEmailForInbounds(email, req.Upload, req.Download, scopedInboundIDs)
	} else {
		trafficErr = a.inboundService.UpdateClientTrafficByEmail(email, req.Upload, req.Download)
	}
	if trafficErr != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), trafficErr)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientUpdateSuccess"), nil)
	notifyClientsChanged()
}

func (a *ClientController) getIps(c *gin.Context) {
	email := c.Param("email")
	var scopedInboundIDs map[int]struct{}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		var ownedErr error
		scopedInboundIDs, ownedErr = a.resellerService.OwnedInboundIdSet(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		isolated, isolatedErr := a.resellerClientIsolated(reseller, email)
		if isolatedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), isolatedErr)
			return
		}
		if len(scopedInboundIDs) == 0 || !isolated {
			// IP history is email-keyed, just like ClientTraffic. A client
			// shared with an admin-owned inbound has unpartitionable history;
			// fail closed instead of returning another tenant's observations.
			abortForbidden(c, errNotYourClient)
			return
		}
	}
	var ips string
	var err error
	if scopedInboundIDs != nil {
		ips, err = a.inboundService.GetInboundClientIpsForInbounds(email, scopedInboundIDs)
	} else {
		ips, err = a.inboundService.GetInboundClientIps(email)
	}
	if err != nil || ips == "" {
		jsonObj(c, "No IP Record", nil)
		return
	}
	type ipWithTimestamp struct {
		IP        string `json:"ip"`
		Timestamp int64  `json:"timestamp"`
	}
	var ipsWithTime []ipWithTimestamp
	if err := json.Unmarshal([]byte(ips), &ipsWithTime); err == nil && len(ipsWithTime) > 0 {
		formatted := make([]string, 0, len(ipsWithTime))
		for _, item := range ipsWithTime {
			if item.IP == "" {
				continue
			}
			if item.Timestamp > 0 {
				ts := time.Unix(item.Timestamp, 0).Local().Format("2006-01-02 15:04:05")
				formatted = append(formatted, fmt.Sprintf("%s (%s)", item.IP, ts))
				continue
			}
			formatted = append(formatted, item.IP)
		}
		jsonObj(c, formatted, nil)
		return
	}
	var oldIps []string
	if err := json.Unmarshal([]byte(ips), &oldIps); err == nil && len(oldIps) > 0 {
		jsonObj(c, oldIps, nil)
		return
	}
	jsonObj(c, ips, nil)
}

func (a *ClientController) clearIps(c *gin.Context) {
	email := c.Param("email")
	var scopedInboundIDs map[int]struct{}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		var ownedErr error
		scopedInboundIDs, ownedErr = a.resellerService.OwnedInboundIdSet(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		isolated, isolatedErr := a.resellerClientIsolated(reseller, email)
		if isolatedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), isolatedErr)
			return
		}
		if len(scopedInboundIDs) == 0 || !isolated {
			abortForbidden(c, errNotYourClient)
			return
		}
	}
	var err error
	if scopedInboundIDs != nil {
		err = a.inboundService.ClearClientIpsForInbounds(email, scopedInboundIDs)
	} else {
		err = a.inboundService.ClearClientIps(email)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.updateSuccess"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.logCleanSuccess"), nil)
}

func (a *ClientController) onlines(c *gin.Context) {
	online := a.inboundService.GetOnlineClients()
	if reseller := resellerSession(c); reseller != nil {
		owned, err := newResellerService().OwnedEmailSet(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		online = filterOwnedEmails(owned, online)
	}
	jsonObj(c, online, nil)
}

func (a *ClientController) onlinesByGuid(c *gin.Context) {
	byGuid := a.inboundService.GetOnlineClientsByGuid()
	if reseller := resellerSession(c); reseller != nil {
		owned, err := newResellerService().OwnedEmailSet(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		scoped := make(map[string][]string, len(byGuid))
		for guid, emails := range byGuid {
			if filtered := filterOwnedEmails(owned, emails); len(filtered) > 0 {
				scoped[guid] = filtered
			}
		}
		jsonObj(c, scoped, nil)
		return
	}
	jsonObj(c, byGuid, nil)
}

func (a *ClientController) activeInbounds(c *gin.Context) {
	active := a.inboundService.GetActiveInboundsByGuid()
	if reseller := resellerSession(c); reseller != nil {
		inbounds, err := a.inboundService.GetInboundsForReseller(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		ownedTags := make(map[string]struct{}, len(inbounds))
		for _, inbound := range inbounds {
			ownedTags[inbound.Tag] = struct{}{}
		}
		scoped := make(map[string][]string, len(active))
		for guid, tags := range active {
			kept := make([]string, 0, len(tags))
			for _, tag := range tags {
				if _, ok := ownedTags[tag]; ok {
					kept = append(kept, tag)
				}
			}
			if len(kept) > 0 {
				scoped[guid] = kept
			}
		}
		jsonObj(c, scoped, nil)
		return
	}
	jsonObj(c, active, nil)
}

func (a *ClientController) lastOnline(c *gin.Context) {
	data, err := a.inboundService.GetClientsLastOnline()
	if err != nil {
		jsonObj(c, data, err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		owned, err := newResellerService().OwnedEmailSet(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		scoped := make(map[string]int64, len(data))
		for email, ts := range data {
			if _, ok := owned[strings.ToLower(strings.TrimSpace(email))]; ok {
				scoped[email] = ts
			}
		}
		jsonObj(c, scoped, nil)
		return
	}
	jsonObj(c, data, nil)
}

func (a *ClientController) getTrafficByEmail(c *gin.Context) {
	email := c.Param("email")
	var traffic interface{}
	var err error
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		owned, ownedErr := a.resellerService.OwnedInboundIdSet(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.trafficGetError"), ownedErr)
			return
		}
		traffic, err = a.inboundService.GetClientTrafficByEmailForInbounds(email, owned)
	} else {
		traffic, err = a.inboundService.GetClientTrafficByEmail(email)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.trafficGetError"), err)
		return
	}
	jsonObj(c, traffic, nil)
}

func (a *ClientController) getSubLinks(c *gin.Context) {
	subId := c.Param("subId")
	if reseller := resellerSession(c); reseller != nil {
		email, err := a.resellerService.EmailBySubID(subId)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
			return
		}
		if email == "" {
			abortForbidden(c, errNotYourClient)
			return
		}
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		ownedInbounds, ownedErr := a.resellerService.OwnedInboundIdSet(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), ownedErr)
			return
		}
		links, linkErr := a.inboundService.GetAllClientLinksForInbounds(resolveHost(c), email, ownedInbounds)
		if linkErr != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), linkErr)
			return
		}
		jsonObj(c, links, nil)
		return
	}
	links, err := a.inboundService.GetSubLinks(resolveHost(c), subId)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	jsonObj(c, links, nil)
}

func (a *ClientController) getClientLinks(c *gin.Context) {
	email := c.Param("email")
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		ownedInbounds, err := a.resellerService.OwnedInboundIdSet(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
			return
		}
		links, err := a.inboundService.GetAllClientLinksForInbounds(resolveHost(c), email, ownedInbounds)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
			return
		}
		jsonObj(c, links, nil)
		return
	}
	links, err := a.inboundService.GetAllClientLinks(resolveHost(c), email)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	jsonObj(c, links, nil)
}

// getOpenvpnProfile returns the rendered .ovpn profile for a client's
// openvpn inbound. Resellers may only fetch profiles of their own clients,
// mirroring the other per-client read endpoints.
func (a *ClientController) getOpenvpnProfile(c *gin.Context) {
	email := c.Param("email")
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	var profile string
	var inbound *model.Inbound
	var err error
	if reseller := resellerSession(c); reseller != nil {
		ownedInbounds, ownedErr := a.resellerService.OwnedInboundIdSet(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), ownedErr)
			return
		}
		profile, inbound, err = a.inboundService.GetOpenvpnProfileForInbounds(resolveHost(c), email, ownedInbounds)
	} else {
		profile, inbound, err = a.inboundService.GetOpenvpnProfile(resolveHost(c), email)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	jsonObj(c, gin.H{"inboundId": inbound.Id, "inboundTag": inbound.Tag, "profile": profile}, nil)
}

func (a *ClientController) detach(c *gin.Context) {
	email := c.Param("email")
	var body attachDetachBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) || !ensureInboundsOwned(c, reseller, body.InboundIds) {
			return
		}
	}
	var needRestart bool
	var err error
	if reseller := resellerSession(c); reseller != nil {
		needRestart, err = a.clientService.DetachByEmailManyForReseller(&a.inboundService, reseller.Id, email, body.InboundIds)
	} else {
		needRestart, err = a.clientService.DetachByEmailMany(&a.inboundService, email, body.InboundIds)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientDeleteSuccess"), pendingNodeObj(a.inboundService.AnyNodePending(body.InboundIds)), nil)
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

type bulkResetRequest struct {
	Emails []string `json:"emails"`
}

func (a *ClientController) bulkResetTraffic(c *gin.Context) {
	var req bulkResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var needRestart bool
	if reseller := resellerSession(c); reseller != nil {
		if !ensureEmailsOwned(c, reseller, req.Emails) {
			return
		}
		scoped, scopedErr := a.resellerOwnedEmails(reseller, req.Emails)
		if scopedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), scopedErr)
			return
		}
		ownedInboundIDs, ownedErr := a.resellerService.OwnedInboundIds(reseller.Id)
		if ownedErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), ownedErr)
			return
		}
		affected, restart, resetErr := a.clientService.BulkResetTrafficForInbounds(&a.inboundService, scoped, ownedInboundIDs)
		if resetErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), resetErr)
			return
		}
		needRestart = restart
		jsonObj(c, gin.H{"affected": affected}, nil)
	} else {
		affected, resetErr := a.clientService.BulkResetTraffic(&a.inboundService, req.Emails)
		if resetErr != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), resetErr)
			return
		}
		jsonObj(c, gin.H{"affected": affected}, nil)
	}
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
	notifyClientsChanged()
}

// scopeResellerClientWrite validates a reseller's client-creation payloads:
// every target inbound must be owned by the reseller and the new clients must
// fit inside its client/traffic quota.
func (a *ClientController) scopeResellerClientWrite(reseller *model.Reseller, payloads []service.ClientCreatePayload) error {
	if err := a.resellerService.EnsureActive(reseller); err != nil {
		return err
	}
	var addBytes int64
	for _, payload := range payloads {
		if email := strings.TrimSpace(payload.Client.Email); email != "" {
			if _, err := a.clientService.GetRecordByEmail(nil, email); err == nil {
				owned, ownerErr := a.resellerService.OwnsClient(reseller.Id, email)
				if ownerErr != nil {
					return ownerErr
				}
				if !owned {
					return errors.New("client not found")
				}
				foreign, foreignErr := a.resellerService.HasForeignInboundAssociation(reseller.Id, email)
				if foreignErr != nil {
					return foreignErr
				}
				if foreign {
					return errors.New("client not found")
				}
			} else if !database.IsNotFound(err) {
				return err
			}
		}
		for _, inboundId := range payload.InboundIds {
			owned, err := a.resellerService.OwnsInbound(reseller.Id, inboundId)
			if err != nil {
				return err
			}
			if !owned {
				return errors.New("inbound not found")
			}
		}
		addBytes += payload.Client.TotalGB
	}
	return a.resellerService.CheckClientQuota(reseller, len(payloads), addBytes)
}

// checkResellerClientQuotaDelta charges only the growth of a single client's
// quota, so lowering it is always allowed.
func (a *ClientController) checkResellerClientQuotaDelta(reseller *model.Reseller, email string, newTotalBytes int64) error {
	current, err := a.clientService.GetRecordByEmail(nil, email)
	if err != nil || current == nil {
		return err
	}
	if newTotalBytes <= current.TotalGB {
		return nil
	}
	return a.resellerService.CheckTrafficQuota(reseller, newTotalBytes-current.TotalGB)
}

// delDepletedForReseller deletes only the reseller's own depleted clients.
func (a *ClientController) delDepletedForReseller(reseller *model.Reseller) (int, bool, error) {
	owned, err := newResellerService().OwnedIsolatedAssociatedEmailSet(reseller.Id)
	if err != nil {
		return 0, false, err
	}
	rows, err := a.clientService.List()
	if err != nil {
		return 0, false, err
	}
	depleted := make([]string, 0)
	for _, row := range rows {
		if _, ok := owned[strings.ToLower(strings.TrimSpace(row.Email))]; !ok {
			continue
		}
		if row.TotalGB <= 0 || row.Traffic == nil {
			continue
		}
		if row.Traffic.Up+row.Traffic.Down >= row.TotalGB {
			depleted = append(depleted, row.Email)
		}
	}
	if len(depleted) == 0 {
		return 0, false, nil
	}
	ownedInboundIDs, err := a.resellerService.OwnedInboundIds(reseller.Id)
	if err != nil {
		return 0, false, err
	}
	result, needRestart, err := a.clientService.BulkDeleteForReseller(&a.inboundService, reseller.Id, depleted, false, ownedInboundIDs)
	if err != nil {
		return 0, false, err
	}
	if len(result.DeletedEmails) > 0 {
		if err := a.resellerService.UnassignClients(reseller.Id, result.DeletedEmails); err != nil {
			return 0, needRestart, err
		}
	}
	return result.Deleted, needRestart, nil
}
