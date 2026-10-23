package controller

import (
	"net/http"
	"strings"

	"github.com/Dark-Sky07/phoenix/internal/web/service/panel"
	"github.com/Dark-Sky07/phoenix/internal/web/session"
	"github.com/gin-gonic/gin"
)

func (a *XUIController) simurghPanel(c *gin.Context) {
	if session.IsResellerLogin(c) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	token, err := session.EnsureCSRFToken(c)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	basePath := strings.TrimSuffix(c.GetString("base_path"), "/")
	prefix := basePath + "/panel/simurgh"
	if c.Request.Method == http.MethodPost && strings.TrimSuffix(c.Request.URL.Path, "/") == prefix+"/api/update" &&
		!panel.HasAdminPermission(session.GetLoginUser(c), "/panel/api/server/updateSimurgh", http.MethodPost) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	a.simurgh.serve(c.Writer, c.Request, prefix, token, c.GetString("csp_nonce"))
}
