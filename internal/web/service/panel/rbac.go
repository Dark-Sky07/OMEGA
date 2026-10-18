package panel

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"gorm.io/gorm"
)

const (
	AdminRoleSuperAdmin = "superadmin"
	AdminRoleAdmin      = "admin"
	AdminRoleSupport    = "support"
	AdminRoleViewer     = "viewer"
)

var ValidAdminModules = []string{
	"inbounds",
	"clients",
	"resellers",
	"nodes",
	"xray",
	"settings",
	"server",
}

func EffectiveAdminRole(u *model.User) string {
	if u == nil {
		return ""
	}
	if u.Id == 1 {
		return AdminRoleSuperAdmin
	}
	switch strings.ToLower(strings.TrimSpace(u.Role)) {
	case "", AdminRoleSuperAdmin:
		return AdminRoleSuperAdmin
	case AdminRoleAdmin:
		return AdminRoleAdmin
	case AdminRoleSupport:
		return AdminRoleSupport
	case AdminRoleViewer:
		return AdminRoleViewer
	default:
		return AdminRoleSuperAdmin
	}
}

func EffectiveAdminPermissions(u *model.User) []string {
	role := EffectiveAdminRole(u)
	switch role {
	case AdminRoleSuperAdmin:
		return []string{"*"}
	case AdminRoleSupport:
		return []string{"clients", "inbounds:read", "server:read"}
	case AdminRoleViewer:
		return []string{"*:read"}
	case AdminRoleAdmin:
		raw := strings.TrimSpace(u.Permissions)
		if raw == "" {
			return append([]string(nil), ValidAdminModules...)
		}
		var list []string
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				t := strings.ToLower(strings.TrimSpace(p))
				if t != "" {
					list = append(list, t)
				}
			}
		}
		if len(list) == 0 {
			return append([]string(nil), ValidAdminModules...)
		}
		return list
	default:
		return []string{"*"}
	}
}

func IsSuperAdmin(u *model.User) bool {
	return u != nil && !u.Disabled && EffectiveAdminRole(u) == AdminRoleSuperAdmin
}

// isReadOnlyPanelPath reports whether a POST endpoint under /panel/api is a
// read-only query (legacy 3x-ui uses POST for several list/status endpoints).
func isReadOnlyPanelPath(path string, method string) bool {
	if IsReadOnlyAPIRequest(method) {
		return true
	}
	readOnlyPostSuffixes := []string{
		"/panel/api/inbounds/list",
		"/panel/api/inbounds/onlines",
		"/panel/api/inbounds/lastOnline",
		"/panel/api/clients/onlines",
		"/panel/api/clients/onlinesByGuid",
		"/panel/api/clients/activeInbounds",
		"/panel/api/clients/lastOnline",
		"/panel/api/clients/ips/",
		"/panel/api/setting/all",
		"/panel/api/setting/defaultSettings",
	}
	for _, suffix := range readOnlyPostSuffixes {
		if path == suffix || strings.HasPrefix(path, suffix) {
			return true
		}
	}
	return false
}

func classifyPanelModule(path string) string {
	switch {
	case strings.HasPrefix(path, "/panel/api/auth/"),
		path == "/panel/api/setting/defaultSettings":
		return "public_auth"
	case strings.HasPrefix(path, "/panel/api/inbounds"):
		return "inbounds"
	case strings.HasPrefix(path, "/panel/api/clients"):
		return "clients"
	case strings.HasPrefix(path, "/panel/api/resellers"),
		strings.HasPrefix(path, "/panel/api/reseller"):
		return "resellers"
	case strings.HasPrefix(path, "/panel/api/nodes"):
		return "nodes"
	case strings.HasPrefix(path, "/panel/api/xray"):
		return "xray"
	case strings.HasPrefix(path, "/panel/api/server"):
		return "server"
	case strings.HasPrefix(path, "/panel/api/setting"),
		path == "/panel/api/backuptotgbot":
		return "settings"
	default:
		return "settings"
	}
}

