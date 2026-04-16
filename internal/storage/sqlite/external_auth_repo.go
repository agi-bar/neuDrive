package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/google/uuid"
)

var sqliteExternalSlugSanitizer = regexp.MustCompile(`[^a-z0-9_-]+`)

type ExternalAuthRepo struct {
	Store *Store
}

func NewExternalAuthRepo(store *Store) services.ExternalAuthRepo {
	return &ExternalAuthRepo{Store: store}
}

func (r *ExternalAuthRepo) CreateAuthTransaction(ctx context.Context, txn models.AuthTransaction) error {
	_, err := r.Store.DB().ExecContext(ctx,
		`INSERT INTO auth_transactions (id, provider_key, state, nonce, code_verifier, redirect_url, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		txn.ID.String(), txn.ProviderKey, txn.State, txn.Nonce, txn.CodeVerifier, txn.RedirectURL, timeText(txn.ExpiresAt), timeText(txn.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite.ExternalAuthRepo.CreateAuthTransaction: %w", err)
	}
	return nil
}

func (r *ExternalAuthRepo) ConsumeAuthTransaction(ctx context.Context, providerKey, state string, now time.Time) (*models.AuthTransaction, error) {
	tx, err := r.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ExternalAuthRepo.ConsumeAuthTransaction: begin tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx,
		`SELECT id, provider_key, state, nonce, code_verifier, redirect_url, expires_at, consumed_at, created_at
		   FROM auth_transactions
		  WHERE provider_key = ? AND state = ?`,
		providerKey, state,
	)
	var (
		id, nonce, codeVerifier, redirectURL string
		expiresAt, createdAt                 string
		consumedAt                           sql.NullString
		txn                                  models.AuthTransaction
	)
	if err := row.Scan(&id, &txn.ProviderKey, &txn.State, &nonce, &codeVerifier, &redirectURL, &expiresAt, &consumedAt, &createdAt); err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	txn.ID = parsedID
	txn.Nonce = nonce
	txn.CodeVerifier = codeVerifier
	txn.RedirectURL = redirectURL
	txn.ExpiresAt = mustParseTime(expiresAt)
	txn.CreatedAt = mustParseTime(createdAt)
	if consumedAt.Valid || now.After(txn.ExpiresAt) {
		return nil, sql.ErrNoRows
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE auth_transactions
		    SET consumed_at = ?
		  WHERE id = ? AND consumed_at IS NULL`,
		timeText(now), txn.ID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.ExternalAuthRepo.ConsumeAuthTransaction: update: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("sqlite.ExternalAuthRepo.ConsumeAuthTransaction: rows affected: %w", err)
	}
	if rows == 0 {
		return nil, sql.ErrNoRows
	}
	txn.ConsumedAt = &now
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite.ExternalAuthRepo.ConsumeAuthTransaction: commit: %w", err)
	}
	return &txn, nil
}

