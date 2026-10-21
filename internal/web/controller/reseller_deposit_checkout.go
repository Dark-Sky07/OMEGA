package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/web/service"
	"github.com/gin-gonic/gin"
)

func HandlePublicResellerDepositInfo(c *gin.Context) {
	secret, ok := resellerSignupSecret(c)
	if !ok {
		return
	}
	info, err := (&service.ResellerDepositCheckoutService{}).Info(c.Query("ref"), secret, time.Now())
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "msg": "deposit checkout is unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": info})
}

func HandlePublicResellerDepositOrder(c *gin.Context) {
	secret, ok := resellerSignupSecret(c)
	if !ok {
		return
	}
	var req struct {
		Amount float64 `json:"amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": "invalid request body"})
		return
	}
	order, err := (&service.ResellerDepositCheckoutService{}).CreateOrder(c.Query("ref"), req.Amount, secret, time.Now())
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrResellerDepositLink) || errors.Is(err, service.ErrCheckoutDisabled) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": order})
}

func HandlePublicResellerDepositStatus(c *gin.Context) {
	secret, ok := resellerSignupSecret(c)
	if !ok {
		return
	}
	status, err := (&service.ResellerDepositCheckoutService{}).Status(c.Query("ref"), c.Query("orderId"), secret, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": status})
}

func (a *ResellerSelfController) createDepositLink(c *gin.Context) {
	reseller := a.currentReseller(c)
	if reseller == nil {
		return
	}
	var settingSvc service.SettingService
	enabled, err := settingSvc.GetCheckoutEnable()
	if err != nil || !enabled {
		jsonMsg(c, "", errors.New("public checkout is disabled"))
		return
	}
	secret, err := settingSvc.GetSecret()
	if err != nil || len(secret) == 0 {
		jsonMsg(c, "", errors.New("deposit link signing is unavailable"))
		return
	}
	token, err := (&service.ResellerDepositCheckoutService{}).CreateLink(reseller.Id, secret, time.Now())
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"token": token, "expiresAt": time.Now().Add(365 * 24 * time.Hour).UnixMilli()}, nil)
}
