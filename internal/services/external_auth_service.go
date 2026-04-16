package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/agi-bar/neudrive/internal/config"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

const (
	externalAuthTxnTTL        = 10 * time.Minute
	externalAuthAllowedSkew   = 2 * time.Minute
	externalAuthDefaultTZ     = "UTC"
	externalAuthDefaultLang   = "en"
	externalAuthMinSlugLength = 3
)

var slugSanitizer = regexp.MustCompile(`[^a-z0-9_-]+`)

type ExternalAuthService struct {
	db          *pgxpool.Pool
	repo        ExternalAuthRepo
	authService *AuthService
	cfg         *config.Config
	httpClient  *http.Client

	pocketMu       sync.RWMutex
	pocketMetadata *pocketProviderMetadata
}

type pocketProviderMetadata struct {
	Issuer                 string   `json:"issuer"`
	AuthorizationEndpoint  string   `json:"authorization_endpoint"`
	TokenEndpoint          string   `json:"token_endpoint"`
	JWKSURI                string   `json:"jwks_uri"`
	UserInfoEndpoint       string   `json:"userinfo_endpoint"`
	IDTokenSigningAlgs     []string `json:"id_token_signing_alg_values_supported"`
	ResponseTypesSupported []string `json:"response_types_supported"`
}

type oidcProfileClaims struct {
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Nickname          string `json:"nickname"`
	Picture           string `json:"picture"`
	Locale            string `json:"locale"`
	Zoneinfo          string `json:"zoneinfo"`
	Azp               string `json:"azp"`
	TokenUse          string `json:"token_use"`
	NotBefore         int64  `json:"nbf"`
	EmailVerified     *bool  `json:"email_verified"`
}

type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

