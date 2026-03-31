package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/auth"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost          = 12
	accessTokenExpiry   = 24 * time.Hour
	refreshTokenExpiry  = 30 * 24 * time.Hour
	accessTokenSeconds  = 86400 // 24 hours
	refreshTokenBytes   = 64
)

var emailRegexp = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type AuthService struct {
	db                 *pgxpool.Pool
	jwtSecret          string
	gitHubClientID     string
	gitHubClientSecret string
}

func NewAuthService(db *pgxpool.Pool, jwtSecret, ghClientID, ghClientSecret string) *AuthService {
	return &AuthService{
		db:                 db,
		jwtSecret:          jwtSecret,
		gitHubClientID:     ghClientID,
		gitHubClientSecret: ghClientSecret,
	}
}

// Register creates a new user with email/password credentials.
func (s *AuthService) Register(ctx context.Context, req models.RegisterRequest) (*models.AuthResponse, error) {
	// Validate email
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRegexp.MatchString(email) {
		return nil, fmt.Errorf("invalid email format")
	}

	// Validate password
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	// Validate slug
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = slug
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("register: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	userID := uuid.New()

	// Check if email already exists
	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM credentials WHERE email = $1`, email).Scan(&existing)
	if err == nil {
		return nil, fmt.Errorf("email already registered")
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("register: check email: %w", err)
	}

	// Check if slug already exists
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE slug = $1`, slug).Scan(&existing)
	if err == nil {
		return nil, fmt.Errorf("slug already taken")
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("register: check slug: %w", err)
	}

	// Create user
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, slug, display_name, email, timezone, language, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'UTC', 'en', $5, $5)`,
		userID, slug, displayName, email, now)
	if err != nil {
		return nil, fmt.Errorf("register: insert user: %w", err)
	}

	// Create credentials
	credID := uuid.New()
	_, err = tx.Exec(ctx,
		`INSERT INTO credentials (id, user_id, email, password_hash, email_verified, login_count, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, false, 0, $5, $5)`,
		credID, userID, email, string(hash), now)
	if err != nil {
		return nil, fmt.Errorf("register: insert credentials: %w", err)
	}

	// Create default 'assistant' role
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, user_id, name, system_prompt, created_at)
		 VALUES ($1, $2, 'assistant', 'You are a helpful assistant.', $3)
		 ON CONFLICT DO NOTHING`,
		uuid.New(), userID, now)
	if err != nil {
		return nil, fmt.Errorf("register: insert default role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("register: commit: %w", err)
	}

	// Generate tokens
	user := models.User{
		ID:          userID,
		Slug:        slug,
		DisplayName: displayName,
		Email:       email,
		Timezone:    "UTC",
		Language:    "en",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return s.generateAuthResponse(ctx, &user, "", "")
}

// Login authenticates a user with email/password.
func (s *AuthService) Login(ctx context.Context, req models.LoginRequest, userAgent, ipAddress string) (*models.AuthResponse, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if req.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	// Look up credentials
	var cred models.Credentials
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, email, password_hash, email_verified, login_count
		 FROM credentials WHERE email = $1`, email).
		Scan(&cred.ID, &cred.UserID, &cred.Email, &cred.PasswordHash, &cred.EmailVerified, &cred.LoginCount)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("invalid email or password")
	}
	if err != nil {
		return nil, fmt.Errorf("login: query credentials: %w", err)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Update last_login_at and login_count
	now := time.Now().UTC()
	_, err = s.db.Exec(ctx,
		`UPDATE credentials SET last_login_at = $1, login_count = login_count + 1, updated_at = $1 WHERE id = $2`,
		now, cred.ID)
	if err != nil {
		return nil, fmt.Errorf("login: update login stats: %w", err)
	}

	// Get user
	var user models.User
	err = s.db.QueryRow(ctx,
		`SELECT id, slug, display_name, COALESCE(email, ''), COALESCE(avatar_url, ''), timezone, language, created_at, updated_at
		 FROM users WHERE id = $1`, cred.UserID).
		Scan(&user.ID, &user.Slug, &user.DisplayName, &user.Email, &user.AvatarURL, &user.Timezone, &user.Language, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("login: get user: %w", err)
	}

	return s.generateAuthResponse(ctx, &user, userAgent, ipAddress)
}

// RefreshToken validates a refresh token and issues a new token pair.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken, userAgent, ipAddress string) (*models.AuthResponse, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	tokenHash := hashToken(refreshToken)

	var session models.Session
	var userID uuid.UUID
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, expires_at FROM sessions WHERE refresh_token_hash = $1`, tokenHash).
		Scan(&session.ID, &userID, &session.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("invalid refresh token")
	}
	if err != nil {
		return nil, fmt.Errorf("refresh: query session: %w", err)
	}

	if time.Now().After(session.ExpiresAt) {
		// Delete expired session
		s.db.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID)
		return nil, fmt.Errorf("refresh token expired")
	}

	// Delete old session (rotation)
	_, err = s.db.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, session.ID)
	if err != nil {
		return nil, fmt.Errorf("refresh: delete old session: %w", err)
	}

	// Get user
	var user models.User
	err = s.db.QueryRow(ctx,
		`SELECT id, slug, display_name, COALESCE(email, ''), COALESCE(avatar_url, ''), timezone, language, created_at, updated_at
		 FROM users WHERE id = $1`, userID).
		Scan(&user.ID, &user.Slug, &user.DisplayName, &user.Email, &user.AvatarURL, &user.Timezone, &user.Language, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("refresh: get user: %w", err)
	}

	return s.generateAuthResponse(ctx, &user, userAgent, ipAddress)
}

