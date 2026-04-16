package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/google/uuid"
)

type UserRepo struct {
	Store *Store
}

func NewUserRepo(store *Store) services.UserRepo {
	return &UserRepo{Store: store}
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return r.Store.UserByID(ctx, id)
}

func (r *UserRepo) GetBySlug(ctx context.Context, slug string) (*models.User, error) {
	return r.Store.UserBySlug(ctx, slug)
}

func (r *UserRepo) GetAuthBinding(ctx context.Context, provider string, providerID string) (*models.AuthBinding, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, provider, provider_id, provider_key, issuer, subject, email, email_verified, provider_data_json, last_login_at, created_at
		   FROM auth_bindings
		  WHERE COALESCE(NULLIF(provider_key, ''), provider) = ? AND COALESCE(NULLIF(subject, ''), provider_id) = ?`,
		strings.TrimSpace(provider),
		strings.TrimSpace(providerID),
	)
	var (
		id          string
		userID      string
		dataJSON    string
		createdAt   string
		emailVerify int
		lastLoginAt sql.NullString
		binding     models.AuthBinding
	)
	if err := row.Scan(&id, &userID, &binding.Provider, &binding.ProviderID, &binding.ProviderKey, &binding.Issuer, &binding.Subject, &binding.Email, &emailVerify, &dataJSON, &lastLoginAt, &createdAt); err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	binding.ID = parsedID
	binding.UserID = parsedUserID
	binding.EmailVerified = emailVerify != 0
	binding.ProviderData = decodeJSONMap(dataJSON)
	binding.CreatedAt = mustParseTime(createdAt)
	if lastLoginAt.Valid {
		ts := mustParseTime(lastLoginAt.String)
		binding.LastLoginAt = &ts
	}
	return &binding, nil
}

type AuthRepo struct {
	Store *Store
}

func NewAuthRepo(store *Store) services.AuthRepo {
	return &AuthRepo{Store: store}
}

func (r *AuthRepo) RegisterUser(ctx context.Context, email, slug, displayName, passwordHash string, now time.Time) (*models.User, error) {
	tx, err := r.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.RegisterUser: begin tx: %w", err)
	}
	defer tx.Rollback()

	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id FROM credentials WHERE email = ?`, email).Scan(&existing)
	if err == nil {
		return nil, fmt.Errorf("email already registered")
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("sqlite.AuthRepo.RegisterUser: check email: %w", err)
	}

	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE slug = ?`, slug).Scan(&existing)
	if err == nil {
		return nil, fmt.Errorf("slug already taken")
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("sqlite.AuthRepo.RegisterUser: check slug: %w", err)
	}

	user := &models.User{
		ID:          uuid.New(),
		Slug:        slug,
		DisplayName: displayName,
		Email:       email,
		Timezone:    "UTC",
		Language:    "en",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	credID := uuid.New()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID.String(), user.Slug, user.DisplayName, user.Email, "", "", user.Timezone, user.Language, timeText(now), timeText(now),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.RegisterUser: insert user: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO credentials (id, user_id, email, password_hash, email_verified, login_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, 0, ?, ?)`,
		credID.String(), user.ID.String(), email, passwordHash, timeText(now), timeText(now),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.RegisterUser: insert credentials: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO roles (id, user_id, name, role_type, config_json, allowed_paths_json, allowed_vault_scopes_json, lifecycle, created_at)
		 VALUES (?, ?, 'assistant', 'assistant', '{}', '["/"]', '[]', 'permanent', ?)`,
		uuid.New().String(), user.ID.String(), timeText(now),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.RegisterUser: insert default role: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.RegisterUser: commit: %w", err)
	}
	return user, nil
}

func (r *AuthRepo) LookupLogin(ctx context.Context, email string) (*models.Credentials, *models.User, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT c.id, c.user_id, c.email, c.password_hash, c.email_verified, c.login_count,
		        u.id, u.slug, u.display_name, u.email, u.avatar_url, u.bio, u.timezone, u.language, u.created_at, u.updated_at
		   FROM credentials c
		   JOIN users u ON u.id = c.user_id
		  WHERE c.email = ?`,
		email,
	)
	var (
		credID, credUserID                                   string
		userID, slug, displayName, userEmail, avatarURL, bio string
		timezone, language, createdAt, updatedAt             string
		cred                                                 models.Credentials
		user                                                 models.User
		emailVerified                                        int
	)
	if err := row.Scan(
		&credID, &credUserID, &cred.Email, &cred.PasswordHash, &emailVerified, &cred.LoginCount,
		&userID, &slug, &displayName, &userEmail, &avatarURL, &bio, &timezone, &language, &createdAt, &updatedAt,
	); err != nil {
		return nil, nil, err
	}
	parsedCredID, err := uuid.Parse(credID)
	if err != nil {
		return nil, nil, err
	}
	parsedUserID, err := uuid.Parse(credUserID)
	if err != nil {
		return nil, nil, err
	}
	cred.ID = parsedCredID
	cred.UserID = parsedUserID
	cred.EmailVerified = emailVerified != 0

	user.ID = parsedUserID
	user.Slug = slug
	user.DisplayName = displayName
	user.Email = userEmail
	user.AvatarURL = avatarURL
	user.Bio = bio
	user.Timezone = timezone
	user.Language = language
	user.CreatedAt = mustParseTime(createdAt)
	user.UpdatedAt = mustParseTime(updatedAt)
	return &cred, &user, nil
}

