package models

import (
	"time"

	"github.com/google/uuid"
)

// Token scope constants - fine-grained permissions
const (
	// Profile scopes
	ScopeReadProfile  = "read:profile"
	ScopeWriteProfile = "write:profile"

	// Memory scopes
	ScopeReadMemory  = "read:memory"
	ScopeWriteMemory = "write:memory"

	// Vault scopes (can be further narrowed: read:vault.auth, read:vault.identity)
	ScopeReadVault  = "read:vault"
	ScopeWriteVault = "write:vault"

	// Skills scopes
	ScopeReadSkills  = "read:skills"
	ScopeWriteSkills = "write:skills"

	// Devices
	ScopeReadDevices = "read:devices"
	ScopeCallDevices = "call:devices"

	// Inbox
	ScopeReadInbox = "read:inbox"
	ScopeSendInbox = "send:inbox"

	// Projects
	ScopeReadProjects  = "read:projects"
	ScopeWriteProjects = "write:projects"

	// Roles
	ScopeReadRoles   = "read:roles"
	ScopeManageRoles = "manage:roles"

	// File tree (general)
	ScopeReadTree  = "read:tree"
	ScopeWriteTree = "write:tree"

	// Admin - full access
	ScopeAdmin = "admin"
)

// AllScopes returns all available scopes
func AllScopes() []string {
	return []string{
		ScopeReadProfile, ScopeWriteProfile,
		ScopeReadMemory, ScopeWriteMemory,
		ScopeReadVault, ScopeWriteVault,
		ScopeReadSkills, ScopeWriteSkills,
		ScopeReadDevices, ScopeCallDevices,
		ScopeReadInbox, ScopeSendInbox,
		ScopeReadProjects, ScopeWriteProjects,
		ScopeReadRoles, ScopeManageRoles,
		ScopeReadTree, ScopeWriteTree,
		ScopeAdmin,
	}
}

// ScopeCategories returns scopes grouped by category for UI display
func ScopeCategories() map[string][]string {
	return map[string][]string{
		"身份与偏好": {ScopeReadProfile, ScopeWriteProfile},
		"记忆":      {ScopeReadMemory, ScopeWriteMemory},
		"密钥保险柜":  {ScopeReadVault, ScopeWriteVault},
		"技能":      {ScopeReadSkills, ScopeWriteSkills},
		"设备":      {ScopeReadDevices, ScopeCallDevices},
		"收件箱":     {ScopeReadInbox, ScopeSendInbox},
		"项目":      {ScopeReadProjects, ScopeWriteProjects},
		"角色":      {ScopeReadRoles, ScopeManageRoles},
		"文件树":     {ScopeReadTree, ScopeWriteTree},
		"管理员":     {ScopeAdmin},
	}
}

type AccessToken struct {
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Name          string     `json:"name"`
	TokenHash     string     `json:"-"`          // never expose
	TokenPrefix   string     `json:"token_prefix"`
	Scopes        []string   `json:"scopes"`
	MaxTrustLevel int        `json:"max_trust_level"`
	ExpiresAt     *time.Time `json:"expires_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	LastUsedIP    string     `json:"last_used_ip,omitempty"`
	UseCount      int        `json:"use_count"`
	IsActive      bool       `json:"is_active"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CreateTokenRequest for API
type CreateTokenRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	MaxTrustLevel int      `json:"max_trust_level"`
	ExpiresInDays *int     `json:"expires_in_days"` // nil = never expires
}

// CreateTokenResponse includes the raw token (shown only once)
type CreateTokenResponse struct {
	Token       string      `json:"token"`        // raw token, shown only once!
	TokenPrefix string      `json:"token_prefix"`
	AccessToken AccessToken `json:"access_token"`
}

// HasScope checks if the token has a specific scope (or admin)
func (t *AccessToken) HasScope(scope string) bool {
	for _, s := range t.Scopes {
		if s == ScopeAdmin || s == scope {
			return true
		}
		// Check wildcard: "read:vault" covers "read:vault.auth"
		if len(scope) > len(s) && scope[:len(s)] == s && scope[len(s)] == '.' {
			return true
		}
	}
	return false
}

// IsExpired checks if the token has expired
func (t *AccessToken) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}
