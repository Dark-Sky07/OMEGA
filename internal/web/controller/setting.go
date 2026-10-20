package controller

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/logger"
	"github.com/Dark-Sky07/phoenix/internal/util/crypto"
	"github.com/Dark-Sky07/phoenix/internal/web/entity"
	"github.com/Dark-Sky07/phoenix/internal/web/middleware"
	"github.com/Dark-Sky07/phoenix/internal/web/service"
	"github.com/Dark-Sky07/phoenix/internal/web/service/panel"
	"github.com/Dark-Sky07/phoenix/internal/web/session"

	"github.com/gin-gonic/gin"
)

// updateUserForm represents the form for updating user credentials.
type updateUserForm struct {
	OldUsername string `json:"oldUsername" form:"oldUsername"`
	OldPassword string `json:"oldPassword" form:"oldPassword"`
	NewUsername string `json:"newUsername" form:"newUsername"`
	NewPassword string `json:"newPassword" form:"newPassword"`
}

// SettingController handles settings and user management operations.
type SettingController struct {
	settingService     service.SettingService
	userService        panel.UserService
	panelService       panel.PanelService
	apiTokenService    panel.ApiTokenService
	xrayService        service.XrayService
	certService        service.CertService
	discordService     service.DiscordService
	cloudBackupService service.CloudBackupService
	auditLogService    service.AuditLogService
}

// NewSettingController creates a new SettingController and initializes its routes.
func NewSettingController(g *gin.RouterGroup) *SettingController {
	a := &SettingController{}
	a.initRouter(g)
	return a
}

// initRouter sets up the routes for settings management.
func (a *SettingController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/setting")

	g.POST("/all", a.getAllSetting)
	g.POST("/defaultSettings", a.getDefaultSettings)
	g.POST("/update", a.updateSetting)
	g.POST("/updateUser", a.updateUser)
	g.POST("/restartPanel", a.restartPanel)
	g.GET("/getDefaultJsonConfig", a.getDefaultXrayConfig)
	g.GET("/apiTokens", a.listApiTokens)
	g.POST("/apiTokens/create", a.createApiToken)
	g.POST("/apiTokens/delete/:id", a.deleteApiToken)
	g.POST("/apiTokens/setEnabled/:id", a.setApiTokenEnabled)
	g.GET("/cert/status", a.getCertStatus)
	g.POST("/cert/issue", a.issueCert)
	g.POST("/discord/test", a.testDiscord)
	g.POST("/cloudBackup/run", a.runCloudBackup)
	g.GET("/auditLogs", a.listAuditLogs)
	g.POST("/auditLogs/clear", a.clearAuditLogs)
	g.GET("/admins", a.listAdmins)
	g.POST("/admins/create", a.createAdmin)
	g.POST("/admins/update/:id", a.updateAdmin)
	g.POST("/admins/delete/:id", a.deleteAdmin)
}

// getAllSetting retrieves all current settings.
func (a *SettingController) getAllSetting(c *gin.Context) {
	allSetting, err := a.settingService.GetAllSetting()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, allSetting, nil)
}

// getDefaultSettings retrieves the default settings based on the host.
func (a *SettingController) getDefaultSettings(c *gin.Context) {
	result, err := a.settingService.GetDefaultSettings(c.Request.Host)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, result, nil)
}

// updateSetting updates all settings with the provided data.
func (a *SettingController) updateSetting(c *gin.Context) {
	allSetting, ok := middleware.BindAndValidate[entity.AllSetting](c)
	if !ok {
		return
	}
	oldTwoFactor, twoFactorErr := a.settingService.GetTwoFactorEnable()
	oldPanelOutbound, _ := a.settingService.GetPanelOutbound()
	err := a.settingService.UpdateAllSetting(allSetting)
	if err == nil && twoFactorErr == nil && !oldTwoFactor && allSetting.TwoFactorEnable {
		if bumpErr := a.userService.BumpLoginEpoch(); bumpErr != nil {
			err = bumpErr
		}
	}
	if err == nil && allSetting.PanelOutbound != oldPanelOutbound {
		// The egress bridge lives in the generated config; reconcile the
		// running core. One SOCKS inbound plus one routing rule — both
		// hot-appliable, so this normally does not restart Xray.
		if applyErr := a.xrayService.RestartXray(false); applyErr != nil {
			logger.Warning("apply panel outbound change failed:", applyErr)
		}
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
}

// updateUser updates the current user's username and password.
func (a *SettingController) updateUser(c *gin.Context) {
	form := &updateUserForm{}
	err := c.ShouldBind(form)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	user := session.GetLoginUser(c)
	if user.Username != form.OldUsername || !crypto.CheckPasswordHash(user.Password, form.OldPassword) {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifyUserError"), errors.New(I18nWeb(c, "pages.settings.toasts.originalUserPassIncorrect")))
		return
	}
	if form.NewUsername == "" || form.NewPassword == "" {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifyUserError"), errors.New(I18nWeb(c, "pages.settings.toasts.userPassMustBeNotEmpty")))
		return
	}
	err = a.userService.UpdateUser(user.Id, form.NewUsername, form.NewPassword)
	if err == nil {
		user.Username = form.NewUsername
		user.Password, _ = crypto.HashPasswordAsBcrypt(form.NewPassword)
		if saveErr := session.SetLoginUser(c, user); saveErr != nil {
			err = saveErr
		}
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifyUser"), err)
}

// restartPanel restarts the panel service after a delay.
func (a *SettingController) restartPanel(c *gin.Context) {
	err := a.panelService.RestartPanel(time.Second * 3)
	jsonMsg(c, I18nWeb(c, "pages.settings.restartPanelSuccess"), err)
}

// getDefaultXrayConfig retrieves the default Xray configuration.
func (a *SettingController) getDefaultXrayConfig(c *gin.Context) {
	defaultJsonConfig, err := a.settingService.GetDefaultXrayConfig()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, defaultJsonConfig, nil)
}

type apiTokenCreateForm struct {
	Name       string `json:"name" form:"name"`
	Scope      string `json:"scope" form:"scope"`
	ExpireDays int    `json:"expireDays" form:"expireDays" validate:"gte=0"`
}

type apiTokenEnabledForm struct {
	Enabled bool `json:"enabled" form:"enabled"`
}

func (a *SettingController) listApiTokens(c *gin.Context) {
	rows, err := a.apiTokenService.List()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *SettingController) createApiToken(c *gin.Context) {
	form := &apiTokenCreateForm{}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	row, err := a.apiTokenService.CreateWithOptions(form.Name, form.Scope, form.ExpireDays)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	jsonObj(c, row, nil)
}

func (a *SettingController) deleteApiToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), a.apiTokenService.Delete(id))
}