func (r *AuthRepo) UpdateLoginStats(ctx context.Context, credentialID uuid.UUID, now time.Time) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`UPDATE credentials
		    SET last_login_at = ?, login_count = login_count + 1, updated_at = ?
		  WHERE id = ?`,
		timeText(now), timeText(now), credentialID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.AuthRepo.UpdateLoginStats: %w", err)
	}
	return nil
}

func (r *AuthRepo) CreateSession(ctx context.Context, userID uuid.UUID, refreshTokenHash, userAgent, ipAddress string, expiresAt, createdAt time.Time) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, refresh_token_hash, user_agent, ip_address, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), userID.String(), refreshTokenHash, userAgent, ipAddress, timeText(expiresAt), timeText(createdAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.AuthRepo.CreateSession: %w", err)
	}
	return nil
}

func (r *AuthRepo) GetSession(ctx context.Context, refreshTokenHash string) (*models.Session, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, refresh_token_hash, user_agent, ip_address, expires_at, created_at
		   FROM sessions
		  WHERE refresh_token_hash = ?`,
		refreshTokenHash,
	)
	return scanSessionRow(row)
}

func (r *AuthRepo) DeleteSessionByID(ctx context.Context, sessionID uuid.UUID) error {
	_, err := r.Store.DB().ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID.String())
	if err != nil {
		return fmt.Errorf("sqlite.AuthRepo.DeleteSessionByID: %w", err)
	}
	return nil
}

func (r *AuthRepo) DeleteSessionByRefreshHash(ctx context.Context, refreshTokenHash string) error {
	_, err := r.Store.DB().ExecContext(ctx, `DELETE FROM sessions WHERE refresh_token_hash = ?`, refreshTokenHash)
	if err != nil {
		return fmt.Errorf("sqlite.AuthRepo.DeleteSessionByRefreshHash: %w", err)
	}
	return nil
}

func (r *AuthRepo) CreateOrUpdateGitHubUser(ctx context.Context, githubID, login, displayName, email, avatarURL string, now time.Time) (*models.User, error) {
	tx, err := r.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: begin tx: %w", err)
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM auth_bindings WHERE provider = 'github' AND provider_id = ?`,
		githubID,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		user := models.User{
			ID:          uuid.New(),
			Slug:        login,
			DisplayName: displayName,
			Email:       email,
			AvatarURL:   avatarURL,
			Timezone:    "UTC",
			Language:    "en",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO users (id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			user.ID.String(), user.Slug, user.DisplayName, user.Email, user.AvatarURL, "", user.Timezone, user.Language, timeText(now), timeText(now),
		)
		if err != nil {
			return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: insert user: %w", err)
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_data_json, created_at)
			 VALUES (?, ?, 'github', ?, '{}', ?)`,
			uuid.New().String(), user.ID.String(), githubID, timeText(now),
		)
		if err != nil {
			return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: insert binding: %w", err)
		}
		_, err = tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO roles (id, user_id, name, role_type, config_json, allowed_paths_json, allowed_vault_scopes_json, lifecycle, created_at)
			 VALUES (?, ?, 'assistant', 'assistant', '{}', '["/"]', '[]', 'permanent', ?)`,
			uuid.New().String(), user.ID.String(), timeText(now),
		)
		if err != nil {
			return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: insert default role: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: commit: %w", err)
		}
		return &user, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: lookup binding: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE users
		    SET slug = ?, display_name = ?, email = ?, avatar_url = ?, updated_at = ?
		  WHERE id = ?`,
		login, displayName, email, avatarURL, timeText(now), userID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: update user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.CreateOrUpdateGitHubUser: commit: %w", err)
	}
	return r.Store.UserByID(ctx, uuid.MustParse(userID))
}

func (r *AuthRepo) ListSessions(ctx context.Context, userID uuid.UUID) ([]models.Session, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
		`SELECT id, user_id, refresh_token_hash, user_agent, ip_address, expires_at, created_at
		   FROM sessions
		  WHERE user_id = ? AND expires_at > ?
		  ORDER BY created_at DESC`,
		userID.String(), timeText(time.Now().UTC()),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.ListSessions: %w", err)
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		session, scanErr := scanSessionRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sessions = append(sessions, *session)
	}
	if sessions == nil {
		sessions = []models.Session{}
	}
	return sessions, rows.Err()
}

func (r *AuthRepo) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	result, err := r.Store.DB().ExecContext(ctx,
		`DELETE FROM sessions WHERE id = ? AND user_id = ?`,
		sessionID.String(), userID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.AuthRepo.RevokeSession: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

func (r *AuthRepo) GetCredentialsByUserID(ctx context.Context, userID uuid.UUID) (*models.Credentials, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, email, password_hash, email_verified, verification_token, reset_token, reset_token_expires_at, last_login_at, login_count, created_at, updated_at
		   FROM credentials
		  WHERE user_id = ?`,
		userID.String(),
	)
	return scanCredentialsRow(row)
}