// HasAdminPermission evaluates whether an authenticated admin user is allowed
// to access the given /panel/api path and HTTP method.
func HasAdminPermission(u *model.User, path string, method string) bool {
	if u == nil {
		return false
	}
	if u.Disabled && u.Id != 1 {
		return false
	}
	role := EffectiveAdminRole(u)
	if role == AdminRoleSuperAdmin {
		return true
	}
	module := classifyPanelModule(path)
	if module == "public_auth" {
		return true
	}
	// Admin account management is strictly superadmin-only.
	if strings.HasPrefix(path, "/panel/api/setting/admins") {
		return false
	}
	isRead := isReadOnlyPanelPath(path, method)

	switch role {
	case AdminRoleViewer:
		return isRead
	case AdminRoleSupport:
		if module == "clients" {
			return true
		}
		if (module == "inbounds" || module == "server") && isRead {
			return true
		}
		return false
	case AdminRoleAdmin:
		perms := EffectiveAdminPermissions(u)
		for _, p := range perms {
			p = strings.ToLower(strings.TrimSpace(p))
			if p == "*" || p == module {
				return true
			}
			if isRead && (p == "*:read" || p == module+":read") {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func normalizeAdminRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case AdminRoleSuperAdmin:
		return AdminRoleSuperAdmin
	case AdminRoleAdmin:
		return AdminRoleAdmin
	case AdminRoleSupport:
		return AdminRoleSupport
	case AdminRoleViewer:
		return AdminRoleViewer
	default:
		return AdminRoleAdmin
	}
}

func normalizePermissionsJSON(perms []string) string {
	seen := make(map[string]struct{}, len(perms))
	clean := make([]string, 0, len(perms))
	for _, p := range perms {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		clean = append(clean, t)
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func (s *UserService) ListAdmins() ([]model.User, error) {
	db := database.GetDB()
	var users []model.User
	if err := db.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		users[i].Password = ""
		users[i].Role = EffectiveAdminRole(&users[i])
	}
	return users, nil
}

func (s *UserService) CreateAdmin(username, password, role string, permissions []string, note string) (*model.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, common.NewError("username is required")
	}
	if len(password) < 4 {
		return nil, common.NewError("password must be at least 4 characters")
	}
	db := database.GetDB()
	var count int64
	if err := db.Model(&model.User{}).Where("LOWER(username) = LOWER(?)", username).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, common.NewError("username already exists:", username)
	}
	hashed, err := crypto.HashPasswordAsBcrypt(password)
	if err != nil {
		return nil, err
	}
	u := &model.User{
		Username:    username,
		Password:    hashed,
		Role:        normalizeAdminRole(role),
		Permissions: normalizePermissionsJSON(permissions),
		Disabled:    false,
		Note:        strings.TrimSpace(note),
		CreatedAt:   time.Now().UnixMilli(),
	}
	if err := db.Create(u).Error; err != nil {
		return nil, err
	}
	cp := *u
	cp.Password = ""
	return &cp, nil
}

func (s *UserService) UpdateAdmin(id int, username, password, role string, permissions []string, note string, disabled bool) (*model.User, error) {
	if id <= 0 {
		return nil, common.NewError("invalid admin id")
	}
	db := database.GetDB()
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewError("admin not found")
		}
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, common.NewError("username is required")
	}
	var dup int64
	if err := db.Model(&model.User{}).Where("LOWER(username) = LOWER(?) AND id != ?", username, id).Count(&dup).Error; err != nil {
		return nil, err
	}
	if dup > 0 {
		return nil, common.NewError("username already exists:", username)
	}

	// Primary superadmin (id=1) cannot be disabled or demoted.
	if u.Id == 1 {
		role = AdminRoleSuperAdmin
		disabled = false
	}

	u.Username = username
	u.Role = normalizeAdminRole(role)
	u.Permissions = normalizePermissionsJSON(permissions)
	u.Note = strings.TrimSpace(note)
	if u.Disabled != disabled {
		u.Disabled = disabled
		u.LoginEpoch++
	}
	if strings.TrimSpace(password) != "" {
		if len(password) < 4 {
			return nil, common.NewError("password must be at least 4 characters")
		}
		hashed, err := crypto.HashPasswordAsBcrypt(password)
		if err != nil {
			return nil, err
		}
		u.Password = hashed
		u.LoginEpoch++
	}
	if err := db.Save(&u).Error; err != nil {
		return nil, err
	}
	cp := u
	cp.Password = ""
	return &cp, nil
}

func (s *UserService) DeleteAdmin(id int, callerId int) error {
	if id <= 0 {
		return common.NewError("invalid admin id")
	}
	if id == 1 {
		return common.NewError("primary superadmin cannot be deleted")
	}
	if id == callerId {
		return common.NewError("cannot delete your own active admin account")
	}
	db := database.GetDB()
	res := db.Delete(&model.User{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return common.NewError("admin not found")
	}
	return nil
}
