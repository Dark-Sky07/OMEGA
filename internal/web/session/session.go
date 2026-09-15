package session

import (
	"encoding/gob"
	"net/http"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	loginUserKey      = "LOGIN_USER"
	loginEpochKey     = "LOGIN_EPOCH"
	loginResellerKey  = "LOGIN_RESELLER"
	resellerEpochKey  = "LOGIN_RESELLER_EPOCH"
	apiAuthUserKey    = "api_auth_user"
	sessionCookieName = "3x-ui"
)

func init() {
	gob.Register(model.User{})
	gob.Register(model.Reseller{})
}

// SetLoginReseller stores a reseller (نمایندگی) login in the session. Admin and
// reseller logins are mutually exclusive: signing in as one clears the other.
func SetLoginReseller(c *gin.Context, reseller *model.Reseller) error {
	if reseller == nil {
		return nil
	}
	s := sessions.Default(c)
	s.Delete(loginUserKey)
	s.Delete(loginEpochKey)
	s.Set(loginResellerKey, reseller.Id)
	s.Set(resellerEpochKey, reseller.LoginEpoch)
	return s.Save()
}

// GetLoginReseller returns the reseller bound to the current session, or nil
// when the session belongs to the admin (or nobody).
func GetLoginReseller(c *gin.Context) *model.Reseller {
	if apiUser, ok := c.Get(apiAuthUserKey); ok {
		if _, isUser := apiUser.(*model.User); isUser {
			return nil
		}
	}
	s := sessions.Default(c)
	id := intValue(s.Get(loginResellerKey))
	if id <= 0 {
		return nil
	}
	reseller, err := getResellerByID(id)
	if err != nil {
		s.Delete(loginResellerKey)
		s.Delete(resellerEpochKey)
		if saveErr := s.Save(); saveErr != nil {
			logger.Warning("session: failed to drop missing reseller:", saveErr)
		}
		return nil
	}
	if int64Value(s.Get(resellerEpochKey)) != reseller.LoginEpoch {
		s.Delete(loginResellerKey)
		s.Delete(resellerEpochKey)
		if saveErr := s.Save(); saveErr != nil {
			logger.Warning("session: failed to drop stale reseller session:", saveErr)
		}
		return nil
	}
	return reseller
}

// IsResellerLogin reports whether the current session is a reseller session.
func IsResellerLogin(c *gin.Context) bool {
	return GetLoginReseller(c) != nil
}

func SetLoginUser(c *gin.Context, user *model.User) error {
	if user == nil {
		return nil
	}
	s := sessions.Default(c)
	s.Set(loginUserKey, user.Id)
	s.Set(loginEpochKey, user.LoginEpoch)
	return s.Save()
}

func SetAPIAuthUser(c *gin.Context, user *model.User) {
	if user == nil {
		return
	}
	c.Set(apiAuthUserKey, user)
}

func GetLoginUser(c *gin.Context) *model.User {
	if v, ok := c.Get(apiAuthUserKey); ok {
		if u, ok2 := v.(*model.User); ok2 {
			return u
		}
	}
	s := sessions.Default(c)
	obj := s.Get(loginUserKey)
	if obj == nil {
		return nil
	}
	userID, ok := sessionUserID(obj)
	if !ok {
		s.Delete(loginUserKey)
		s.Delete(loginEpochKey)
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to drop stale user payload:", err)
		}
		return nil
	}
	if legacyUserID, ok := legacySessionUserID(obj); ok {
		s.Set(loginUserKey, legacyUserID)
		if err := s.Save(); err != nil {
			logger.Warning("session: failed to migrate legacy user payload:", err)
		}
	}
	user, err := getUserByID(userID)
	if err != nil {
		logger.Warning("session: failed to load user:", err)
		s.Delete(loginUserKey)
		s.Delete(loginEpochKey)
		if saveErr := s.Save(); saveErr != nil {
			logger.Warning("session: failed to drop missing user:", saveErr)
		}
		return nil
	}
	if !sessionEpochMatches(s.Get(loginEpochKey), user.LoginEpoch) {
		s.Delete(loginUserKey)
		s.Delete(loginEpochKey)
		if saveErr := s.Save(); saveErr != nil {
			logger.Warning("session: failed to drop stale epoch:", saveErr)
		}
		return nil
	}
	return user
}

func sessionEpochMatches(cookieVal any, userEpoch int64) bool {
	var got int64
	switch v := cookieVal.(type) {
	case nil:
	case int64:
		got = v
	case int:
		got = int64(v)
	case int32:
		got = int64(v)
	case float64:
		got = int64(v)
	default:
		return false
	}
	return got == userEpoch
}

func IsLogin(c *gin.Context) bool {
	return GetLoginUser(c) != nil || GetLoginReseller(c) != nil
}

// intValue coerces a session-stored numeric value (gob round-trips ints as int)
// into an int, returning 0 when it is not a positive integer.
func intValue(obj any) int {
	switch v := obj.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func int64Value(obj any) int64 {
	switch v := obj.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func getResellerByID(id int) (*model.Reseller, error) {
	db := database.GetDB()
	if db == nil {
		return nil, http.ErrServerClosed
	}
	reseller := &model.Reseller{}
	if err := db.Model(model.Reseller{}).Where("id = ?", id).First(reseller).Error; err != nil {
		return nil, err
	}
	return reseller, nil
}

func sessionUserID(obj any) (int, bool) {
	switch v := obj.(type) {
	case int:
		return v, v > 0
	case int64:
		return int(v), v > 0
	case int32:
		return int(v), v > 0
	case float64:
		id := int(v)
		return id, v == float64(id) && id > 0
	case model.User:
		return v.Id, v.Id > 0
	case *model.User:
		if v == nil {
			return 0, false
		}
		return v.Id, v.Id > 0
	default:
		return 0, false
	}
}

func legacySessionUserID(obj any) (int, bool) {
	switch v := obj.(type) {
	case model.User:
		return v.Id, v.Id > 0
	case *model.User:
		if v == nil {
			return 0, false
		}
		return v.Id, v.Id > 0
	default:
		return 0, false
	}
}

func getUserByID(id int) (*model.User, error) {
	db := database.GetDB()
	if db == nil {
		return nil, http.ErrServerClosed
	}
	user := &model.User{}
	if err := db.Model(model.User{}).Where("id = ?", id).First(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func ClearSession(c *gin.Context) error {
	s := sessions.Default(c)
	s.Clear()
	cookiePath := c.GetString("base_path")
	if cookiePath == "" {
		cookiePath = "/"
	}
	secure := c.Request.TLS != nil
	s.Options(sessions.Options{
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	if err := s.Save(); err != nil {
		return err
	}
	if cookiePath != "/" {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
	return nil
}