func (r *AuthRepo) UpdatePasswordHash(ctx context.Context, credentialID uuid.UUID, passwordHash string, now time.Time) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`UPDATE credentials SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, timeText(now), credentialID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.AuthRepo.UpdatePasswordHash: %w", err)
	}
	return nil
}

func (r *AuthRepo) GetProfile(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return r.Store.UserByID(ctx, userID)
}

func (r *AuthRepo) UpdateProfile(ctx context.Context, userID uuid.UUID, displayName, bio, timezone, language string, now time.Time) (*models.User, error) {
	_, err := r.Store.DB().ExecContext(ctx,
		`UPDATE users
		    SET display_name = ?, bio = ?, timezone = ?, language = ?, updated_at = ?
		  WHERE id = ?`,
		displayName, bio, timezone, language, timeText(now), userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.AuthRepo.UpdateProfile: %w", err)
	}
	return r.Store.UserByID(ctx, userID)
}

type ConnectionRepo struct {
	Store *Store
}

func NewConnectionRepo(store *Store) services.ConnectionRepo {
	return &ConnectionRepo{Store: store}
}

func (r *ConnectionRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Connection, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
		`SELECT id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix,
		        config_json, last_used_at, created_at, updated_at
		   FROM connections
		  WHERE user_id = ?
		  ORDER BY created_at DESC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ConnectionRepo.ListByUser: %w", err)
	}
	defer rows.Close()

	var conns []models.Connection
	for rows.Next() {
		conn, scanErr := scanConnectionRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		conns = append(conns, *conn)
	}
	if conns == nil {
		conns = []models.Connection{}
	}
	return conns, rows.Err()
}

func (r *ConnectionRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Connection, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix,
		        config_json, last_used_at, created_at, updated_at
		   FROM connections
		  WHERE id = ?`,
		id.String(),
	)
	return scanConnectionRow(row)
}

func (r *ConnectionRepo) GetByAPIKey(ctx context.Context, apiKeyHash string) (*models.Connection, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix,
		        config_json, last_used_at, created_at, updated_at
		   FROM connections
		  WHERE api_key_hash = ?`,
		apiKeyHash,
	)
	return scanConnectionRow(row)
}

func (r *ConnectionRepo) Create(ctx context.Context, conn models.Connection) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO connections (id, user_id, name, platform, trust_level, api_key_hash, api_key_prefix, config_json, last_used_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conn.ID.String(), conn.UserID.String(), conn.Name, conn.Platform, conn.TrustLevel, conn.APIKeyHash, conn.APIKeyPrefix,
		encodeJSON(conn.Config), nullableTimeText(conn.LastUsedAt), timeText(conn.CreatedAt), timeText(conn.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.ConnectionRepo.Create: %w", err)
	}
	return nil
}

