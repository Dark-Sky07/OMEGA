package controller

import (
	"net/http"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Session identity (/panel/api/auth/me)
// ---------------------------------------------------------------------------

// AuthController exposes the identity of the caller so the SPA can render the
// right navigation for admins and resellers alike.
type AuthController struct {
	BaseController
	resellerService service.ResellerService
	userService     panel.UserService
}

// NewAuthController registers the session-identity endpoint.
func NewAuthController(g *gin.RouterGroup, users panel.UserService) *AuthController {
	a := &AuthController{
		resellerService: service.ResellerService{},
		userService:     users,
	}
	g.GET("/me", a.me)
	return a
}

func (a *AuthController) me(c *gin.Context) {
	if reseller := session.GetLoginReseller(c); reseller != nil {
		stat, err := a.resellerService.Stat(reseller)
		if err != nil {
			jsonMsg(c, "", err)
			return
		}
		jsonObj(c, gin.H{
			"role":     "reseller",
			"username": reseller.Username,
			"name":     reseller.Name,
			"stat":     stat,
		}, nil)
		return
	}
	user := session.GetLoginUser(c)
	if user == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "not logged in"})
		return
	}
	username := user.Username
	if first, err := a.userService.GetFirstUser(); err == nil && first != nil {
		username = first.Username
	}
	jsonObj(c, gin.H{
		"role":     "admin",
		"username": username,
		"name":     username,
	}, nil)
}
