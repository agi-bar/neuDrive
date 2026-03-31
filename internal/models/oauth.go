package models

import (
	"time"

	"github.com/google/uuid"
)

// OAuthApp represents a registered third-party OAuth application.
type OAuthApp struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	Name             string    `json:"name"`
	ClientID         string    `json:"client_id"`
	ClientSecretHash string    `json:"-"`
	RedirectURIs     []string  `json:"redirect_uris"`
	Scopes           []string  `json:"scopes"`
	Description      string    `json:"description"`
	LogoURL          string    `json:"logo_url,omitempty"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
}

// OAuthCode represents a short-lived authorization code.
type OAuthCode struct {
	ID          uuid.UUID `json:"id"`
	AppID       uuid.UUID `json:"app_id"`
	UserID      uuid.UUID `json:"user_id"`
	CodeHash    string    `json:"-"`
	Scopes      []string  `json:"scopes"`
	RedirectURI string    `json:"redirect_uri"`
	ExpiresAt   time.Time `json:"expires_at"`
	Used        bool      `json:"used"`
	CreatedAt   time.Time `json:"created_at"`
}

// OAuthGrant represents a user's authorization of a third-party app.
type OAuthGrant struct {
	ID        uuid.UUID `json:"id"`
	AppID     uuid.UUID `json:"app_id"`
	UserID    uuid.UUID `json:"user_id"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"created_at"`
}

// OAuthAppResponse is the API representation for app registration,
// which includes the client_id but never the secret hash.
type OAuthAppResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	ClientID     string    `json:"client_id"`
	RedirectURIs []string  `json:"redirect_uris"`
	Scopes       []string  `json:"scopes"`
	Description  string    `json:"description"`
	LogoURL      string    `json:"logo_url,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// ToResponse converts an OAuthApp to its API response form.
func (a *OAuthApp) ToResponse() OAuthAppResponse {
	return OAuthAppResponse{
		ID:           a.ID,
		Name:         a.Name,
		ClientID:     a.ClientID,
		RedirectURIs: a.RedirectURIs,
		Scopes:       a.Scopes,
		Description:  a.Description,
		LogoURL:      a.LogoURL,
		IsActive:     a.IsActive,
		CreatedAt:    a.CreatedAt,
	}
}

// RegisterOAuthAppRequest is the request body for registering a new OAuth app.
type RegisterOAuthAppRequest struct {
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
	Scopes       []string `json:"scopes"`
	Description  string   `json:"description"`
	LogoURL      string   `json:"logo_url"`
}

// RegisterOAuthAppResponse includes the client secret (shown only once).
type RegisterOAuthAppResponse struct {
	ClientID     string           `json:"client_id"`
	ClientSecret string           `json:"client_secret"`
	App          OAuthAppResponse `json:"app"`
}

// OAuthTokenRequest is the POST body for the /oauth/token endpoint.
type OAuthTokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
}

// OAuthTokenResponse is returned from the /oauth/token endpoint.
type OAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// OAuthUserInfoResponse is returned from the /oauth/userinfo endpoint.
type OAuthUserInfoResponse struct {
	Sub         string `json:"sub"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Email       string `json:"email,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
	Language    string `json:"language,omitempty"`
}

// OAuthGrantResponse is used when listing grants a user has given.
type OAuthGrantResponse struct {
	ID        uuid.UUID        `json:"id"`
	App       OAuthAppResponse `json:"app"`
	Scopes    []string         `json:"scopes"`
	CreatedAt time.Time        `json:"created_at"`
}