func (r *ConnectionRepo) Update(ctx context.Context, id uuid.UUID, name string, trustLevel int, updatedAt time.Time) error {
	result, err := r.Store.DB().ExecContext(ctx,
		`UPDATE connections SET name = ?, trust_level = ?, updated_at = ? WHERE id = ?`,
		name, trustLevel, timeText(updatedAt), id.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.ConnectionRepo.Update: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return services.ErrEntryNotFound
	}
	return nil
}

func (r *ConnectionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.Store.DB().ExecContext(ctx, `DELETE FROM connections WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("sqlite.ConnectionRepo.Delete: %w", err)
	}
	return nil
}

func (r *ConnectionRepo) UpdateLastUsed(ctx context.Context, id uuid.UUID, lastUsedAt time.Time) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`UPDATE connections SET last_used_at = ?, updated_at = ? WHERE id = ?`,
		timeText(lastUsedAt), timeText(lastUsedAt), id.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.ConnectionRepo.UpdateLastUsed: %w", err)
	}
	return nil
}

type VaultRepo struct {
	Store *Store
}

func NewVaultRepo(store *Store) services.VaultRepo {
	return &VaultRepo{Store: store}
}

func (r *VaultRepo) ListScopes(ctx context.Context, userID uuid.UUID, trustLevel int) ([]models.VaultScope, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
		`SELECT id, scope, description, min_trust_level, created_at
		   FROM vault_entries
		  WHERE user_id = ? AND min_trust_level <= ?
		  ORDER BY scope ASC`,
		userID.String(), trustLevel,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.VaultRepo.ListScopes: %w", err)
	}
	defer rows.Close()
	var scopes []models.VaultScope
	for rows.Next() {
		var (
			id        string
			scope     models.VaultScope
			createdAt string
		)
		if err := rows.Scan(&id, &scope.Scope, &scope.Description, &scope.MinTrustLevel, &createdAt); err != nil {
			return nil, err
		}
		scope.ID = uuid.MustParse(id)
		scope.CreatedAt = mustParseTime(createdAt)
		scopes = append(scopes, scope)
	}
	if scopes == nil {
		scopes = []models.VaultScope{}
	}
	return scopes, rows.Err()
}

func (r *VaultRepo) GetEntry(ctx context.Context, userID uuid.UUID, scope string) (*models.VaultEntry, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, scope, encrypted_data, nonce, description, min_trust_level, created_at, updated_at
		   FROM vault_entries
		  WHERE user_id = ? AND scope = ?`,
		userID.String(), scope,
	)
	return scanVaultEntryRow(row)
}

func (r *VaultRepo) UpsertEntry(ctx context.Context, entry models.VaultEntry) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO vault_entries (id, user_id, scope, encrypted_data, nonce, description, min_trust_level, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, scope) DO UPDATE SET
		   encrypted_data = excluded.encrypted_data,
		   nonce = excluded.nonce,
		   description = excluded.description,
		   min_trust_level = excluded.min_trust_level,
		   updated_at = excluded.updated_at`,
		entry.ID.String(), entry.UserID.String(), entry.Scope, entry.EncryptedData, entry.Nonce, entry.Description, entry.MinTrustLevel,
		timeText(entry.CreatedAt), timeText(entry.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.VaultRepo.UpsertEntry: %w", err)
	}
	return nil
}

func (r *VaultRepo) DeleteEntry(ctx context.Context, userID uuid.UUID, scope string) error {
	_, err := r.Store.DB().ExecContext(ctx, `DELETE FROM vault_entries WHERE user_id = ? AND scope = ?`, userID.String(), scope)
	if err != nil {
		return fmt.Errorf("sqlite.VaultRepo.DeleteEntry: %w", err)
	}
	return nil
}

type RoleRepo struct {
	Store *Store
}

func NewRoleRepo(store *Store) services.RoleRepo {
	return &RoleRepo{Store: store}
}

func (r *RoleRepo) List(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
		`SELECT id, user_id, name, role_type, config_json, allowed_paths_json, allowed_vault_scopes_json, lifecycle, created_at
		   FROM roles
		  WHERE user_id = ?
		  ORDER BY name ASC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.RoleRepo.List: %w", err)
	}
	defer rows.Close()
	var roles []models.Role
	for rows.Next() {
		role, scanErr := scanRoleRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		roles = append(roles, *role)
	}
	if roles == nil {
		roles = []models.Role{}
	}
	return roles, rows.Err()
}