type githubProfile struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
	Location  string `json:"location"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func NewExternalAuthService(db *pgxpool.Pool, authSvc *AuthService, cfg *config.Config) *ExternalAuthService {
	return &ExternalAuthService{
		db:          db,
		authService: authSvc,
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func NewExternalAuthServiceWithRepo(repo ExternalAuthRepo, authSvc *AuthService, cfg *config.Config) *ExternalAuthService {
	return &ExternalAuthService{
		repo:        repo,
		authService: authSvc,
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *ExternalAuthService) ListProviders() []models.AuthProvider {
	providers := []models.AuthProvider{
		{
			ID:          "github",
			Kind:        "oauth2",
			DisplayName: "GitHub",
			Enabled:     s.githubEnabled(),
		},
		{
			ID:          s.pocketProviderKey(),
			Kind:        "oidc",
			DisplayName: "Pocket ID",
			Enabled:     s.pocketEnabled(),
		},
	}
	return providers
}

func (s *ExternalAuthService) Start(ctx context.Context, providerKey, callbackURL, redirectURL string) (*models.StartAuthProviderResponse, error) {
	if s.authService == nil {
		return nil, fmt.Errorf("auth service not configured")
	}
	switch providerKey {
	case "github":
		if !s.githubEnabled() {
			return nil, fmt.Errorf("provider %q is not enabled", providerKey)
		}
	case s.pocketProviderKey():
		if !s.pocketEnabled() {
			return nil, fmt.Errorf("provider %q is not enabled", providerKey)
		}
	default:
		return nil, fmt.Errorf("unsupported provider %q", providerKey)
	}

	state, err := randomURLSafeString(32)
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	verifier, err := randomURLSafeString(64)
	if err != nil {
		return nil, fmt.Errorf("generate code verifier: %w", err)
	}
	nonce := ""
	if providerKey == s.pocketProviderKey() {
		nonce, err = randomURLSafeString(32)
		if err != nil {
			return nil, fmt.Errorf("generate nonce: %w", err)
		}
	}
	now := time.Now().UTC()
	if err := s.createAuthTransaction(ctx, models.AuthTransaction{
		ID:           uuid.New(),
		ProviderKey:  providerKey,
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		RedirectURL:  redirectURL,
		ExpiresAt:    now.Add(externalAuthTxnTTL),
		CreatedAt:    now,
	}); err != nil {
		return nil, err
	}

	var authorizationURL string
	switch providerKey {
	case "github":
		authorizationURL = s.githubAuthorizationURL(state, verifier, callbackURL)
	case s.pocketProviderKey():
		authorizationURL, err = s.pocketAuthorizationURL(ctx, state, verifier, nonce, callbackURL)
		if err != nil {
			return nil, err
		}
	}

	return &models.StartAuthProviderResponse{AuthorizationURL: authorizationURL}, nil
}

func (s *ExternalAuthService) Complete(
	ctx context.Context,
	providerKey, state, code, providerError, providerErrorDescription, callbackURL, userAgent, ipAddress string,
) (*models.ExternalAuthCallbackResult, error) {
	if s.authService == nil {
		return nil, fmt.Errorf("auth service not configured")
	}
	if strings.TrimSpace(state) == "" {
		return nil, fmt.Errorf("missing state")
	}

	txn, err := s.consumeAuthTransaction(ctx, providerKey, state, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	result := &models.ExternalAuthCallbackResult{RedirectURL: txn.RedirectURL}
	if providerError != "" {
		msg := providerError
		if providerErrorDescription != "" {
			msg = providerErrorDescription
		}
		return result, fmt.Errorf("%s", msg)
	}
	if strings.TrimSpace(code) == "" {
		return result, fmt.Errorf("missing authorization code")
	}

	var authResp *models.AuthResponse
	switch providerKey {
	case "github":
		authResp, err = s.completeGitHub(ctx, txn, code, callbackURL, userAgent, ipAddress)
	case s.pocketProviderKey():
		authResp, err = s.completePocket(ctx, txn, code, callbackURL, userAgent, ipAddress)
	default:
		err = fmt.Errorf("unsupported provider %q", providerKey)
	}
	if err != nil {
		return result, err
	}
	result.Auth = authResp
	return result, nil
}

func (s *ExternalAuthService) completeGitHub(
	ctx context.Context,
	txn *models.AuthTransaction,
	code, callbackURL, userAgent, ipAddress string,
) (*models.AuthResponse, error) {
	form := url.Values{}
	form.Set("client_id", strings.TrimSpace(s.cfg.GithubClientID))
	form.Set("client_secret", strings.TrimSpace(s.cfg.GithubClientSecret))
	form.Set("code", code)
	form.Set("redirect_uri", callbackURL)
	form.Set("code_verifier", txn.CodeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("github token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token githubTokenResponse
	if err := s.doJSON(req, &token); err != nil {
		return nil, err
	}
	if token.Error != "" {
		return nil, fmt.Errorf("github authorization failed: %s", token.Error)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("github did not return an access token")
	}

	profile, err := s.fetchGitHubProfile(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	email, err := s.fetchVerifiedGitHubEmail(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	if email == "" {
		return nil, fmt.Errorf("verify your email with the provider first")
	}

	displayName := strings.TrimSpace(profile.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(profile.Login)
	}
	subject := fmt.Sprintf("%d", profile.ID)
	now := time.Now().UTC()
	user, binding, err := s.upsertExternalIdentity(ctx, models.ExternalIdentityUpsert{
		ProviderKey:    "github",
		Subject:        subject,
		Email:          normalizeEmail(email),
		EmailVerified:  true,
		DisplayName:    displayName,
		AvatarURL:      strings.TrimSpace(profile.AvatarURL),
		Language:       externalAuthDefaultLang,
		Timezone:       externalAuthDefaultTZ,
		SlugCandidates: []string{profile.Login, displayName},
		ProfileData: map[string]interface{}{
			"id":         profile.ID,
			"login":      profile.Login,
			"name":       profile.Name,
			"avatar_url": profile.AvatarURL,
		},
	}, now)
	if err != nil {
		return nil, err
	}
	if binding != nil && binding.Issuer != "" {
		return nil, fmt.Errorf("github identity issuer mismatch")
	}
	return s.authService.IssueSession(ctx, user, userAgent, ipAddress)
}

func (s *ExternalAuthService) completePocket(
	ctx context.Context,
	txn *models.AuthTransaction,
	code, callbackURL, userAgent, ipAddress string,
) (*models.AuthResponse, error) {
	metadata, err := s.loadPocketMetadata(ctx)
	if err != nil {
		return nil, err
	}
	oauthCfg := oauth2.Config{
		ClientID:     strings.TrimSpace(s.cfg.PocketClientID),
		ClientSecret: strings.TrimSpace(s.cfg.PocketClientSecret),
		RedirectURL:  callbackURL,
		Scopes:       s.pocketScopes(),
		Endpoint: oauth2.Endpoint{
			AuthURL:  metadata.AuthorizationEndpoint,
			TokenURL: metadata.TokenEndpoint,
		},
	}
	token, err := oauthCfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", txn.CodeVerifier))
	if err != nil {
		return nil, fmt.Errorf("pocket id token exchange failed: %w", err)
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if strings.TrimSpace(rawIDToken) == "" {
		return nil, fmt.Errorf("pocket id did not return an id_token")
	}

	verifierConfig := &oidc.Config{ClientID: strings.TrimSpace(s.cfg.PocketClientID)}
	if len(metadata.IDTokenSigningAlgs) > 0 {
		verifierConfig.SupportedSigningAlgs = metadata.IDTokenSigningAlgs
	}
	keySet := oidc.NewRemoteKeySet(ctx, metadata.JWKSURI)
	verifier := oidc.NewVerifier(metadata.Issuer, keySet, verifierConfig)
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("pocket id id_token verification failed: %w", err)
	}
	if idToken.Nonce != txn.Nonce {
		return nil, fmt.Errorf("invalid nonce")
	}
	if idToken.AccessTokenHash != "" {
		if err := idToken.VerifyAccessToken(token.AccessToken); err != nil {
			return nil, fmt.Errorf("pocket id access token hash verification failed: %w", err)
		}
	}

	var claims oidcProfileClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("decode id_token claims: %w", err)
	}
	if len(idToken.Audience) > 1 && claims.Azp != strings.TrimSpace(s.cfg.PocketClientID) {
		return nil, fmt.Errorf("invalid azp claim")
	}
	if !idToken.IssuedAt.IsZero() && idToken.IssuedAt.After(time.Now().UTC().Add(externalAuthAllowedSkew)) {
		return nil, fmt.Errorf("id_token issued in the future")
	}
	if claims.NotBefore != 0 {
		nbf := time.Unix(claims.NotBefore, 0)
		if nbf.After(time.Now().UTC().Add(externalAuthAllowedSkew)) {
			return nil, fmt.Errorf("id_token is not yet valid")
		}
	}
	if claims.TokenUse != "" && claims.TokenUse != "id" {
		return nil, fmt.Errorf("unexpected token_use %q", claims.TokenUse)
	}

	userInfoClaims, err := s.fetchPocketUserInfo(ctx, metadata, token.AccessToken)
	if err != nil {
		return nil, err
	}
	if userInfoClaims != nil && userInfoClaims.Subject != "" && userInfoClaims.Subject != idToken.Subject {
		return nil, fmt.Errorf("userinfo sub does not match id_token sub")
	}

	mergedClaims := mergeOIDCClaims(claims, userInfoClaims)
	emailVerified := mergedClaims.EmailVerified != nil && *mergedClaims.EmailVerified
	email := normalizeEmail(mergedClaims.Email)
	if email == "" || !emailVerified {
		return nil, fmt.Errorf("verify your email with the provider first")
	}

	displayName := strings.TrimSpace(mergedClaims.Name)
	if displayName == "" {
		displayName = firstNonEmptyValue(mergedClaims.PreferredUsername, mergedClaims.Nickname, email)
	}
	now := time.Now().UTC()
	user, binding, err := s.upsertExternalIdentity(ctx, models.ExternalIdentityUpsert{
		ProviderKey:   s.pocketProviderKey(),
		Issuer:        idToken.Issuer,
		Subject:       idToken.Subject,
		Email:         email,
		EmailVerified: true,
		DisplayName:   displayName,
		AvatarURL:     strings.TrimSpace(mergedClaims.Picture),
		Timezone:      firstNonEmptyValue(strings.TrimSpace(mergedClaims.Zoneinfo), externalAuthDefaultTZ),
		Language:      normalizeLocale(mergedClaims.Locale),
		SlugCandidates: []string{
			mergedClaims.PreferredUsername,
			mergedClaims.Nickname,
			mergedClaims.Name,
		},
		ProfileData: map[string]interface{}{
			"sub":                idToken.Subject,
			"issuer":             idToken.Issuer,
			"email":              email,
			"email_verified":     true,
			"name":               mergedClaims.Name,
			"preferred_username": mergedClaims.PreferredUsername,
			"nickname":           mergedClaims.Nickname,
			"picture":            mergedClaims.Picture,
			"locale":             mergedClaims.Locale,
			"zoneinfo":           mergedClaims.Zoneinfo,
		},
	}, now)
	if err != nil {
		return nil, err
	}
	if binding != nil && binding.Issuer != "" && binding.Issuer != idToken.Issuer {
		return nil, fmt.Errorf("provider issuer mismatch")
	}
	return s.authService.IssueSession(ctx, user, userAgent, ipAddress)
}

func (s *ExternalAuthService) fetchGitHubProfile(ctx context.Context, accessToken string) (*githubProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("github profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	var profile githubProfile
	if err := s.doJSON(req, &profile); err != nil {
		return nil, err
	}
	if profile.ID == 0 {
		return nil, fmt.Errorf("github profile did not include a stable user id")
	}
	return &profile, nil
}

func (s *ExternalAuthService) fetchVerifiedGitHubEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", fmt.Errorf("github email request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	var emails []githubEmail
	if err := s.doJSON(req, &emails); err != nil {
		return "", err
	}
	slices.SortStableFunc(emails, func(a, b githubEmail) int {
		if a.Verified != b.Verified {
			if a.Verified {
				return -1
			}
			return 1
		}
		if a.Primary != b.Primary {
			if a.Primary {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Email, b.Email)
	})
	for _, email := range emails {
		if email.Verified && strings.TrimSpace(email.Email) != "" {
			return email.Email, nil
		}
	}
	return "", nil
}

func (s *ExternalAuthService) fetchPocketUserInfo(ctx context.Context, metadata *pocketProviderMetadata, accessToken string) (*oidcProfileClaims, error) {
	if metadata == nil || strings.TrimSpace(metadata.UserInfoEndpoint) == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadata.UserInfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("pocket id userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	var claims oidcProfileClaims
	if err := s.doJSON(req, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func (s *ExternalAuthService) pocketAuthorizationURL(ctx context.Context, state, verifier, nonce, callbackURL string) (string, error) {
	metadata, err := s.loadPocketMetadata(ctx)
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("client_id", strings.TrimSpace(s.cfg.PocketClientID))
	values.Set("redirect_uri", callbackURL)
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(s.pocketScopes(), " "))
	values.Set("state", state)
	values.Set("nonce", nonce)
	values.Set("code_challenge", pkceCodeChallenge(verifier))
	values.Set("code_challenge_method", "S256")
	return metadata.AuthorizationEndpoint + "?" + values.Encode(), nil
}

func (s *ExternalAuthService) githubAuthorizationURL(state, verifier, callbackURL string) string {
	values := url.Values{}
	values.Set("client_id", strings.TrimSpace(s.cfg.GithubClientID))
	values.Set("redirect_uri", callbackURL)
	values.Set("scope", "read:user user:email")
	values.Set("state", state)
	values.Set("code_challenge", pkceCodeChallenge(verifier))
	values.Set("code_challenge_method", "S256")
	return "https://github.com/login/oauth/authorize?" + values.Encode()
}

func (s *ExternalAuthService) loadPocketMetadata(ctx context.Context) (*pocketProviderMetadata, error) {
	s.pocketMu.RLock()
	cached := s.pocketMetadata
	s.pocketMu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	discoveryURL := strings.TrimSpace(s.cfg.PocketDiscoveryURL)
	if discoveryURL == "" {
		issuer := strings.TrimRight(strings.TrimSpace(s.cfg.PocketIssuer), "/")
		if issuer == "" {
			return nil, fmt.Errorf("pocket id issuer is not configured")
		}
		discoveryURL = issuer + "/.well-known/openid-configuration"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pocket id discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	var metadata pocketProviderMetadata
	if err := s.doJSON(req, &metadata); err != nil {
		return nil, err
	}
	metadata.Issuer = strings.TrimRight(strings.TrimSpace(metadata.Issuer), "/")
	configuredIssuer := strings.TrimRight(strings.TrimSpace(s.cfg.PocketIssuer), "/")
	if configuredIssuer != "" && metadata.Issuer != configuredIssuer {
		return nil, fmt.Errorf("pocket id discovery issuer mismatch")
	}
	if metadata.Issuer == "" || metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" || metadata.JWKSURI == "" {
		return nil, fmt.Errorf("pocket id discovery metadata is incomplete")
	}

	s.pocketMu.Lock()
	s.pocketMetadata = &metadata
	s.pocketMu.Unlock()
	return &metadata, nil
}

func (s *ExternalAuthService) createAuthTransaction(ctx context.Context, txn models.AuthTransaction) error {
	if s.repo != nil {
		if err := s.repo.CreateAuthTransaction(ctx, txn); err != nil {
			return fmt.Errorf("create auth transaction: %w", err)
		}
		return nil
	}
	if s.db == nil {
		return fmt.Errorf("auth transaction storage is not configured")
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO auth_transactions (id, provider_key, state, nonce, code_verifier, redirect_url, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		txn.ID, txn.ProviderKey, txn.State, txn.Nonce, txn.CodeVerifier, txn.RedirectURL, txn.ExpiresAt, txn.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create auth transaction: %w", err)
	}
	return nil
}

func (s *ExternalAuthService) consumeAuthTransaction(ctx context.Context, providerKey, state string, now time.Time) (*models.AuthTransaction, error) {
	if s.repo != nil {
		txn, err := s.repo.ConsumeAuthTransaction(ctx, providerKey, state, now)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("invalid or expired authentication state")
			}
			return nil, fmt.Errorf("consume auth transaction: %w", err)
		}
		return txn, nil
	}
	if s.db == nil {
		return nil, fmt.Errorf("auth transaction storage is not configured")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("consume auth transaction: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		txn        models.AuthTransaction
		consumedAt *time.Time
	)
	err = tx.QueryRow(ctx,
		`SELECT id, provider_key, state, nonce, code_verifier, redirect_url, expires_at, consumed_at, created_at
		   FROM auth_transactions
		  WHERE provider_key = $1 AND state = $2`,
		providerKey, state,
	).Scan(&txn.ID, &txn.ProviderKey, &txn.State, &txn.Nonce, &txn.CodeVerifier, &txn.RedirectURL, &txn.ExpiresAt, &consumedAt, &txn.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("invalid or expired authentication state")
	}
	if err != nil {
		return nil, fmt.Errorf("consume auth transaction: lookup: %w", err)
	}
	if consumedAt != nil || now.After(txn.ExpiresAt) {
		return nil, fmt.Errorf("invalid or expired authentication state")
	}
	tag, err := tx.Exec(ctx,
		`UPDATE auth_transactions
		    SET consumed_at = $1
		  WHERE id = $2 AND consumed_at IS NULL`,
		now, txn.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("consume auth transaction: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("invalid or expired authentication state")
	}
	consumedAt = &now
	txn.ConsumedAt = consumedAt
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("consume auth transaction: commit: %w", err)
	}
	return &txn, nil
}

func (s *ExternalAuthService) upsertExternalIdentity(ctx context.Context, input models.ExternalIdentityUpsert, now time.Time) (*models.User, *models.AuthBinding, error) {
	if s.repo != nil {
		user, binding, err := s.repo.UpsertExternalIdentity(ctx, input, now)
		if err != nil {
			return nil, nil, fmt.Errorf("upsert external identity: %w", err)
		}
		return user, binding, nil
	}
	if s.db == nil {
		return nil, nil, fmt.Errorf("external identity storage is not configured")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("upsert external identity: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	providerKey := strings.TrimSpace(input.ProviderKey)
	subject := strings.TrimSpace(input.Subject)
	if providerKey == "" || subject == "" {
		return nil, nil, fmt.Errorf("provider_key and subject are required")
	}
	profileJSON, err := json.Marshal(defaultMap(input.ProfileData))
	if err != nil {
		return nil, nil, fmt.Errorf("marshal provider profile: %w", err)
	}

	binding, user, err := loadAuthBindingAndUserPG(ctx, tx, providerKey, subject)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	if binding != nil {
		if binding.Issuer != "" && strings.TrimSpace(input.Issuer) != "" && binding.Issuer != strings.TrimSpace(input.Issuer) {
			return nil, nil, fmt.Errorf("provider issuer mismatch")
		}
		applyExternalProfile(user, input, now)
		_, err = tx.Exec(ctx,
			`UPDATE users
			    SET display_name = $1,
			        email = $2,
			        avatar_url = $3,
			        timezone = $4,
			        language = $5,
			        updated_at = $6
			  WHERE id = $7`,
			user.DisplayName, user.Email, user.AvatarURL, user.Timezone, user.Language, user.UpdatedAt, user.ID,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("update external user: %w", err)
		}
		_, err = tx.Exec(ctx,
			`UPDATE auth_bindings
			    SET provider = $1,
			        provider_id = $2,
			        provider_key = $3,
			        issuer = $4,
			        subject = $5,
			        email = $6,
			        email_verified = $7,
			        provider_data = $8,
			        last_login_at = $9
			  WHERE id = $10`,
			providerKey, subject, providerKey, strings.TrimSpace(input.Issuer), subject, normalizeEmail(input.Email), input.EmailVerified, profileJSON, now, binding.ID,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("update auth binding: %w", err)
		}
		binding.Provider = providerKey
		binding.ProviderID = subject
		binding.ProviderKey = providerKey
		binding.Subject = subject
		binding.Issuer = strings.TrimSpace(input.Issuer)
		binding.Email = normalizeEmail(input.Email)
		binding.EmailVerified = input.EmailVerified
		binding.ProviderData = defaultMap(input.ProfileData)
		binding.LastLoginAt = &now
		if err := tx.Commit(ctx); err != nil {
			return nil, nil, fmt.Errorf("upsert external identity: commit update: %w", err)
		}
		return user, binding, nil
	}

	slug, err := reserveUniqueSlugPG(ctx, tx, input.SlugCandidates)
	if err != nil {
		return nil, nil, err
	}
	user = &models.User{
		ID:          uuid.New(),
		Slug:        slug,
		DisplayName: resolveDisplayName(input.DisplayName, slug),
		Email:       normalizeEmail(input.Email),
		AvatarURL:   strings.TrimSpace(input.AvatarURL),
		Timezone:    defaultString(strings.TrimSpace(input.Timezone), externalAuthDefaultTZ),
		Language:    defaultString(strings.TrimSpace(input.Language), externalAuthDefaultLang),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, '', $6, $7, $8, $8)`,
		user.ID, user.Slug, user.DisplayName, user.Email, user.AvatarURL, user.Timezone, user.Language, now,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create external user: %w", err)
	}
	binding = &models.AuthBinding{
		ID:            uuid.New(),
		UserID:        user.ID,
		Provider:      providerKey,
		ProviderID:    subject,
		ProviderKey:   providerKey,
		Issuer:        strings.TrimSpace(input.Issuer),
		Subject:       subject,
		Email:         user.Email,
		EmailVerified: input.EmailVerified,
		ProviderData:  defaultMap(input.ProfileData),
		LastLoginAt:   &now,
		CreatedAt:     now,
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_key, issuer, subject, email, email_verified, provider_data, last_login_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)`,
		binding.ID, binding.UserID, binding.Provider, binding.ProviderID, binding.ProviderKey, binding.Issuer, binding.Subject, binding.Email, binding.EmailVerified, profileJSON, now,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create auth binding: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, user_id, name, role_type, config, allowed_paths, allowed_vault_scopes, lifecycle, created_at)
		 VALUES ($1, $2, 'assistant', 'assistant', '{}', ARRAY['/'], ARRAY[]::TEXT[], 'permanent', $3)
		 ON CONFLICT DO NOTHING`,
		uuid.New(), user.ID, now,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create default role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("upsert external identity: commit create: %w", err)
	}
	return user, binding, nil
}