// Logout invalidates a refresh token by deleting the session.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	tokenHash := hashToken(refreshToken)
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE refresh_token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("logout: delete session: %w", err)
	}
	return nil
}

// GitHubLogin exchanges a GitHub OAuth code and creates/gets a user.
func (s *AuthService) GitHubLogin(ctx context.Context, code, userAgent, ipAddress string) (*models.AuthResponse, error) {
	if s.gitHubClientID == "" || s.gitHubClientSecret == "" {
		return nil, fmt.Errorf("github oauth not configured")
	}

	ghUser, err := auth.ExchangeGitHubCode(ctx, s.gitHubClientID, s.gitHubClientSecret, code)
	if err != nil {
		return nil, fmt.Errorf("github exchange failed: %w", err)
	}

	displayName := ghUser.Name
	if displayName == "" {
		displayName = ghUser.Login
	}

	ghID := strconv.Itoa(ghUser.ID)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("github login: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	now := time.Now().UTC()

	err = tx.QueryRow(ctx,
		`SELECT user_id FROM auth_bindings WHERE provider = 'github' AND provider_id = $1`, ghID).
		Scan(&userID)

	if err == pgx.ErrNoRows {
		// New user
		userID = uuid.New()
		email := ghUser.Email
		avatarURL := fmt.Sprintf("https://avatars.githubusercontent.com/u/%d", ghUser.ID)

		_, err = tx.Exec(ctx,
			`INSERT INTO users (id, slug, display_name, email, avatar_url, timezone, language, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, 'UTC', 'en', $6, $6)`,
			userID, ghUser.Login, displayName, email, avatarURL, now)
		if err != nil {
			return nil, fmt.Errorf("github login: insert user: %w", err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_data, created_at)
			 VALUES ($1, $2, 'github', $3, '{}', $4)`,
			uuid.New(), userID, ghID, now)
		if err != nil {
			return nil, fmt.Errorf("github login: insert binding: %w", err)
		}

		// Create default role
		_, err = tx.Exec(ctx,
			`INSERT INTO roles (id, user_id, name, system_prompt, created_at)
			 VALUES ($1, $2, 'assistant', 'You are a helpful assistant.', $3)
			 ON CONFLICT DO NOTHING`,
			uuid.New(), userID, now)
		if err != nil {
			return nil, fmt.Errorf("github login: insert default role: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("github login: lookup binding: %w", err)
	} else {
		// Existing user: update display name
		_, err = tx.Exec(ctx,
			`UPDATE users SET display_name = $1, updated_at = $2 WHERE id = $3`,
			displayName, now, userID)
		if err != nil {
			return nil, fmt.Errorf("github login: update user: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("github login: commit: %w", err)
	}

	// Load user
	var user models.User
	err = s.db.QueryRow(ctx,
		`SELECT id, slug, display_name, COALESCE(email, ''), COALESCE(avatar_url, ''), timezone, language, created_at, updated_at
		 FROM users WHERE id = $1`, userID).
		Scan(&user.ID, &user.Slug, &user.DisplayName, &user.Email, &user.AvatarURL, &user.Timezone, &user.Language, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("github login: get user: %w", err)
	}

	return s.generateAuthResponse(ctx, &user, userAgent, ipAddress)
}

// ListSessions returns all active sessions for a user.
func (s *AuthService) ListSessions(ctx context.Context, userID uuid.UUID) ([]models.Session, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, user_agent, ip_address, expires_at, created_at
		 FROM sessions WHERE user_id = $1 AND expires_at > NOW() ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var sess models.Session
		var ua, ip *string
		if err := rows.Scan(&sess.ID, &sess.UserID, &ua, &ip, &sess.ExpiresAt, &sess.CreatedAt); err != nil {
			return nil, fmt.Errorf("list sessions: scan: %w", err)
		}
		if ua != nil {
			sess.UserAgent = *ua
		}
		if ip != nil {
			sess.IPAddress = *ip
		}
		sessions = append(sessions, sess)
	}
	if sessions == nil {
		sessions = []models.Session{}
	}
	return sessions, nil
}

// RevokeSession deletes a specific session belonging to the user.
func (s *AuthService) RevokeSession(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM sessions WHERE id = $1 AND user_id = $2`, sessionID, userID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

// generateAuthResponse creates access+refresh tokens and persists the session.
func (s *AuthService) generateAuthResponse(ctx context.Context, user *models.User, userAgent, ipAddress string) (*models.AuthResponse, error) {
	// Generate access token (JWT)
	accessToken, err := auth.GenerateToken(user.ID, user.Slug, s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	// Generate refresh token (random bytes)
	refreshBytes := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(refreshBytes); err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	refreshToken := hex.EncodeToString(refreshBytes)
	refreshHash := hashToken(refreshToken)

	// Store session
	now := time.Now().UTC()
	expiresAt := now.Add(refreshTokenExpiry)
	_, err = s.db.Exec(ctx,
		`INSERT INTO sessions (id, user_id, refresh_token_hash, user_agent, ip_address, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.New(), user.ID, refreshHash, userAgent, ipAddress, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    accessTokenSeconds,
		User:         *user,
	}, nil
}

// ChangePassword validates the old password and updates to a new one.
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error {
	if oldPassword == "" {
		return fmt.Errorf("current password is required")
	}
	if len(newPassword) < 8 {
		return fmt.Errorf("new password must be at least 8 characters")
	}

	// Look up credentials by user_id
	var cred models.Credentials
	err := s.db.QueryRow(ctx,
		`SELECT id, password_hash FROM credentials WHERE user_id = $1`, userID).
		Scan(&cred.ID, &cred.PasswordHash)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("no password credentials found for this account")
	}
	if err != nil {
		return fmt.Errorf("change password: query credentials: %w", err)
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(oldPassword)); err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("change password: hash: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.Exec(ctx,
		`UPDATE credentials SET password_hash = $1, updated_at = $2 WHERE id = $3`,
		string(hash), now, cred.ID)
	if err != nil {
		return fmt.Errorf("change password: update: %w", err)
	}

	return nil
}

// GetProfile returns the user profile for the given user ID.
func (s *AuthService) GetProfile(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	var user models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, slug, display_name, COALESCE(email, ''), COALESCE(avatar_url, ''),
		        COALESCE(bio, ''), timezone, language, created_at, updated_at
		 FROM users WHERE id = $1`, userID).
		Scan(&user.ID, &user.Slug, &user.DisplayName, &user.Email, &user.AvatarURL,
			&user.Bio, &user.Timezone, &user.Language, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &user, nil
}

// UpdateProfile updates the user's display name, bio, timezone, and language.
func (s *AuthService) UpdateProfile(ctx context.Context, userID uuid.UUID, displayName, bio, timezone, language string) (*models.User, error) {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx,
		`UPDATE users SET display_name = $1, bio = $2, timezone = $3, language = $4, updated_at = $5
		 WHERE id = $6`,
		displayName, bio, timezone, language, now, userID)
	if err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return s.GetProfile(ctx, userID)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