func (r *RoleRepo) Create(ctx context.Context, role models.Role) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO roles (id, user_id, name, role_type, config_json, allowed_paths_json, allowed_vault_scopes_json, lifecycle, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		role.ID.String(), role.UserID.String(), role.Name, role.RoleType, encodeJSON(role.Config),
		encodeStringSlice(role.AllowedPaths), encodeStringSlice(role.AllowedVaultScopes), role.Lifecycle, timeText(role.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.RoleRepo.Create: %w", err)
	}
	return nil
}

func (r *RoleRepo) Delete(ctx context.Context, userID uuid.UUID, name string) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`DELETE FROM roles WHERE user_id = ? AND name = ?`,
		userID.String(), name,
	)
	if err != nil {
		return fmt.Errorf("sqlite.RoleRepo.Delete: %w", err)
	}
	return nil
}

func (r *RoleRepo) HasRole(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
	var exists int
	if err := r.Store.DB().QueryRowContext(ctx,
		`SELECT CASE WHEN EXISTS(SELECT 1 FROM roles WHERE user_id = ? AND name = ?) THEN 1 ELSE 0 END`,
		userID.String(), name,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("sqlite.RoleRepo.HasRole: %w", err)
	}
	return exists == 1, nil
}

type OAuthRepo struct {
	Store *Store
}

func NewOAuthRepo(store *Store) services.OAuthRepo {
	return &OAuthRepo{Store: store}
}

func (r *OAuthRepo) CreateApp(ctx context.Context, app models.OAuthApp) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO oauth_apps (id, user_id, name, client_id, client_secret_hash, redirect_uris_json, scopes_json, description, logo_url, is_active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(client_id) DO UPDATE SET
		     user_id = excluded.user_id,
		     name = excluded.name,
		     redirect_uris_json = excluded.redirect_uris_json,
		     scopes_json = excluded.scopes_json,
		     description = excluded.description,
		     logo_url = excluded.logo_url,
		     is_active = excluded.is_active`,
		app.ID.String(), app.UserID.String(), app.Name, app.ClientID, app.ClientSecretHash,
		encodeStringSlice(app.RedirectURIs), encodeStringSlice(app.Scopes), app.Description, app.LogoURL, boolInt(app.IsActive), timeText(app.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.OAuthRepo.CreateApp: %w", err)
	}
	return nil
}

func (r *OAuthRepo) GetAppByID(ctx context.Context, id uuid.UUID) (*models.OAuthApp, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, name, client_id, client_secret_hash, redirect_uris_json, scopes_json, description, logo_url, is_active, created_at
		   FROM oauth_apps
		  WHERE id = ?`,
		id.String(),
	)
	return scanOAuthAppRow(row)
}

func (r *OAuthRepo) GetAppByClientID(ctx context.Context, clientID string) (*models.OAuthApp, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, user_id, name, client_id, client_secret_hash, redirect_uris_json, scopes_json, description, logo_url, is_active, created_at
		   FROM oauth_apps
		  WHERE client_id = ?`,
		clientID,
	)
	return scanOAuthAppRow(row)
}

func (r *OAuthRepo) DeleteApp(ctx context.Context, userID, appID uuid.UUID) error {
	result, err := r.Store.DB().ExecContext(ctx,
		`DELETE FROM oauth_apps WHERE id = ? AND user_id = ?`,
		appID.String(), userID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.OAuthRepo.DeleteApp: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return services.ErrEntryNotFound
	}
	return nil
}

func (r *OAuthRepo) ListApps(ctx context.Context, userID uuid.UUID) ([]models.OAuthApp, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
		`SELECT id, user_id, name, client_id, client_secret_hash, redirect_uris_json, scopes_json, description, logo_url, is_active, created_at
		   FROM oauth_apps
		  WHERE user_id = ?
		  ORDER BY created_at DESC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.OAuthRepo.ListApps: %w", err)
	}
	defer rows.Close()
	var apps []models.OAuthApp
	for rows.Next() {
		app, scanErr := scanOAuthAppRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		apps = append(apps, *app)
	}
	if apps == nil {
		apps = []models.OAuthApp{}
	}
	return apps, rows.Err()
}