func (r *ExternalAuthRepo) UpsertExternalIdentity(ctx context.Context, input models.ExternalIdentityUpsert, now time.Time) (*models.User, *models.AuthBinding, error) {
	tx, err := r.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: begin tx: %w", err)
	}
	defer tx.Rollback()

	providerKey := strings.TrimSpace(input.ProviderKey)
	subject := strings.TrimSpace(input.Subject)
	if providerKey == "" || subject == "" {
		return nil, nil, fmt.Errorf("provider_key and subject are required")
	}

	binding, user, err := loadExternalBindingAndUserSQLite(ctx, tx, providerKey, subject)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, err
	}
	if binding != nil {
		if binding.Issuer != "" && strings.TrimSpace(input.Issuer) != "" && binding.Issuer != strings.TrimSpace(input.Issuer) {
			return nil, nil, fmt.Errorf("provider issuer mismatch")
		}
		applySQLiteExternalProfile(user, input, now)
		_, err = tx.ExecContext(ctx,
			`UPDATE users
			    SET display_name = ?, email = ?, avatar_url = ?, timezone = ?, language = ?, updated_at = ?
			  WHERE id = ?`,
			user.DisplayName, user.Email, user.AvatarURL, user.Timezone, user.Language, timeText(user.UpdatedAt), user.ID.String(),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: update user: %w", err)
		}
		_, err = tx.ExecContext(ctx,
			`UPDATE auth_bindings
			    SET provider = ?,
			        provider_id = ?,
			        provider_key = ?,
			        issuer = ?,
			        subject = ?,
			        email = ?,
			        email_verified = ?,
			        provider_data_json = ?,
			        last_login_at = ?
			  WHERE id = ?`,
			providerKey, subject, providerKey, strings.TrimSpace(input.Issuer), subject, strings.ToLower(strings.TrimSpace(input.Email)), boolInt(input.EmailVerified), encodeJSON(input.ProfileData), timeText(now), binding.ID.String(),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: update binding: %w", err)
		}
		binding.Provider = providerKey
		binding.ProviderID = subject
		binding.ProviderKey = providerKey
		binding.Issuer = strings.TrimSpace(input.Issuer)
		binding.Subject = subject
		binding.Email = strings.ToLower(strings.TrimSpace(input.Email))
		binding.EmailVerified = input.EmailVerified
		binding.ProviderData = decodeJSONMap(encodeJSON(input.ProfileData))
		binding.LastLoginAt = &now
		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: commit update: %w", err)
		}
		return user, binding, nil
	}

	slug, err := reserveUniqueSlugSQLite(ctx, tx, input.SlugCandidates)
	if err != nil {
		return nil, nil, err
	}
	user = &models.User{
		ID:          uuid.New(),
		Slug:        slug,
		DisplayName: sqliteResolveDisplayName(input.DisplayName, slug),
		Email:       strings.ToLower(strings.TrimSpace(input.Email)),
		AvatarURL:   strings.TrimSpace(input.AvatarURL),
		Bio:         "",
		Timezone:    sqliteDefaultString(strings.TrimSpace(input.Timezone), "UTC"),
		Language:    sqliteDefaultString(strings.TrimSpace(input.Language), "en"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, ?)`,
		user.ID.String(), user.Slug, user.DisplayName, user.Email, user.AvatarURL, user.Timezone, user.Language, timeText(now), timeText(now),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: create user: %w", err)
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
		ProviderData:  decodeJSONMap(encodeJSON(input.ProfileData)),
		LastLoginAt:   &now,
		CreatedAt:     now,
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO auth_bindings (id, user_id, provider, provider_id, provider_key, issuer, subject, email, email_verified, provider_data_json, last_login_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		binding.ID.String(), binding.UserID.String(), binding.Provider, binding.ProviderID, binding.ProviderKey, binding.Issuer, binding.Subject, binding.Email, boolInt(binding.EmailVerified), encodeJSON(input.ProfileData), timeText(now), timeText(now),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: create binding: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO roles (id, user_id, name, role_type, config_json, allowed_paths_json, allowed_vault_scopes_json, lifecycle, created_at)
		 VALUES (?, ?, 'assistant', 'assistant', '{}', '["/"]', '[]', 'permanent', ?)`,
		uuid.New().String(), user.ID.String(), timeText(now),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: create role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("sqlite.ExternalAuthRepo.UpsertExternalIdentity: commit create: %w", err)
	}
	return user, binding, nil
}

