package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/web/service"
	"github.com/Dark-Sky07/phoenix/internal/web/session"

	"github.com/gin-gonic/gin"
)

// resellerForm is the create/update payload for a reseller. The password is
// accepted on the wire but never serialised back (model.Reseller hides it).
type resellerForm struct {
	model.Reseller
	Password string `json:"password" form:"password"`
}

// ResellerController serves the admin-side reseller management API
// (/panel/api/resellers/*).
type ResellerController struct {
	BaseController
	resellerService service.ResellerService
	ticketService   service.TicketService
	webhookService  service.PaymentWebhookService
	voucherService  service.VoucherService
}

// NewResellerController wires the admin reseller routes.
func NewResellerController(g *gin.RouterGroup) *ResellerController {
	a := &ResellerController{}
	a.initRouter(g)
	return a
}

func (a *ResellerController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.GET("/get/:id", a.get)
	g.GET("/assignments", a.assignments)
	g.GET("/report/:id", a.report)
	g.POST("/add", a.add)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.del)
	g.POST("/setEnable/:id", a.setEnable)
	g.POST("/resetPassword/:id", a.resetPassword)
	g.POST("/assignInbound", a.assignInbound)
	g.POST("/unassignInbound", a.unassignInbound)
	g.POST("/assignInbounds", a.assignInbounds)
	g.POST("/unassignInbounds", a.unassignInbounds)
	g.POST("/setInbounds", a.setInbounds)
	g.POST("/assignClient", a.assignClient)
	g.POST("/unassignClient", a.unassignClient)
	g.POST("/assignClients", a.assignClients)
	g.POST("/unassignClients", a.unassignClients)
	g.POST("/balance", a.balance)
	g.GET("/tickets", a.listTickets)
	g.GET("/tickets/:id", a.getTicket)
	g.POST("/tickets/:id/reply", a.replyTicket)
	g.POST("/tickets/:id/close", a.closeTicket)
	g.GET("/orders", a.listOrders)
	g.POST("/orders/process", a.processOrder)
	g.GET("/vouchers", a.listVouchers)
	g.POST("/vouchers/create", a.createVoucher)
	g.POST("/vouchers/delete/:id", a.deleteVoucher)
}

func (a *ResellerController) list(c *gin.Context) {
	rows, err := a.resellerService.List()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ResellerController) get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	stat, err := a.resellerService.StatFor(id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, stat, nil)
}

func (a *ResellerController) report(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	report, err := a.resellerService.Report(id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, report, nil)
}

// assignments returns the ownership map (inbound -> reseller, client -> reseller)
// so the admin UI can show who owns what without N+1 requests. Names are
// included so consumers (e.g. the clients page owner tags) can label the
// owner without a second /resellers/list round trip.
func (a *ResellerController) assignments(c *gin.Context) {
	type ownership struct {
		ResellerId int      `json:"resellerId"`
		Name       string   `json:"name"`
		Username   string   `json:"username"`
		InboundIds []int    `json:"inboundIds"`
		Emails     []string `json:"emails"`
	}
	db := database.GetDB()
	resellers := make([]*model.Reseller, 0)
	if err := db.Model(model.Reseller{}).Order("id ASC").Find(&resellers).Error; err != nil {
		jsonMsg(c, "", err)
		return
	}
	out := make([]ownership, 0, len(resellers))
	for _, reseller := range resellers {
		inboundIds, err := a.resellerService.OwnedInboundIds(reseller.Id)
		if err != nil {
			jsonMsg(c, "", err)
			return
		}
		emails, err := a.resellerService.ExplicitAssignedEmails(reseller.Id)
		if err != nil {
			jsonMsg(c, "", err)
			return
		}
		if inboundIds == nil {
			inboundIds = []int{}
		}
		if emails == nil {
			emails = []string{}
		}
		out = append(out, ownership{
			ResellerId: reseller.Id,
			Name:       reseller.Name,
			Username:   reseller.Username,
			InboundIds: inboundIds,
			Emails:     emails,
		})
	}
	jsonObj(c, out, nil)
}

func (a *ResellerController) add(c *gin.Context) {
	var form resellerForm
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	reseller := form.Reseller
	if err := a.resellerService.Add(&reseller, form.Password); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, reseller, nil)
}

func (a *ResellerController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	var form resellerForm
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	reseller := form.Reseller
	reseller.Id = id
	if err := a.resellerService.Update(&reseller, form.Password); err != nil {
		jsonMsg(c, "", err)
		return
	}
	stat, err := a.resellerService.StatFor(id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, stat, nil)
}

