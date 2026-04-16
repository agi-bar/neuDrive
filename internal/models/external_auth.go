package models

import (
	"time"

	"github.com/google/uuid"
)

type AuthProvider struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
}

type AuthProviderAction string

const (
	AuthProviderActionLogin  AuthProviderAction = "login"
	AuthProviderActionSignup AuthProviderAction = "signup"
)

type StartAuthProviderRequest struct {
	RedirectURL string             `json:"redirect_url"`
	Action      AuthProviderAction `json:"action"`
}

type StartAuthProviderResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

type AuthTransaction struct {
	ID           uuid.UUID  `json:"id"`
	ProviderKey  string     `json:"provider_key"`
	State        string     `json:"state"`
	Nonce        string     `json:"nonce"`
	CodeVerifier string     `json:"code_verifier"`
	RedirectURL  string     `json:"redirect_url"`
	ExpiresAt    time.Time  `json:"expires_at"`
	ConsumedAt   *time.Time `json:"consumed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type ExternalIdentityUpsert struct {
	ProviderKey    string                 `json:"provider_key"`
	Issuer         string                 `json:"issuer"`
	Subject        string                 `json:"subject"`
	Email          string                 `json:"email"`
	EmailVerified  bool                   `json:"email_verified"`
	DisplayName    string                 `json:"display_name"`
	AvatarURL      string                 `json:"avatar_url"`
	Timezone       string                 `json:"timezone"`
	Language       string                 `json:"language"`
	SlugCandidates []string               `json:"slug_candidates"`
	ProfileData    map[string]interface{} `json:"profile_data"`
}

type ExternalAuthCallbackResult struct {
	RedirectURL string        `json:"redirect_url"`
	Auth        *AuthResponse `json:"auth,omitempty"`
}
