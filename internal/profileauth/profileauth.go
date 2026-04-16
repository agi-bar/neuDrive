package profileauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
)

type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
}

type oauthTokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func OAuthClientID(apiBase string) string {
	return strings.TrimRight(strings.TrimSpace(apiBase), "/") + "/oauth/clients/neu-cli"
}

func ExchangeCode(ctx context.Context, apiBase, code, redirectURI, codeVerifier string) (*Session, error) {
	return exchangeToken(ctx, apiBase, oauthTokenRequest{
		GrantType:    "authorization_code",
		Code:         strings.TrimSpace(code),
		ClientID:     OAuthClientID(apiBase),
		RedirectURI:  strings.TrimSpace(redirectURI),
		CodeVerifier: strings.TrimSpace(codeVerifier),
	})
}

func Refresh(ctx context.Context, apiBase, refreshToken string) (*Session, error) {
	return exchangeToken(ctx, apiBase, oauthTokenRequest{
		GrantType:    "refresh_token",
		ClientID:     OAuthClientID(apiBase),
		RefreshToken: strings.TrimSpace(refreshToken),
	})
}

func exchangeToken(ctx context.Context, apiBase string, payload oauthTokenRequest) (*Session, error) {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if !strings.HasPrefix(strings.ToLower(apiBase), "https://") {
		return nil, fmt.Errorf("hosted OAuth login requires an HTTPS api base")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeTokenError(resp.StatusCode, respBody)
	}

	var tokenResp models.OAuthTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return &Session{
		AccessToken:  strings.TrimSpace(tokenResp.AccessToken),
		RefreshToken: strings.TrimSpace(tokenResp.RefreshToken),
		ExpiresAt:    expiresAt,
		Scopes:       strings.Fields(tokenResp.Scope),
	}, nil
}

func TokenExpired(expiresAt string) bool {
	expiresAt = strings.TrimSpace(expiresAt)
	if expiresAt == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false
	}
	return !parsed.After(time.Now().UTC())
}

func EnsureProfileToken(ctx context.Context, configPath string, cfg *runtimecfg.CLIConfig, profileName string) (runtimecfg.SyncProfile, error) {
	if cfg == nil {
		return runtimecfg.SyncProfile{}, fmt.Errorf("missing cli config")
	}
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return runtimecfg.SyncProfile{}, fmt.Errorf("profile name is required")
	}
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return runtimecfg.SyncProfile{}, fmt.Errorf("profile %s does not exist", profileName)
	}
	if strings.TrimSpace(profile.AuthMode) != runtimecfg.AuthModeOAuthSession {
		return profile, nil
	}
	if strings.TrimSpace(profile.Token) != "" && !TokenExpired(profile.ExpiresAt) {
		return profile, nil
	}
	if strings.TrimSpace(profile.RefreshToken) == "" {
		return runtimecfg.SyncProfile{}, fmt.Errorf("profile %s has no refresh token; run `neu login --profile %s`", profileName, profileName)
	}
	session, err := Refresh(ctx, profile.APIBase, profile.RefreshToken)
	if err != nil {
		return runtimecfg.SyncProfile{}, fmt.Errorf("refresh profile %s: %w", profileName, err)
	}
	profile.Token = session.AccessToken
	if session.RefreshToken != "" {
		profile.RefreshToken = session.RefreshToken
	}
	profile.ExpiresAt = session.ExpiresAt.UTC().Format(time.RFC3339)
	profile.Scopes = append([]string{}, session.Scopes...)
	profile.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	profile.AuthMode = runtimecfg.AuthModeOAuthSession
	cfg.Profiles[profileName] = profile
	if err := runtimecfg.SaveConfig(configPath, cfg); err != nil {
		return runtimecfg.SyncProfile{}, err
	}
	return profile, nil
}

func decodeTokenError(status int, body []byte) error {
	var oauthErr struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
	}
	if err := json.Unmarshal(body, &oauthErr); err == nil {
		switch {
		case strings.TrimSpace(oauthErr.ErrorDescription) != "":
			return fmt.Errorf("oauth token exchange failed (%d): %s", status, oauthErr.ErrorDescription)
		case strings.TrimSpace(oauthErr.Message) != "":
			return fmt.Errorf("oauth token exchange failed (%d): %s", status, oauthErr.Message)
		case strings.TrimSpace(oauthErr.Error) != "":
			return fmt.Errorf("oauth token exchange failed (%d): %s", status, oauthErr.Error)
		}
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = http.StatusText(status)
	}
	return fmt.Errorf("oauth token exchange failed (%d): %s", status, text)
}
