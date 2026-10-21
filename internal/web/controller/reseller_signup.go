package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/web/service"
	"github.com/gin-gonic/gin"
)

// HandlePublicResellerSignupInfo returns only the public plan details attached
// to a signed reseller registration link.
func HandlePublicResellerSignupInfo(c *gin.Context) {
	secret, ok := resellerSignupSecret(c)
	if !ok {
		return
	}
	info, err := (&service.ResellerSignupService{}).Info(c.Query("ref"), secret, time.Now())
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "msg": "registration link is invalid or unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": info})
}

// HandlePublicResellerSignup creates a customer from a plan fixed into the
// reseller's signed public registration link.
func HandlePublicResellerSignup(c *gin.Context) {
	secret, ok := resellerSignupSecret(c)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": "invalid request body"})
		return
	}
	subID, needRestart, err := (&service.ResellerSignupService{}).Register(c.Query("ref"), req.Email, secret, time.Now())
	if needRestart {
		(&service.XrayService{}).SetToNeedRestart()
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrResellerSignupLink) || errors.Is(err, service.ErrResellerSignupPlan) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": gin.H{"registered": true, "subId": subID}})
	notifyClientsChanged()
}

func resellerSignupSecret(c *gin.Context) ([]byte, bool) {
	var settingSvc service.SettingService
	secret, err := settingSvc.GetSecret()
	if err != nil || len(secret) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "msg": "registration is unavailable"})
		return nil, false
	}
	return secret, true
}

func (a *ResellerSelfController) signupTemplates(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	rows, err := (&service.ResellerSignupService{}).EligibleTemplates(reseller.Id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ResellerSelfController) createSignupLink(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	var req struct {
		TemplateID int `json:"templateId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.TemplateID <= 0 {
		jsonMsg(c, "", errors.New("a valid templateId is required"))
		return
	}
	var settingSvc service.SettingService
	secret, err := settingSvc.GetSecret()
	if err != nil || len(secret) == 0 {
		jsonMsg(c, "", errors.New("registration link signing is unavailable"))
		return
	}
	token, err := (&service.ResellerSignupService{}).CreateLink(reseller.Id, req.TemplateID, secret, time.Now())
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"token": token, "expiresAt": time.Now().Add(30 * 24 * time.Hour).UnixMilli()}, nil)
}
