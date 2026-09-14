package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

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
	g.POST("/assignClient", a.assignClient)
	g.POST("/unassignClient", a.unassignClient)
	g.POST("/balance", a.balance)
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
// so the admin UI can show who owns what without N+1 requests.
func (a *ResellerController) assignments(c *gin.Context) {
	type ownership struct {
		ResellerId int      `json:"resellerId"`
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
		out = append(out, ownership{ResellerId: reseller.Id, InboundIds: inboundIds, Emails: emails})
	}
	jsonObj(c, out, nil)
}

func (a *ResellerController) add(c *gin.Context) {
	var form resellerForm
	if err := c.ShouldBindJSON(&form); err != nil {
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
	if err := c.ShouldBindJSON(&form); err != nil {
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