func loadExternalBindingAndUserSQLite(ctx context.Context, tx *sql.Tx, providerKey, subject string) (*models.AuthBinding, *models.User, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT ab.id,
		        ab.user_id,
		        COALESCE(ab.provider, ''),
		        COALESCE(ab.provider_id, ''),
		        COALESCE(ab.provider_key, ''),
		        COALESCE(ab.issuer, ''),
		        COALESCE(ab.subject, ''),
		        COALESCE(ab.email, ''),
		        COALESCE(ab.email_verified, 0),
		        COALESCE(ab.provider_data_json, '{}'),
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
		  WHERE COALESCE(NULLIF(ab.provider_key, ''), ab.provider) = ?
		    AND COALESCE(NULLIF(ab.subject, ''), ab.provider_id) = ?`,
		providerKey, subject,
	)
	var (
		binding                       models.AuthBinding
		user                          models.User
		bindingID, userID, rawUserID  string
		profileJSON, bindingCreatedAt string
		userCreatedAt, userUpdatedAt  string
		lastLoginAt                   sql.NullString
		emailVerified                 int
	)
	if err := row.Scan(
		&bindingID,
		&userID,
		&binding.Provider,
		&binding.ProviderID,
		&binding.ProviderKey,
		&binding.Issuer,
		&binding.Subject,
		&binding.Email,
		&emailVerified,
		&profileJSON,
		&lastLoginAt,
		&bindingCreatedAt,
		&rawUserID,
		&user.Slug,
		&user.DisplayName,
		&user.Email,
		&user.AvatarURL,
		&user.Bio,
		&user.Timezone,
		&user.Language,
		&userCreatedAt,
		&userUpdatedAt,
	); err != nil {
		return nil, nil, err
	}
	parsedBindingID, err := uuid.Parse(bindingID)
	if err != nil {
		return nil, nil, err
	}
	parsedUserID, err := uuid.Parse(rawUserID)
	if err != nil {
		return nil, nil, err
	}
	binding.ID = parsedBindingID
	binding.UserID = parsedUserID
	binding.EmailVerified = emailVerified != 0
	binding.ProviderData = decodeJSONMap(profileJSON)
	binding.CreatedAt = mustParseTime(bindingCreatedAt)
	if lastLoginAt.Valid {
		ts := mustParseTime(lastLoginAt.String)
		binding.LastLoginAt = &ts
	}
	user.ID = parsedUserID
	user.CreatedAt = mustParseTime(userCreatedAt)
	user.UpdatedAt = mustParseTime(userUpdatedAt)
	return &binding, &user, nil
}

func applySQLiteExternalProfile(user *models.User, input models.ExternalIdentityUpsert, now time.Time) {
	if user == nil {
		return
	}
	if displayName := strings.TrimSpace(input.DisplayName); displayName != "" {
		user.DisplayName = displayName
	}
	if email := strings.ToLower(strings.TrimSpace(input.Email)); email != "" {
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
		user.Timezone = "UTC"
	}
	if user.Language == "" {
		user.Language = "en"
	}
	user.UpdatedAt = now
}

func reserveUniqueSlugSQLite(ctx context.Context, tx *sql.Tx, candidates []string) (string, error) {
	for _, candidate := range normalizeSQLiteSlugCandidates(candidates) {
		for suffix := 0; suffix < 100; suffix++ {
			slug := candidate
			if suffix > 0 {
				slug = fmt.Sprintf("%s-%d", candidate, suffix+1)
			}
			var existing string
			err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE slug = ?`, slug).Scan(&existing)
			if err == sql.ErrNoRows {
				return slug, nil
			}
			if err != nil {
				return "", fmt.Errorf("sqlite.ExternalAuthRepo.reserveUniqueSlugSQLite: %w", err)
			}
		}
	}
	return "", fmt.Errorf("unable to generate unique slug")
}

func normalizeSQLiteSlugCandidates(candidates []string) []string {
	seen := map[string]struct{}{}
	var normalized []string
	for _, candidate := range candidates {
		slug := sqliteSlugify(candidate)
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

func sqliteSlugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = sqliteExternalSlugSanitizer.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	if len(value) > 64 {
		value = strings.Trim(value[:64], "-_")
	}
	if len(value) < 3 {
		return ""
	}
	return value
}

func sqliteResolveDisplayName(displayName, slug string) string {
	if strings.TrimSpace(displayName) != "" {
		return strings.TrimSpace(displayName)
	}
	return slug
}

func sqliteDefaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
