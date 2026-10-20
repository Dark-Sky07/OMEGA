package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/web/service"
	"github.com/Dark-Sky07/phoenix/internal/web/session"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Reseller self-service API (/panel/api/reseller/*)
// ---------------------------------------------------------------------------

// ResellerSelfController serves the endpoints a logged-in reseller calls for
// its own profile, usage and report.
type ResellerSelfController struct {
	BaseController
	resellerService service.ResellerService
	ticketService   service.TicketService
	voucherService  service.VoucherService
}

// NewResellerSelfController wires the reseller self-service routes.
func NewResellerSelfController(g *gin.RouterGroup) *ResellerSelfController {
	a := &ResellerSelfController{}
	a.initRouter(g)
	return a
}

func (a *ResellerSelfController) initRouter(g *gin.RouterGroup) {
	g.GET("/profile", a.profile)
	g.GET("/stats", a.stats)
	g.GET("/report", a.report)
	g.POST("/password", a.changePassword)
	g.GET("/tickets", a.listTickets)
	g.GET("/tickets/:id", a.getTicket)
	g.POST("/tickets/create", a.createTicket)
	g.POST("/tickets/:id/reply", a.replyTicket)
	g.POST("/tickets/:id/close", a.closeTicket)
	g.POST("/vouchers/redeem", a.redeemVoucher)
}

// currentReseller returns the reseller bound to the request, or nil for admin
// sessions.
func (a *ResellerSelfController) currentReseller(c *gin.Context) *model.Reseller {
	reseller := session.GetLoginReseller(c)
	if reseller == nil {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"msg":     "reseller account required",
		})
		return nil
	}
	if err := (&service.ResellerService{}).EnsureActive(reseller); err != nil {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"msg":     err.Error(),
		})
		return nil
	}
	return reseller
}

func (a *ResellerSelfController) profile(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	stat, err := a.resellerService.Stat(reseller)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, stat, nil)
}

func (a *ResellerSelfController) stats(c *gin.Context) {
	a.profile(c)
}

func (a *ResellerSelfController) report(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	report, err := a.resellerService.Report(reseller.Id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, report, nil)
}

func (a *ResellerSelfController) changePassword(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	var form struct {
		OldPassword string `json:"oldPassword" form:"oldPassword"`
		NewPassword string `json:"newPassword" form:"newPassword"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if _, err := a.resellerService.CheckReseller(reseller.Username, form.OldPassword); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	if strings.TrimSpace(form.NewPassword) == "" {
		jsonMsg(c, "", errors.New("new password can not be empty"))
		return
	}
	if err := a.resellerService.ChangePassword(reseller.Id, form.NewPassword); err != nil {
		jsonMsg(c, "", err)
		return
	}
	// The password change bumps the login epoch, so this session is stale now.
	_ = session.ClearSession(c)
	jsonObj(c, nil, nil)
}

func (a *ResellerSelfController) listTickets(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	rows, err := a.ticketService.ListForReseller(reseller.Id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ResellerSelfController) getTicket(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "", errors.New("invalid ticket id"))
		return
	}
	t, msgs, err := a.ticketService.GetTicketWithMessages(id, reseller.Id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{
		"ticket":   t,
		"messages": msgs,
	}, nil)
}

func (a *ResellerSelfController) createTicket(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	var form struct {
		Subject  string `json:"subject"`
		Priority string `json:"priority"`
		Message  string `json:"message"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	created, err := a.ticketService.CreateTicket(reseller, form.Subject, form.Priority, form.Message)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, created, nil)
}

func (a *ResellerSelfController) replyTicket(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
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
	msg, err := a.ticketService.ReplyTicket(id, "reseller", reseller.Username, form.Message, reseller.Id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, msg, nil)
}

func (a *ResellerSelfController) closeTicket(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, "", errors.New("invalid ticket id"))
		return
	}
	jsonMsg(c, "", a.ticketService.CloseTicket(id, reseller.Id))
}

func (a *ResellerSelfController) redeemVoucher(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	var form struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "", err)
		return
	}
	updatedReseller, voucher, err := a.voucherService.Redeem(reseller.Id, form.Code)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{
		"reseller": updatedReseller,
		"voucher":  voucher,
	}, nil)
}
