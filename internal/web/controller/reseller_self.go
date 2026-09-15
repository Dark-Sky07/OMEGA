package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

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
