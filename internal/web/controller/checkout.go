package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Dark-Sky07/phoenix/internal/web/service"
)

// checkoutRequest is the shared public checkout body.
type checkoutRequest struct {
	OrderId string `json:"orderId"`
	Gb      int64  `json:"gb"`
	Months  int    `json:"months"`
}

// checkoutRefEmail resolves and validates the ?ref= link token against the
// panel secret, mapping every failure to an opaque 403 so tokens cannot be
// probed.
func checkoutRefEmail(c *gin.Context) (string, bool) {
	var settingSvc service.SettingService
	secret, err := settingSvc.GetSecret()
	if err != nil || len(secret) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "msg": "checkout is unavailable"})
		return "", false
	}
	email, err := service.ParseCheckoutRef(c.Query("ref"), secret, time.Now())
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "msg": "invalid or expired checkout link"})
		return "", false
	}
	return email, true
}

// HandlePublicCheckoutInfo returns the client's plan state and the checkout
// pricing (GET <basePath>/api/checkout/info?ref=<token>).
func HandlePublicCheckoutInfo(c *gin.Context) {
	email, ok := checkoutRefEmail(c)
	if !ok {
		return
	}
	var svc service.CheckoutService
	info, err := svc.Info(email)
	if err != nil {
		if errors.Is(err, service.ErrCheckoutDisabled) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "msg": "checkout is disabled"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"success": false, "msg": "client not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": info})
}

// HandlePublicCheckoutOrder creates a pending order for the linked client
// (POST <basePath>/api/checkout/order?ref=<token>).
func HandlePublicCheckoutOrder(c *gin.Context) {
	email, ok := checkoutRefEmail(c)
	if !ok {
		return
	}
	var req checkoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": "invalid request body"})
		return
	}
	var svc service.CheckoutService
	order, err := svc.CreateOrder(email, req.Gb, req.Months)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": order})
}

// HandlePublicCheckoutStatus reports one order's payment state for the linked
// client (GET <basePath>/api/checkout/status?ref=<token>&orderId=<id>).
func HandlePublicCheckoutStatus(c *gin.Context) {
	email, ok := checkoutRefEmail(c)
	if !ok {
		return
	}
	var svc service.CheckoutService
	status, err := svc.Status(email, strings.TrimSpace(c.Query("orderId")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": status})
}

// checkoutLink issues a signed public checkout link for one client
// (GET /panel/api/clients/checkout-link/:email). Resellers can only mint
// links for their own clients.
func (a *ClientController) checkoutLink(c *gin.Context) {
	email := strings.ToLower(strings.TrimSpace(c.Param("email")))
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": "email is required"})
		return
	}
	if reseller := resellerSession(c); reseller != nil {
		if !ensureClientOwned(c, reseller, email) {
			return
		}
	}
	if _, err := a.clientService.GetRecordByEmail(nil, email); err != nil {
		jsonMsg(c, I18nWeb(c, "get"), err)
		return
	}
	secret, err := a.settingService.GetSecret()
	if err != nil || len(secret) == 0 {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	token, err := service.SignCheckoutRef(email, secret, time.Now())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"token": token}, nil)
}