func (r *OAuthRepo) CreateCode(ctx context.Context, code models.OAuthCode) error {
	createdAt := code.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO oauth_codes (id, app_id, user_id, code_hash, scopes_json, redirect_uri, code_challenge, code_challenge_method, expires_at, used, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		code.ID.String(), code.AppID.String(), code.UserID.String(), code.CodeHash, encodeStringSlice(code.Scopes),
		code.RedirectURI, code.CodeChallenge, code.CodeChallengeMethod, timeText(code.ExpiresAt), boolInt(code.Used), timeText(createdAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.OAuthRepo.CreateCode: %w", err)
	}
	return nil
}

func (r *OAuthRepo) GetCodeByHash(ctx context.Context, codeHash string) (*models.OAuthCode, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, app_id, user_id, code_hash, scopes_json, redirect_uri, code_challenge, code_challenge_method, expires_at, used, created_at
		   FROM oauth_codes
		  WHERE code_hash = ?`,
		codeHash,
	)
	return scanOAuthCodeRow(row)
}

func (r *OAuthRepo) MarkCodeUsed(ctx context.Context, codeID uuid.UUID) error {
	_, err := r.Store.DB().ExecContext(ctx, `UPDATE oauth_codes SET used = 1 WHERE id = ?`, codeID.String())
	if err != nil {
		return fmt.Errorf("sqlite.OAuthRepo.MarkCodeUsed: %w", err)
	}
	return nil
}

func (r *OAuthRepo) UpsertGrant(ctx context.Context, grant models.OAuthGrant) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO oauth_grants (id, app_id, user_id, scopes_json, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(app_id, user_id) DO UPDATE SET scopes_json = excluded.scopes_json`,
		grant.ID.String(), grant.AppID.String(), grant.UserID.String(), encodeStringSlice(grant.Scopes), timeText(grant.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.OAuthRepo.UpsertGrant: %w", err)
	}
	return nil
}

func (r *OAuthRepo) GetGrant(ctx context.Context, userID, appID uuid.UUID) (*models.OAuthGrant, error) {
	row := r.Store.DB().QueryRowContext(ctx,
		`SELECT id, app_id, user_id, scopes_json, created_at
		   FROM oauth_grants
		  WHERE user_id = ? AND app_id = ?`,
		userID.String(), appID.String(),
	)
	var (
		id        string
		appIDRaw  string
		userIDRaw string
		scopes    string
		createdAt string
	)
	if err := row.Scan(&id, &appIDRaw, &userIDRaw, &scopes, &createdAt); err != nil {
		return nil, err
	}
	return &models.OAuthGrant{
		ID:        uuid.MustParse(id),
		AppID:     uuid.MustParse(appIDRaw),
		UserID:    uuid.MustParse(userIDRaw),
		Scopes:    decodeJSONStringSlice(scopes),
		CreatedAt: mustParseTime(createdAt),
	}, nil
}

func (r *OAuthRepo) ListGrants(ctx context.Context, userID uuid.UUID) ([]models.OAuthGrantResponse, error) {
	rows, err := r.Store.DB().QueryContext(ctx,
		`SELECT g.id, g.scopes_json, g.created_at,
		        a.id, a.name, a.client_id, a.redirect_uris_json, a.scopes_json, a.description, a.logo_url, a.is_active, a.created_at
		   FROM oauth_grants g
		   JOIN oauth_apps a ON a.id = g.app_id
		  WHERE g.user_id = ?
		  ORDER BY g.created_at DESC`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.OAuthRepo.ListGrants: %w", err)
	}
	defer rows.Close()

	var grants []models.OAuthGrantResponse
	for rows.Next() {
		var (
			grantID, appID                          string
			grantScopes, redirectURIs, appScopes    string
			grantCreated, appCreated                string
			appName, clientID, description, logoURL string
			isActive                                int
			item                                    models.OAuthGrantResponse
		)
		if err := rows.Scan(&grantID, &grantScopes, &grantCreated, &appID, &appName, &clientID, &redirectURIs, &appScopes, &description, &logoURL, &isActive, &appCreated); err != nil {
			return nil, err
		}
		item.ID = uuid.MustParse(grantID)
		item.Scopes = decodeJSONStringSlice(grantScopes)
		item.CreatedAt = mustParseTime(grantCreated)
		item.App = models.OAuthAppResponse{
			ID:           uuid.MustParse(appID),
			Name:         appName,
			ClientID:     clientID,
			RedirectURIs: decodeJSONStringSlice(redirectURIs),
			Scopes:       decodeJSONStringSlice(appScopes),
			Description:  description,
			LogoURL:      logoURL,
			IsActive:     isActive != 0,
			CreatedAt:    mustParseTime(appCreated),
		}
		grants = append(grants, item)
	}
	if grants == nil {
		grants = []models.OAuthGrantResponse{}
	}
	return grants, rows.Err()
}

func (r *OAuthRepo) RevokeGrant(ctx context.Context, userID, grantID uuid.UUID) error {
	result, err := r.Store.DB().ExecContext(ctx,
		`DELETE FROM oauth_grants WHERE id = ? AND user_id = ?`,
		grantID.String(), userID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite.OAuthRepo.RevokeGrant: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return services.ErrEntryNotFound
	}
	return nil
}

func (r *OAuthRepo) GetUserSlug(ctx context.Context, userID uuid.UUID) (string, error) {
	var slug string
	err := r.Store.DB().QueryRowContext(ctx, `SELECT slug FROM users WHERE id = ?`, userID.String()).Scan(&slug)
	if err != nil {
		return "", err
	}
	return slug, nil
}

func scanCredentialsRow(row interface{ Scan(dest ...any) error }) (*models.Credentials, error) {
	var (
		id, userID, email, passwordHash, verificationToken, resetToken, createdAt, updatedAt string
		resetTokenExpires, lastLoginAt                                                       *string
		emailVerified                                                                        int
		loginCount                                                                           int
	)
	if err := row.Scan(&id, &userID, &email, &passwordHash, &emailVerified, &verificationToken, &resetToken, &resetTokenExpires, &lastLoginAt, &loginCount, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	cred := &models.Credentials{
		ID:                uuid.MustParse(id),
		UserID:            uuid.MustParse(userID),
		Email:             email,
		PasswordHash:      passwordHash,
		EmailVerified:     emailVerified != 0,
		VerificationToken: verificationToken,
		ResetToken:        resetToken,
		LoginCount:        loginCount,
		CreatedAt:         mustParseTime(createdAt),
		UpdatedAt:         mustParseTime(updatedAt),
	}
	if resetTokenExpires != nil && *resetTokenExpires != "" {
		ts := mustParseTime(*resetTokenExpires)
		cred.ResetTokenExpires = &ts
	}
	if lastLoginAt != nil && *lastLoginAt != "" {
		ts := mustParseTime(*lastLoginAt)
		cred.LastLoginAt = &ts
	}
	return cred, nil
}

func scanSessionRow(row interface{ Scan(dest ...any) error }) (*models.Session, error) {
	var (
		id, userID, refreshHash, userAgent, ipAddress, expiresAt, createdAt string
	)
	if err := row.Scan(&id, &userID, &refreshHash, &userAgent, &ipAddress, &expiresAt, &createdAt); err != nil {
		return nil, err
	}
	return &models.Session{
		ID:               uuid.MustParse(id),
		UserID:           uuid.MustParse(userID),
		RefreshTokenHash: refreshHash,
		UserAgent:        userAgent,
		IPAddress:        ipAddress,
		ExpiresAt:        mustParseTime(expiresAt),
		CreatedAt:        mustParseTime(createdAt),
	}, nil
}

func scanConnectionRow(row interface{ Scan(dest ...any) error }) (*models.Connection, error) {
	var (
		id, userID, name, platform, apiKeyHash, apiKeyPrefix, configJSON, createdAt, updatedAt string
		lastUsedAt                                                                             *string
		trustLevel                                                                             int
	)
	if err := row.Scan(&id, &userID, &name, &platform, &trustLevel, &apiKeyHash, &apiKeyPrefix, &configJSON, &lastUsedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	conn := &models.Connection{
		ID:           uuid.MustParse(id),
		UserID:       uuid.MustParse(userID),
		Name:         name,
		Platform:     platform,
		TrustLevel:   trustLevel,
		APIKeyHash:   apiKeyHash,
		APIKeyPrefix: apiKeyPrefix,
		Config:       decodeJSONMap(configJSON),
		CreatedAt:    mustParseTime(createdAt),
		UpdatedAt:    mustParseTime(updatedAt),
	}
	if lastUsedAt != nil && *lastUsedAt != "" {
		ts := mustParseTime(*lastUsedAt)
		conn.LastUsedAt = &ts
	}
	return conn, nil
}

func scanVaultEntryRow(row interface{ Scan(dest ...any) error }) (*models.VaultEntry, error) {
	var (
		id, userID, scope, description, createdAt, updatedAt string
		encryptedData, nonce                                 []byte
		minTrustLevel                                        int
	)
	if err := row.Scan(&id, &userID, &scope, &encryptedData, &nonce, &description, &minTrustLevel, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return &models.VaultEntry{
		ID:            uuid.MustParse(id),
		UserID:        uuid.MustParse(userID),
		Scope:         scope,
		EncryptedData: encryptedData,
		Nonce:         nonce,
		Description:   description,
		MinTrustLevel: minTrustLevel,
		CreatedAt:     mustParseTime(createdAt),
		UpdatedAt:     mustParseTime(updatedAt),
	}, nil
}

func scanRoleRow(row interface{ Scan(dest ...any) error }) (*models.Role, error) {
	var (
		id, userID, name, roleType, configJSON, allowedPathsJSON, allowedVaultScopesJSON, lifecycle, createdAt string
	)
	if err := row.Scan(&id, &userID, &name, &roleType, &configJSON, &allowedPathsJSON, &allowedVaultScopesJSON, &lifecycle, &createdAt); err != nil {
		return nil, err
	}
	return &models.Role{
		ID:                 uuid.MustParse(id),
		UserID:             uuid.MustParse(userID),
		Name:               name,
		RoleType:           roleType,
		Config:             decodeJSONMap(configJSON),
		AllowedPaths:       decodeJSONStringSlice(allowedPathsJSON),
		AllowedVaultScopes: decodeJSONStringSlice(allowedVaultScopesJSON),
		Lifecycle:          lifecycle,
		CreatedAt:          mustParseTime(createdAt),
	}, nil
}

func scanOAuthAppRow(row interface{ Scan(dest ...any) error }) (*models.OAuthApp, error) {
	var (
		id, userID, name, clientID, secretHash, redirectURIs, scopes, description, logoURL, createdAt string
		isActive                                                                                      int
	)
	if err := row.Scan(&id, &userID, &name, &clientID, &secretHash, &redirectURIs, &scopes, &description, &logoURL, &isActive, &createdAt); err != nil {
		return nil, err
	}
	return &models.OAuthApp{
		ID:               uuid.MustParse(id),
		UserID:           uuid.MustParse(userID),
		Name:             name,
		ClientID:         clientID,
		ClientSecretHash: secretHash,
		RedirectURIs:     decodeJSONStringSlice(redirectURIs),
		Scopes:           decodeJSONStringSlice(scopes),
		Description:      description,
		LogoURL:          logoURL,
		IsActive:         isActive != 0,
		CreatedAt:        mustParseTime(createdAt),
	}, nil
}

func scanOAuthCodeRow(row interface{ Scan(dest ...any) error }) (*models.OAuthCode, error) {
	var (
		id, appID, userID, codeHash, scopes, redirectURI, codeChallenge, codeChallengeMethod, expiresAt, createdAt string
		used                                                                                                       int
	)
	if err := row.Scan(&id, &appID, &userID, &codeHash, &scopes, &redirectURI, &codeChallenge, &codeChallengeMethod, &expiresAt, &used, &createdAt); err != nil {
		return nil, err
	}
	return &models.OAuthCode{
		ID:                  uuid.MustParse(id),
		AppID:               uuid.MustParse(appID),
		UserID:              uuid.MustParse(userID),
		CodeHash:            codeHash,
		Scopes:              decodeJSONStringSlice(scopes),
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           mustParseTime(expiresAt),
		Used:                used != 0,
		CreatedAt:           mustParseTime(createdAt),
	}, nil
}

func nullableTimeText(ts *time.Time) any {
	if ts == nil || ts.IsZero() {
		return nil
	}
	return timeText(ts.UTC())
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
