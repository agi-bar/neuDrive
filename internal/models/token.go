package models

import (
	"time"

	"github.com/google/uuid"
)

// Token scope constants — hierarchical, colon-separated
// Format: "action:resource" or "action:resource.sub"
const (
	ScopeReadProfile  = "read:profile"
	ScopeWriteProfile = "write:profile"

	ScopeReadMemory  = "read:memory"
	ScopeWriteMemory = "write:memory"

	ScopeReadVault     = "read:vault"      // all vault
	ScopeReadVaultAuth = "read:vault.auth" // only auth.* scopes
	ScopeWriteVault    = "write:vault"

	ScopeReadSkills  = "read:skills"
	ScopeWriteSkills = "write:skills"

	ScopeReadDevices = "read:devices"
	ScopeCallDevices = "call:devices"

	ScopeReadInbox  = "read:inbox"
	ScopeWriteInbox = "write:inbox"

	ScopeReadProjects  = "read:projects"
	ScopeWriteProjects = "write:projects"

	ScopeReadTree  = "read:tree"
	ScopeWriteTree = "write:tree"

	ScopeReadBundle  = "read:bundle"
	ScopeWriteBundle = "write:bundle"

	ScopeSearch = "search"

	ScopeAdmin = "admin" // full access
)

// AllScopes returns every recognised scope for validation.
var AllScopes = []string{
	ScopeReadProfile, ScopeWriteProfile,
	ScopeReadMemory, ScopeWriteMemory,
	ScopeReadVault, ScopeReadVaultAuth, ScopeWriteVault,
	ScopeReadSkills, ScopeWriteSkills,
	ScopeReadDevices, ScopeCallDevices,
	ScopeReadInbox, ScopeWriteInbox,
	ScopeReadProjects, ScopeWriteProjects,
	ScopeReadTree, ScopeWriteTree,
	ScopeReadBundle, ScopeWriteBundle,
	ScopeSearch,
	ScopeAdmin,
}

// Predefined scope bundles (for easy token creation).
var ScopeBundleReadOnly = []string{
	ScopeReadProfile, ScopeReadMemory, ScopeReadSkills,
	ScopeReadProjects, ScopeReadTree, ScopeSearch,
}

var ScopeBundleAgent = []string{
	ScopeReadProfile, ScopeReadMemory, ScopeWriteMemory,
	ScopeReadSkills, ScopeReadVaultAuth,
	ScopeReadProjects, ScopeWriteProjects,
	ScopeReadTree, ScopeWriteTree,
	ScopeSearch,
}

var ScopeBundleFull = []string{ScopeAdmin}

// ScopeCategories returns scopes grouped by category for UI display.
func ScopeCategories() map[string][]string {
	return map[string][]string{
		"Profile":  {ScopeReadProfile, ScopeWriteProfile},
		"Memory":   {ScopeReadMemory, ScopeWriteMemory},
		"Vault":    {ScopeReadVault, ScopeReadVaultAuth, ScopeWriteVault},
		"Skills":   {ScopeReadSkills, ScopeWriteSkills},
		"Projects": {ScopeReadProjects, ScopeWriteProjects},
		"Tree":     {ScopeReadTree, ScopeWriteTree},
		"Bundle":   {ScopeReadBundle, ScopeWriteBundle},
		"Search":   {ScopeSearch},
		"Admin":    {ScopeAdmin},
	}
}

// ScopedToken represents a scoped access token stored in the database.
type ScopedToken struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	Name             string     `json:"name"`
	TokenHash        string     `json:"-"` // never expose
	TokenPrefix      string     `json:"token_prefix"`
	Scopes           []string   `json:"scopes"`
	MaxTrustLevel    int        `json:"max_trust_level"`
	ExpiresAt        time.Time  `json:"expires_at"`
	RateLimit        int        `json:"rate_limit"`
	RequestCount     int        `json:"request_count"`
	RateLimitResetAt time.Time  `json:"rate_limit_reset_at"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP       *string    `json:"last_used_ip,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
}

// ScopedTokenResponse is the API representation (excludes hash, adds computed fields).
type ScopedTokenResponse struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Name          string     `json:"name"`
	TokenPrefix   string     `json:"token_prefix"`
	Scopes        []string   `json:"scopes"`
	MaxTrustLevel int        `json:"max_trust_level"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RateLimit     int        `json:"rate_limit"`
	RequestCount  int        `json:"request_count"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	LastUsedIP    *string    `json:"last_used_ip,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	IsExpired     bool       `json:"is_expired"`
	IsRevoked     bool       `json:"is_revoked"`
}

// CreateTokenRequest for the API.
type CreateTokenRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	MaxTrustLevel int      `json:"max_trust_level"`
	ExpiresInDays int      `json:"expires_in_days"`
}

// UpdateTokenRequest allows changing mutable token metadata.
type UpdateTokenRequest struct {
	Name string `json:"name"`
}

// CreateTokenResponse includes the raw token (shown only once).
type CreateTokenResponse struct {
	Token       string              `json:"token"` // raw token, shown only once
	TokenPrefix string              `json:"token_prefix"`
	ScopedToken ScopedTokenResponse `json:"scoped_token"`
}

// ValidateTokenRequest is used by external services to validate a token.
type ValidateTokenRequest struct {
	Token string `json:"token"`
}

// ValidateTokenResponse is returned by the validate endpoint.
type ValidateTokenResponse struct {
	Valid         bool     `json:"valid"`
	UserID        string   `json:"user_id,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	MaxTrustLevel int      `json:"max_trust_level,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// ToResponse converts a ScopedToken to a ScopedTokenResponse.
func (t *ScopedToken) ToResponse() ScopedTokenResponse {
	return ScopedTokenResponse{
		ID:            t.ID,
		UserID:        t.UserID,
		Name:          t.Name,
		TokenPrefix:   t.TokenPrefix,
		Scopes:        t.Scopes,
		MaxTrustLevel: t.MaxTrustLevel,
		ExpiresAt:     t.ExpiresAt,
		RateLimit:     t.RateLimit,
		RequestCount:  t.RequestCount,
		LastUsedAt:    t.LastUsedAt,
		LastUsedIP:    t.LastUsedIP,
		CreatedAt:     t.CreatedAt,
		RevokedAt:     t.RevokedAt,
		IsExpired:     t.IsExpired(),
		IsRevoked:     t.RevokedAt != nil,
	}
}

// IsExpired checks if the token has passed its expiration time.
func (t *ScopedToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsRevoked checks if the token has been revoked.
func (t *ScopedToken) IsRevoked() bool {
	return t.RevokedAt != nil
}

// IsActive returns true if the token is neither expired nor revoked.
func (t *ScopedToken) IsActive() bool {
	return !t.IsExpired() && !t.IsRevoked()
}

// HasScope checks if the token's scopes include the required scope.
// Handles hierarchical matching: "read:vault" matches "read:vault.auth".
// "admin" matches everything.
func HasScope(scopes []string, required string) bool {
	for _, s := range scopes {
		if s == ScopeAdmin || s == required {
			return true
		}
		// Hierarchical: "read:vault" covers "read:vault.auth"
		if len(required) > len(s) && required[:len(s)] == s && required[len(s)] == '.' {
			return true
		}
	}
	return false
}
