package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OAuthService handles OAuth 2.0 provider operations.
type OAuthService struct {
	db        *pgxpool.Pool
	jwtSecret string
}

// NewOAuthService creates a new OAuthService.
func NewOAuthService(db *pgxpool.Pool, jwtSecret string) *OAuthService {
	return &OAuthService{db: db, jwtSecret: jwtSecret}
}

// RegisterApp creates a new OAuth application and returns it along with the
// plaintext client secret (shown only once).
func (s *OAuthService) RegisterApp(ctx context.Context, userID uuid.UUID, name string, redirectURIs, scopes []string, description, logoURL string) (*models.RegisterOAuthAppResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("oauth.RegisterApp: name is required")
	}
	if len(redirectURIs) == 0 {
		return nil, fmt.Errorf("oauth.RegisterApp: at least one redirect_uri is required")
	}

	clientID, err := generateClientID()
	if err != nil {
		return nil, fmt.Errorf("oauth.RegisterApp: %w", err)
	}
	clientSecret, err := generateClientSecret()
	if err != nil {
		return nil, fmt.Errorf("oauth.RegisterApp: %w", err)
	}
	secretHash := hashString(clientSecret)

	id := uuid.New()
	now := time.Now().UTC()

	_, err = s.db.Exec(ctx,
		`INSERT INTO oauth_apps (id, user_id, name, client_id, client_secret_hash, redirect_uris, scopes, description, logo_url, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, userID, name, clientID, secretHash, redirectURIs, scopes, description, logoURL, now)
	if err != nil {
		return nil, fmt.Errorf("oauth.RegisterApp: %w", err)
	}

	app, err := s.getAppByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &models.RegisterOAuthAppResponse{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		App:          app.ToResponse(),
	}, nil
}

// Authorize creates an authorization code for the given app and user.
// It also creates or updates the grant record.
func (s *OAuthService) Authorize(ctx context.Context, appID, userID uuid.UUID, scopes []string, redirectURI string) (string, error) {
	code, err := generateAuthCode()
	if err != nil {
		return "", fmt.Errorf("oauth.Authorize: %w", err)
	}
	codeHash := hashString(code)
	expiresAt := time.Now().UTC().Add(10 * time.Minute)

	id := uuid.New()
	_, err = s.db.Exec(ctx,
		`INSERT INTO oauth_codes (id, app_id, user_id, code_hash, scopes, redirect_uri, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, appID, userID, codeHash, scopes, redirectURI, expiresAt)
	if err != nil {
		return "", fmt.Errorf("oauth.Authorize: insert code: %w", err)
	}

	// Upsert the grant.
	_, err = s.db.Exec(ctx,
		`INSERT INTO oauth_grants (id, app_id, user_id, scopes)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (app_id, user_id) DO UPDATE SET scopes = $4`,
		uuid.New(), appID, userID, scopes)
	if err != nil {
		return "", fmt.Errorf("oauth.Authorize: upsert grant: %w", err)
	}

	return code, nil
}

// ExchangeCode validates an authorization code and returns a JWT access token.
func (s *OAuthService) ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (*models.OAuthTokenResponse, error) {
	app, err := s.GetAppByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("oauth.ExchangeCode: invalid client_id")
	}

	if hashString(clientSecret) != app.ClientSecretHash {
		return nil, fmt.Errorf("oauth.ExchangeCode: invalid client_secret")
	}

	if !app.IsActive {
		return nil, fmt.Errorf("oauth.ExchangeCode: app is deactivated")
	}

	codeHash := hashString(code)
	var oc models.OAuthCode
	err = s.db.QueryRow(ctx,
		`SELECT id, app_id, user_id, code_hash, scopes, redirect_uri, expires_at, used
		 FROM oauth_codes
		 WHERE code_hash = $1 AND used = false`, codeHash).
		Scan(&oc.ID, &oc.AppID, &oc.UserID, &oc.CodeHash, &oc.Scopes, &oc.RedirectURI, &oc.ExpiresAt, &oc.Used)
	if err != nil {
		return nil, fmt.Errorf("oauth.ExchangeCode: invalid or already-used code")
	}

	if oc.AppID != app.ID {
		return nil, fmt.Errorf("oauth.ExchangeCode: code does not belong to this app")
	}

	if oc.RedirectURI != redirectURI {
		return nil, fmt.Errorf("oauth.ExchangeCode: redirect_uri mismatch")
	}

	if time.Now().UTC().After(oc.ExpiresAt) {
		return nil, fmt.Errorf("oauth.ExchangeCode: code has expired")
	}

	// Mark the code as used.
	_, err = s.db.Exec(ctx,
		`UPDATE oauth_codes SET used = true WHERE id = $1`, oc.ID)
	if err != nil {
		return nil, fmt.Errorf("oauth.ExchangeCode: failed to mark code used: %w", err)
	}

	// Look up the user to get slug for JWT.
	var slug string
	err = s.db.QueryRow(ctx, `SELECT slug FROM users WHERE id = $1`, oc.UserID).Scan(&slug)
	if err != nil {
		return nil, fmt.Errorf("oauth.ExchangeCode: user not found")
	}

	accessToken, err := s.generateOAuthAccessToken(oc.UserID, slug)
	if err != nil {
		return nil, fmt.Errorf("oauth.ExchangeCode: failed to generate token: %w", err)
	}

	scopeStr := strings.Join(oc.Scopes, " ")

	return &models.OAuthTokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   86400,
		Scope:       scopeStr,
	}, nil
}