func loadAuthBindingAndUserPG(ctx context.Context, tx pgx.Tx, providerKey, subject string) (*models.AuthBinding, *models.User, error) {
	var (
		binding            models.AuthBinding
		user               models.User
		profileJSON        []byte
		bindingLastLoginAt *time.Time
		userCreatedAt      time.Time
		userUpdatedAt      time.Time
		bindingCreatedAt   time.Time
	)
	err := tx.QueryRow(ctx,
		`SELECT ab.id,
		        ab.user_id,
		        COALESCE(ab.provider, ''),
		        COALESCE(ab.provider_id, ''),
		        COALESCE(ab.provider_key, ''),
		        COALESCE(ab.issuer, ''),
		        COALESCE(ab.subject, ''),
		        COALESCE(ab.email, ''),
		        COALESCE(ab.email_verified, false),
		        COALESCE(ab.provider_data, '{}'::jsonb),
		        ab.last_login_at,
		        ab.created_at,
		        u.id,
		        u.slug,
		        COALESCE(u.display_name, ''),
		        COALESCE(u.email, ''),
		        COALESCE(u.avatar_url, ''),
		        COALESCE(u.bio, ''),
		        COALESCE(u.timezone, 'UTC'),
		        COALESCE(u.language, 'en'),
		        u.created_at,
		        u.updated_at
		   FROM auth_bindings ab
		   JOIN users u ON u.id = ab.user_id
		  WHERE COALESCE(NULLIF(ab.provider_key, ''), ab.provider) = $1
		    AND COALESCE(NULLIF(ab.subject, ''), ab.provider_id) = $2`,
		providerKey, subject,
	).Scan(
		&binding.ID,
		&binding.UserID,
		&binding.Provider,
		&binding.ProviderID,
		&binding.ProviderKey,
		&binding.Issuer,
		&binding.Subject,
		&binding.Email,
		&binding.EmailVerified,
		&profileJSON,
		&bindingLastLoginAt,
		&bindingCreatedAt,
		&user.ID,
		&user.Slug,
		&user.DisplayName,
		&user.Email,
		&user.AvatarURL,
		&user.Bio,
		&user.Timezone,
		&user.Language,
		&userCreatedAt,
		&userUpdatedAt,
	)
	if err != nil {
		return nil, nil, err
	}
	binding.LastLoginAt = bindingLastLoginAt
	binding.CreatedAt = bindingCreatedAt
	binding.ProviderData = decodeJSONBytes(profileJSON)
	user.CreatedAt = userCreatedAt
	user.UpdatedAt = userUpdatedAt
	return &binding, &user, nil
}