func (a *SettingController) setApiTokenEnabled(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	form := &apiTokenEnabledForm{}
	if bindErr := c.ShouldBind(form); bindErr != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), bindErr)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), a.apiTokenService.SetEnabled(id, form.Enabled))
}

type certIssueForm struct {
	Domain       string `json:"domain" form:"domain"`
	Email        string `json:"email" form:"email"`
	Mode         string `json:"mode" form:"mode"` // "letsencrypt" (default) or "selfsigned"
	ApplyToPanel bool   `json:"applyToPanel" form:"applyToPanel"`
}

func (a *SettingController) getCertStatus(c *gin.Context) {
	jsonObj(c, a.certService.CurrentStatus(), nil)
}

func (a *SettingController) issueCert(c *gin.Context) {
	form := &certIssueForm{ApplyToPanel: true}
	if err := c.ShouldBind(form); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	var (
		st  *service.CertStatus
		err error
	)
	if strings.EqualFold(strings.TrimSpace(form.Mode), "selfsigned") {
		st, err = a.certService.IssueSelfSigned(form.Domain, "", form.ApplyToPanel)
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()
		st, err = a.certService.IssueLetsEncrypt(ctx, form.Domain, form.Email, "", form.ApplyToPanel)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	jsonObj(c, st, nil)
}

func (a *SettingController) testDiscord(c *gin.Context) {
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), a.discordService.SendTestMessage())
}

func (a *SettingController) runCloudBackup(c *gin.Context) {
	key, err := a.cloudBackupService.BackupNow()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	jsonObj(c, gin.H{"key": key}, nil)
}

func (a *SettingController) listAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	actorType := c.Query("actorType")
	module := c.Query("module")
	search := c.Query("search")
	logs, err := a.auditLogService.List(limit, actorType, module, search)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, logs, nil)
}

func (a *SettingController) clearAuditLogs(c *gin.Context) {
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), a.auditLogService.Clear())
}

type adminForm struct {
	Username    string   `json:"username" form:"username"`
	Password    string   `json:"password" form:"password"`
	Role        string   `json:"role" form:"role"`
	Permissions []string `json:"permissions" form:"permissions"`
	Note        string   `json:"note" form:"note"`
	Disabled    bool     `json:"disabled" form:"disabled"`
}

func (a *SettingController) ensureSuperAdmin(c *gin.Context) bool {
	if c.GetBool("api_authed") {
		return true
	}
	user := session.GetLoginUser(c)
	if !panel.IsSuperAdmin(user) {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), errors.New("superadmin permission required"))
		return false
	}
	return true
}

func (a *SettingController) listAdmins(c *gin.Context) {
	admins, err := a.userService.ListAdmins()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, admins, nil)
}

func (a *SettingController) createAdmin(c *gin.Context) {
	if !a.ensureSuperAdmin(c) {
		return
	}
	var form adminForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	created, err := a.userService.CreateAdmin(form.Username, form.Password, form.Role, form.Permissions, form.Note)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	jsonObj(c, created, nil)
}

func (a *SettingController) updateAdmin(c *gin.Context) {
	if !a.ensureSuperAdmin(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), errors.New("invalid admin id"))
		return
	}
	var form adminForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	updated, err := a.userService.UpdateAdmin(id, form.Username, form.Password, form.Role, form.Permissions, form.Note, form.Disabled)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	jsonObj(c, updated, nil)
}

func (a *SettingController) deleteAdmin(c *gin.Context) {
	if !a.ensureSuperAdmin(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), errors.New("invalid admin id"))
		return
	}
	callerId := 0
	if u := session.GetLoginUser(c); u != nil {
		callerId = u.Id
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), a.userService.DeleteAdmin(id, callerId))
}
