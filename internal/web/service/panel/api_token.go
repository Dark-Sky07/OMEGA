package panel

import (
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/util/common"
	"github.com/Dark-Sky07/phoenix/internal/util/crypto"
	"github.com/Dark-Sky07/phoenix/internal/util/random"
)

type ApiTokenService struct{}

const (
	apiTokenLength = 48
	// apiTokenPrefixLen is how many leading plaintext characters are kept so
	// admins can tell tokens apart in the list without seeing the secret.
	apiTokenPrefixLen = 8
	// apiTokenUsageFlushSecs throttles last-used writes: at most one UPDATE
	// per token per interval keeps high-frequency API callers (node sync
	// every few seconds) from writing on every request.
	apiTokenUsageFlushSecs = 60
)

// Token scopes. A read-only token may call GET/HEAD/OPTIONS but never a
// mutating method. Legacy rows carry an empty scope and behave as full.
const (
	ApiTokenScopeFull = "full"
	ApiTokenScopeRead = "read"
)

// IsReadOnlyAPIRequest reports whether the HTTP method is safe to serve to a
// read-only token. Pure helper so the policy is unit-testable.
func IsReadOnlyAPIRequest(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return true
	}
	return false
}

type ApiTokenView struct {
	Id         int    `json:"id" example:"2"`
	Name       string `json:"name" example:"central-panel-a"`
	Token      string `json:"token,omitempty" example:"new-token-string"`
	Prefix     string `json:"prefix" example:"a1b2c3d4"`
	Scope      string `json:"scope" example:"read"`
	Enabled    bool   `json:"enabled" example:"true"`
	ExpiresAt  int64  `json:"expiresAt" example:"0"`
	LastUsedAt int64  `json:"lastUsedAt" example:"0"`
	CreatedAt  int64  `json:"createdAt" example:"1736000000"`
}

// NormalizeTokenScope maps any accepted scope spelling to its canonical
// value; empty and unknown inputs fall back to full access.
func NormalizeTokenScope(scope string) string {
	if strings.EqualFold(strings.TrimSpace(scope), ApiTokenScopeRead) {
		return ApiTokenScopeRead
	}
	return ApiTokenScopeFull
}

// toView builds the metadata view returned by List. It never carries the
// token value: only a SHA-256 hash is stored, and the plaintext is shown
// exactly once at creation time.
func toView(t *model.ApiToken) *ApiTokenView {
	scope := NormalizeTokenScope(t.Scope)
	return &ApiTokenView{
		Id:         t.Id,
		Name:       t.Name,
		Prefix:     t.Prefix,
		Scope:      scope,
		Enabled:    t.Enabled,
		ExpiresAt:  t.ExpiresAt,
		LastUsedAt: t.LastUsedAt,
		CreatedAt:  t.CreatedAt,
	}
}

func (s *ApiTokenService) List() ([]*ApiTokenView, error) {
	db := database.GetDB()
	var rows []*model.ApiToken
	if err := db.Model(model.ApiToken{}).Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*ApiTokenView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toView(r))
	}
	return out, nil
}

// Create generates a full-access, non-expiring token. Callers that want a
// read-only scope or an expiry deadline use CreateWithOptions.
func (s *ApiTokenService) Create(name string) (*ApiTokenView, error) {
	return s.CreateWithOptions(name, ApiTokenScopeFull, 0)
}

// CreateWithOptions generates a new token. scope is one of the ApiTokenScope*
// values (empty means full); expireDays of 0 means the token never expires.
func (s *ApiTokenService) CreateWithOptions(name string, scope string, expireDays int) (*ApiTokenView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, common.NewError("token name is required")
	}
	if len(name) > 64 {
		return nil, common.NewError("token name must be 64 characters or fewer")
	}
	if expireDays < 0 {
		return nil, common.NewError("expire days must be 0 or greater")
	}
	db := database.GetDB()
	var count int64
	if err := db.Model(model.ApiToken{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, common.NewError("a token with that name already exists")
	}
	plaintext := random.Seq(apiTokenLength)
	row := &model.ApiToken{
		Name:    name,
		Token:   crypto.HashTokenSHA256(plaintext),
		Prefix:  plaintext[:apiTokenPrefixLen],
		Scope:   NormalizeTokenScope(scope),
		Enabled: true,
	}
	if expireDays > 0 {
		row.ExpiresAt = time.Now().AddDate(0, 0, expireDays).UnixMilli()
	}
	if err := db.Create(row).Error; err != nil {
		return nil, err
	}
	view := toView(row)
	view.Token = plaintext
	return view, nil
}

func (s *ApiTokenService) Delete(id int) error {
	if id <= 0 {
		return common.NewError("invalid token id")
	}
	db := database.GetDB()
	return db.Where("id = ?", id).Delete(model.ApiToken{}).Error
}

func (s *ApiTokenService) SetEnabled(id int, enabled bool) error {
	if id <= 0 {
		return common.NewError("invalid token id")
	}
	db := database.GetDB()
	res := db.Model(model.ApiToken{}).Where("id = ?", id).Update("enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("token not found")
	}
	return nil
}

// Authenticate returns the token row matching the presented bearer value.
// Tokens are stored as SHA-256 hashes, so the presented value is hashed
// before a constant-time compare per row keeps a remote attacker from timing
// the comparison byte-by-byte. Disabled and expired tokens never match. On
// success the row's last-used timestamp is refreshed (at most once per
// apiTokenUsageFlushSecs to keep write volume flat).
func (s *ApiTokenService) Authenticate(presented string) (*model.ApiToken, bool) {
	if presented == "" {
		return nil, false
	}
	db := database.GetDB()
	var rows []*model.ApiToken
	if err := db.Model(model.ApiToken{}).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return nil, false
	}
	presentedHash := []byte(crypto.HashTokenSHA256(presented))
	now := time.Now().UnixMilli()
	var matched *model.ApiToken
	for _, r := range rows {
		if r.ExpiresAt > 0 && now > r.ExpiresAt {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(r.Token), presentedHash) == 1 {
			matched = r
		}
	}
	if matched == nil {
		return nil, false
	}
	if now-matched.LastUsedAt > int64(apiTokenUsageFlushSecs)*1000 {
		db.Model(model.ApiToken{}).Where("id = ?", matched.Id).Update("last_used_at", now)
		matched.LastUsedAt = now
	}
	return matched, true
}

// Match is the boolean form of Authenticate, kept for callers that only need
// to know whether the request is authorized.
func (s *ApiTokenService) Match(presented string) bool {
	_, ok := s.Authenticate(presented)
	return ok
}