func reserveUniqueSlugPG(ctx context.Context, tx pgx.Tx, candidates []string) (string, error) {
	baseCandidates := normalizeSlugCandidates(candidates)
	for _, candidate := range baseCandidates {
		slug, err := findAvailableSlugPG(ctx, tx, candidate)
		if err != nil {
			return "", err
		}
		if slug != "" {
			return slug, nil
		}
	}
	return "", fmt.Errorf("unable to generate unique slug")
}

func findAvailableSlugPG(ctx context.Context, tx pgx.Tx, base string) (string, error) {
	for suffix := 0; suffix < 100; suffix++ {
		slug := base
		if suffix > 0 {
			slug = fmt.Sprintf("%s-%d", base, suffix+1)
		}
		var existing uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE slug = $1`, slug).Scan(&existing)
		if err == pgx.ErrNoRows {
			return slug, nil
		}
		if err != nil {
			return "", fmt.Errorf("check slug availability: %w", err)
		}
	}
	return "", nil
}

func applyExternalProfile(user *models.User, input models.ExternalIdentityUpsert, now time.Time) {
	if user == nil {
		return
	}
	if displayName := strings.TrimSpace(input.DisplayName); displayName != "" {
		user.DisplayName = displayName
	}
	if email := normalizeEmail(input.Email); email != "" {
		user.Email = email
	}
	if avatar := strings.TrimSpace(input.AvatarURL); avatar != "" {
		user.AvatarURL = avatar
	}
	if timezone := strings.TrimSpace(input.Timezone); timezone != "" {
		user.Timezone = timezone
	}
	if language := strings.TrimSpace(input.Language); language != "" {
		user.Language = language
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Slug
	}
	if user.Timezone == "" {
		user.Timezone = externalAuthDefaultTZ
	}
	if user.Language == "" {
		user.Language = externalAuthDefaultLang
	}
	user.UpdatedAt = now
}

func mergeOIDCClaims(primary oidcProfileClaims, fallback *oidcProfileClaims) oidcProfileClaims {
	if fallback == nil {
		return primary
	}
	merged := primary
	if merged.Subject == "" {
		merged.Subject = fallback.Subject
	}
	if merged.Email == "" {
		merged.Email = fallback.Email
	}
	if merged.PreferredUsername == "" {
		merged.PreferredUsername = fallback.PreferredUsername
	}
	if merged.Name == "" {
		merged.Name = fallback.Name
	}
	if merged.Nickname == "" {
		merged.Nickname = fallback.Nickname
	}
	if merged.Picture == "" {
		merged.Picture = fallback.Picture
	}
	if merged.Locale == "" {
		merged.Locale = fallback.Locale
	}
	if merged.Zoneinfo == "" {
		merged.Zoneinfo = fallback.Zoneinfo
	}
	if merged.EmailVerified == nil {
		merged.EmailVerified = fallback.EmailVerified
	}
	return merged
}

func normalizeSlugCandidates(candidates []string) []string {
	seen := map[string]struct{}{}
	var normalized []string
	for _, candidate := range candidates {
		slug := slugify(candidate)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		normalized = append(normalized, slug)
	}
	if len(normalized) == 0 {
		return []string{"user"}
	}
	return normalized
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = slugSanitizer.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	if len(value) > 64 {
		value = strings.Trim(value[:64], "-_")
	}
	if len(value) < externalAuthMinSlugLength {
		return ""
	}
	return value
}

func resolveDisplayName(displayName, slug string) string {
	if strings.TrimSpace(displayName) != "" {
		return strings.TrimSpace(displayName)
	}
	return slug
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return externalAuthDefaultLang
	}
	return locale
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}

func decodeJSONBytes(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return map[string]interface{}{}
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return map[string]interface{}{}
	}
	return decoded
}

func randomURLSafeString(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pkceCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *ExternalAuthService) doJSON(req *http.Request, out interface{}) error {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s failed: %w", req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return fmt.Errorf("read response body: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("%s %s failed: %s", req.Method, req.URL.String(), message)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response from %s: %w", req.URL.String(), err)
	}
	return nil
}

func (s *ExternalAuthService) githubEnabled() bool {
	return s != nil && s.cfg != nil && strings.TrimSpace(s.cfg.GithubClientID) != "" && strings.TrimSpace(s.cfg.GithubClientSecret) != ""
}

func (s *ExternalAuthService) pocketEnabled() bool {
	return s != nil && s.cfg != nil &&
		strings.TrimSpace(s.cfg.PocketProviderID) != "" &&
		strings.TrimSpace(s.cfg.PocketClientID) != "" &&
		strings.TrimSpace(s.cfg.PocketClientSecret) != "" &&
		(strings.TrimSpace(s.cfg.PocketIssuer) != "" || strings.TrimSpace(s.cfg.PocketDiscoveryURL) != "")
}

func (s *ExternalAuthService) pocketProviderKey() string {
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.PocketProviderID) == "" {
		return "pocket"
	}
	return strings.TrimSpace(s.cfg.PocketProviderID)
}

func (s *ExternalAuthService) pocketScopes() []string {
	if s == nil || s.cfg == nil || len(s.cfg.PocketScopes) == 0 {
		return []string{"openid", "profile", "email"}
	}
	return s.cfg.PocketScopes
}