func (a *ResellerController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.Delete(id); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) setEnable(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	var form struct {
		Enable bool `json:"enable" form:"enable"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if _, err := a.resellerService.SetEnable(id, form.Enable); err != nil {
		jsonMsg(c, "", err)
		return
	}
	stat, err := a.resellerService.StatFor(id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, stat, nil)
}

func (a *ResellerController) resetPassword(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	var form struct {
		Password string `json:"password" form:"password"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.ChangePassword(id, form.Password); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) assignInbound(c *gin.Context) {
	var form struct {
		ResellerId int `json:"resellerId" form:"resellerId"`
		InboundId  int `json:"inboundId" form:"inboundId"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.AssignInbound(form.ResellerId, form.InboundId); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) unassignInbound(c *gin.Context) {
	var form struct {
		ResellerId int `json:"resellerId" form:"resellerId"`
		InboundId  int `json:"inboundId" form:"inboundId"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.UnassignInbound(form.ResellerId, form.InboundId); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

// assignInbounds and unassignInbounds are the atomic multi-select endpoints
// used by the admin UI. They deliberately accept the complete desired batch so
// a partial loop of HTTP requests can never leave half an assignment applied.
func (a *ResellerController) assignInbounds(c *gin.Context) {
	var form struct {
		ResellerId int   `json:"resellerId" form:"resellerId"`
		InboundIds []int `json:"inboundIds" form:"inboundIds"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.AssignInbounds(form.ResellerId, form.InboundIds); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) unassignInbounds(c *gin.Context) {
	var form struct {
		ResellerId int   `json:"resellerId" form:"resellerId"`
		InboundIds []int `json:"inboundIds" form:"inboundIds"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.UnassignInbounds(form.ResellerId, form.InboundIds); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) setInbounds(c *gin.Context) {
	var form struct {
		ResellerId int   `json:"resellerId" form:"resellerId"`
		InboundIds []int `json:"inboundIds" form:"inboundIds"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.SetInbounds(form.ResellerId, form.InboundIds); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) assignClient(c *gin.Context) {
	var form struct {
		ResellerId int    `json:"resellerId" form:"resellerId"`
		Email      string `json:"email" form:"email"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.AssignClient(form.ResellerId, form.Email); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) unassignClient(c *gin.Context) {
	var form struct {
		ResellerId int    `json:"resellerId" form:"resellerId"`
		Email      string `json:"email" form:"email"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.UnassignClient(form.ResellerId, form.Email); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) assignClients(c *gin.Context) {
	var form struct {
		ResellerId int      `json:"resellerId" form:"resellerId"`
		Emails     []string `json:"emails" form:"emails"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.AssignClients(form.ResellerId, form.Emails); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) unassignClients(c *gin.Context) {
	var form struct {
		ResellerId int      `json:"resellerId" form:"resellerId"`
		Emails     []string `json:"emails" form:"emails"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.resellerService.UnassignClients(form.ResellerId, form.Emails); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, nil, nil)
}

func (a *ResellerController) balance(c *gin.Context) {
	var form struct {
		ResellerId int     `json:"resellerId" form:"resellerId"`
		Amount     float64 `json:"amount" form:"amount"`
		Comment    string  `json:"comment" form:"comment"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	reseller, err := a.resellerService.AdjustBalance(form.ResellerId, form.Amount, form.Comment)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	stat, err := a.resellerService.Stat(reseller)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, stat, nil)
}

func (a *ResellerController) listTickets(c *gin.Context) {
	rows, err := a.ticketService.ListForAdmin(c.Query("status"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ResellerController) getTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "", errors.New("invalid ticket id"))
		return
	}
	t, msgs, err := a.ticketService.GetTicketWithMessages(id, 0)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{
		"ticket":   t,
		"messages": msgs,
	}, nil)
}

func (a *ResellerController) replyTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "", errors.New("invalid ticket id"))
		return
	}
	var form struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	sender := "admin"
	if u := session.GetLoginUser(c); u != nil && u.Username != "" {
		sender = u.Username
	}
	msg, err := a.ticketService.ReplyTicket(id, "admin", sender, form.Message, 0)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, msg, nil)
}

func (a *ResellerController) closeTicket(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "", errors.New("invalid ticket id"))
		return
	}
	jsonMsg(c, "", a.ticketService.CloseTicket(id, 0))
}

func (a *ResellerController) listOrders(c *gin.Context) {
	rows, err := a.webhookService.ListOrders(200)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ResellerController) processOrder(c *gin.Context) {
	var payload service.PaymentWebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonMsg(c, "", err)
		return
	}
	order, err := a.webhookService.ProcessWebhook(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, order, nil)
}

func (a *ResellerController) listVouchers(c *gin.Context) {
	rows, err := a.voucherService.List()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ResellerController) createVoucher(c *gin.Context) {
	var form struct {
		Code      string  `json:"code"`
		Amount    float64 `json:"amount"`
		TrafficGB int64   `json:"trafficGB"`
		MaxUses   int     `json:"maxUses"`
		ExpiresAt int64   `json:"expiresAt"`
		Note      string  `json:"note"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	v, err := a.voucherService.Create(form.Code, form.Amount, form.TrafficGB, form.MaxUses, form.ExpiresAt, form.Note)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, v, nil)
}

func (a *ResellerController) deleteVoucher(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "", errors.New("invalid voucher id"))
		return
	}
	jsonMsg(c, "", a.voucherService.Delete(id))
}

// HandlePublicPaymentWebhook processes HMAC-SHA256 signed external payment
// gateway callbacks (POST <basePath>/api/payment/webhook).
func HandlePublicPaymentWebhook(c *gin.Context) {
	var settingSvc service.SettingService
	secretBytes, err := settingSvc.GetSecret()
	secret := string(secretBytes)
	if err != nil || secret == "" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "msg": "webhook secret not available"})
		return
	}
	rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": "invalid body"})
		return
	}
	sig := c.GetHeader("X-Phoenix-Signature")
	if !service.VerifyPaymentWebhookHMAC(rawBody, sig, secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "invalid webhook HMAC signature"})
		return
	}
	var payload service.PaymentWebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": "invalid JSON payload"})
		return
	}
	var svc service.PaymentWebhookService
	order, err := svc.ProcessWebhook(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": order})
}
