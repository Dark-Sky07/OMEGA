package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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

func (a *ClientController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.GET("/list/paged", a.listPaged)
	g.GET("/get/:email", a.get)
	g.GET("/traffic/:email", a.getTrafficByEmail)
	g.GET("/subLinks/:subId", a.getSubLinks)
	g.GET("/links/:email", a.getClientLinks)

	g.POST("/add", a.create)
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
		filtered := make([]service.ClientWithAttachments, 0, len(rows))
		for _, row := range rows {
			if _, ok := owned[row.Email]; ok {
				filtered = append(filtered, row)
			}
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
		params.ScopeEmails = scope
	}
	resp, err := a.clientService.ListPaged(&a.inboundService, &a.settingService, params)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	jsonObj(c, resp, nil)
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
	flow, err := a.clientService.EffectiveFlow(nil, rec.Id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	rec.Flow = flow
	// Consumed bytes (up+down, including cross-node global overlay) so API
	// consumers can pair usage with the client's totalGB quota (#4973).
	// Best-effort: a traffic lookup failure must not break the client fetch.
	var usedTraffic int64
	if t, tErr := a.inboundService.GetClientTrafficByEmail(email); tErr == nil && t != nil {
		usedTraffic = t.Up + t.Down
	}
	jsonObj(c, gin.H{"client": rec, "inboundIds": inboundIds, "usedTraffic": usedTraffic}, nil)
}

func (a *ClientController) create(c *gin.Context) {
	var payload service.ClientCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		if err := a.scopeResellerClientWrite(reseller, []service.ClientCreatePayload{payload}); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	needRestart, err := a.clientService.Create(&a.inboundService, &payload)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
		if err := a.resellerService.EnsureActive(reseller); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		if len(inboundFilter) > 0 && !ensureInboundsOwned(c, reseller, inboundFilter) {
			return
		}
		if err := a.checkResellerClientQuotaDelta(reseller, email, updated.TotalGB); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	needRestart, err := a.clientService.UpdateByEmail(&a.inboundService, email, updated, inboundFilter...)
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	keepTraffic := c.Query("keepTraffic") == "1"
	needRestart, err := a.clientService.DeleteByEmail(&a.inboundService, email, keepTraffic)
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
	}
	needRestart, err := a.clientService.AttachByEmail(&a.inboundService, email, body.InboundIds)
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
		owned, err := newResellerService().OwnedEmails(reseller.Id)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		if len(owned) == 0 {
			jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.resetAllClientTrafficSuccess"), nil)
			return
		}
		if _, err := a.clientService.BulkResetTraffic(&a.inboundService, owned); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		a.xrayService.SetToNeedRestart()
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
		if err := a.resellerService.CheckTrafficQuota(reseller, int64(len(req.Emails))*req.AddBytes); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	result, needRestart, err := a.clientService.BulkAdjust(&a.inboundService, req.Emails, req.AddDays, req.AddBytes)
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
	}
	result, needRestart, err := a.clientService.BulkAttach(&a.inboundService, req.Emails, req.InboundIds)
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
	result, needRestart, err := a.clientService.BulkDetach(&a.inboundService, req.Emails, req.InboundIds)
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureEmailsOwned(c, reseller, req.Emails) {
			return
		}
	}
	result, needRestart, err := a.clientService.BulkDelete(&a.inboundService, req.Emails, req.KeepTraffic)
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
	if reseller := resellerSession(c); reseller != nil {
		if err := a.scopeResellerClientWrite(reseller, payloads); err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
	}
	result, needRestart, err := a.clientService.BulkCreate(&a.inboundService, payloads)
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	needRestart, err := a.clientService.ResetTrafficByEmail(&a.inboundService, email)
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	if err := a.inboundService.UpdateClientTrafficByEmail(email, req.Upload, req.Download); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.inboundClientUpdateSuccess"), nil)
	notifyClientsChanged()
}

func (a *ClientController) getIps(c *gin.Context) {
	email := c.Param("email")
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	ips, err := a.inboundService.GetInboundClientIps(email)
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	if err := a.inboundService.ClearClientIps(email); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.updateSuccess"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.logCleanSuccess"), nil)
}

func (a *ClientController) onlines(c *gin.Context) {
	online := a.inboundService.GetOnlineClients()
	if reseller := resellerSession(c); reseller != nil {
		owned, err := resellerEmailSet(reseller)
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
		owned, err := resellerEmailSet(reseller)
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
		owned, err := resellerEmailSet(reseller)
		if err != nil {
			jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
			return
		}
		scoped := make(map[string]int64, len(data))
		for email, ts := range data {
			if _, ok := owned[email]; ok {
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	traffic, err := a.inboundService.GetClientTrafficByEmail(email)
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
		if err != nil || email == "" || !ensureClientOwned(c, reseller, email) {
			return
		}
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
	}
	links, err := a.inboundService.GetAllClientLinks(resolveHost(c), email)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.inbounds.toasts.obtain"), err)
		return
	}
	jsonObj(c, links, nil)
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
	needRestart, err := a.clientService.DetachByEmailMany(&a.inboundService, email, body.InboundIds)
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
	if reseller := resellerSession(c); reseller != nil {
		if !ensureEmailsOwned(c, reseller, req.Emails) {
			return
		}
	}
	affected, err := a.clientService.BulkResetTraffic(&a.inboundService, req.Emails)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"affected": affected}, nil)
	a.xrayService.SetToNeedRestart()
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
	owned, err := resellerEmailSet(reseller)
	if err != nil {
		return 0, false, err
	}
	rows, err := a.clientService.List()
	if err != nil {
		return 0, false, err
	}
	depleted := make([]string, 0)
	for _, row := range rows {
		if _, ok := owned[row.Email]; !ok {
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
	result, needRestart, err := a.clientService.BulkDelete(&a.inboundService, depleted, false)
	if err != nil {
		return 0, false, err
	}
	return result.Deleted, needRestart, nil
}