// ValidateGrant checks if a user has authorized a specific app.
func (s *OAuthService) ValidateGrant(ctx context.Context, userID, appID uuid.UUID) (*models.OAuthGrant, error) {
	var g models.OAuthGrant
	err := s.db.QueryRow(ctx,
		`SELECT id, app_id, user_id, scopes, created_at
		 FROM oauth_grants
		 WHERE user_id = $1 AND app_id = $2`, userID, appID).
		Scan(&g.ID, &g.AppID, &g.UserID, &g.Scopes, &g.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("oauth.ValidateGrant: no grant found")
	}
	return &g, nil
}

// ListApps returns all OAuth apps registered by the given user.
func (s *OAuthService) ListApps(ctx context.Context, userID uuid.UUID) ([]models.OAuthApp, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, client_id, client_secret_hash, redirect_uris, scopes, description, COALESCE(logo_url, ''), is_active, created_at
		 FROM oauth_apps
		 WHERE user_id = $1
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("oauth.ListApps: %w", err)
	}
	defer rows.Close()

	var apps []models.OAuthApp
	for rows.Next() {
		var a models.OAuthApp
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.ClientID, &a.ClientSecretHash,
			&a.RedirectURIs, &a.Scopes, &a.Description, &a.LogoURL, &a.IsActive, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("oauth.ListApps: scan: %w", err)
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

// DeleteApp removes an OAuth app, cascading to codes and grants.
func (s *OAuthService) DeleteApp(ctx context.Context, userID, appID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM oauth_apps WHERE id = $1 AND user_id = $2`, appID, userID)
	if err != nil {
		return fmt.Errorf("oauth.DeleteApp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("oauth.DeleteApp: app not found or not owned by user")
	}
	return nil
}

// ListGrants returns all apps that a user has authorized, along with app details.
func (s *OAuthService) ListGrants(ctx context.Context, userID uuid.UUID) ([]models.OAuthGrantResponse, error) {
	rows, err := s.db.Query(ctx,
		`SELECT g.id, g.scopes, g.created_at,
		        a.id, a.name, a.client_id, a.redirect_uris, a.scopes, a.description, COALESCE(a.logo_url, ''), a.is_active, a.created_at
		 FROM oauth_grants g
		 JOIN oauth_apps a ON a.id = g.app_id
		 WHERE g.user_id = $1
		 ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("oauth.ListGrants: %w", err)
	}
	defer rows.Close()

	var grants []models.OAuthGrantResponse
	for rows.Next() {
		var gr models.OAuthGrantResponse
		var app models.OAuthAppResponse
		if err := rows.Scan(&gr.ID, &gr.Scopes, &gr.CreatedAt,
			&app.ID, &app.Name, &app.ClientID, &app.RedirectURIs, &app.Scopes, &app.Description, &app.LogoURL, &app.IsActive, &app.CreatedAt); err != nil {
			return nil, fmt.Errorf("oauth.ListGrants: scan: %w", err)
		}
		gr.App = app
		grants = append(grants, gr)
	}
	return grants, rows.Err()
}

// RevokeGrant removes a user's authorization for an app.
func (s *OAuthService) RevokeGrant(ctx context.Context, userID, grantID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM oauth_grants WHERE id = $1 AND user_id = $2`, grantID, userID)
	if err != nil {
		return fmt.Errorf("oauth.RevokeGrant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("oauth.RevokeGrant: grant not found")
	}
	return nil
}

// GetAppByClientID looks up an app by its public client_id.
func (s *OAuthService) GetAppByClientID(ctx context.Context, clientID string) (*models.OAuthApp, error) {
	var a models.OAuthApp
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, client_id, client_secret_hash, redirect_uris, scopes, description, COALESCE(logo_url, ''), is_active, created_at
		 FROM oauth_apps
		 WHERE client_id = $1`, clientID).
		Scan(&a.ID, &a.UserID, &a.Name, &a.ClientID, &a.ClientSecretHash,
			&a.RedirectURIs, &a.Scopes, &a.Description, &a.LogoURL, &a.IsActive, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("oauth.GetAppByClientID: %w", err)
	}
	return &a, nil
}

// getAppByID fetches an app by primary key (internal).
func (s *OAuthService) getAppByID(ctx context.Context, id uuid.UUID) (*models.OAuthApp, error) {
	var a models.OAuthApp
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, client_id, client_secret_hash, redirect_uris, scopes, description, COALESCE(logo_url, ''), is_active, created_at
		 FROM oauth_apps
		 WHERE id = $1`, id).
		Scan(&a.ID, &a.UserID, &a.Name, &a.ClientID, &a.ClientSecretHash,
			&a.RedirectURIs, &a.Scopes, &a.Description, &a.LogoURL, &a.IsActive, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("oauth.getAppByID: %w", err)
	}
	return &a, nil
}

// ValidateRedirectURI checks if the given redirect URI is in the app's allowed list.
func (s *OAuthService) ValidateRedirectURI(app *models.OAuthApp, uri string) bool {
	for _, allowed := range app.RedirectURIs {
		if allowed == uri {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func generateClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: failed to generate client ID: %w", err)
	}
	return "ahc_" + hex.EncodeToString(b), nil
}

func generateClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: failed to generate client secret: %w", err)
	}
	return "ahs_" + hex.EncodeToString(b), nil
}

func generateAuthCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: failed to generate auth code: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// oauthJWTClaims are the JWT claims for OAuth access tokens.
type oauthJWTClaims struct {
	UserID string `json:"user_id"`
	Slug   string `json:"slug"`
	jwt.RegisteredClaims
}

// generateOAuthAccessToken creates a 24-hour JWT for OAuth access tokens.
func (s *OAuthService) generateOAuthAccessToken(userID uuid.UUID, slug string) (string, error) {
	claims := oauthJWTClaims{
		UserID: userID.String(),
		Slug:   slug,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}
