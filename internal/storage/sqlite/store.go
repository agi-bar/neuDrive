package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Store struct {
	path string
	db   *sql.DB
}

const sqliteDriverName = "sqlite"

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite.Open: sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{path: path, db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) init(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("sqlite.init: database not configured")
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			bio TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL,
			language TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN avatar_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN bio TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS auth_bindings (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			provider_data_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			UNIQUE(provider, provider_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS credentials (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email_verified INTEGER NOT NULL DEFAULT 0,
			verification_token TEXT NOT NULL DEFAULT '',
			reset_token TEXT NOT NULL DEFAULT '',
			reset_token_expires_at TEXT,
			last_login_at TEXT,
			login_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			refresh_token_hash TEXT NOT NULL UNIQUE,
			user_agent TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS connections (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			platform TEXT NOT NULL,
			trust_level INTEGER NOT NULL,
			api_key_hash TEXT NOT NULL UNIQUE,
			api_key_prefix TEXT NOT NULL,
			config_json TEXT NOT NULL DEFAULT '{}',
			last_used_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS vault_entries (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			scope TEXT NOT NULL,
			encrypted_data BLOB NOT NULL,
			nonce BLOB NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			min_trust_level INTEGER NOT NULL DEFAULT 4,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, scope),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS roles (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			role_type TEXT NOT NULL,
			config_json TEXT NOT NULL DEFAULT '{}',
			allowed_paths_json TEXT NOT NULL DEFAULT '[]',
			allowed_vault_scopes_json TEXT NOT NULL DEFAULT '[]',
			lifecycle TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(user_id, name),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			device_type TEXT NOT NULL,
			brand TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT '',
			endpoint TEXT NOT NULL DEFAULT '',
			skill_md TEXT NOT NULL DEFAULT '',
			config_json TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'online',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, name),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS inbox_messages (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			from_address TEXT NOT NULL,
			to_address TEXT NOT NULL,
			thread_id TEXT NOT NULL DEFAULT '',
			priority TEXT NOT NULL DEFAULT 'normal',
			action_required INTEGER NOT NULL DEFAULT 0,
			ttl TEXT,
			expires_at TEXT,
			domain TEXT NOT NULL DEFAULT '',
			action_type TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			context_hash TEXT NOT NULL DEFAULT '',
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			structured_payload_json TEXT NOT NULL DEFAULT '{}',
			attachments_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'incoming',
			created_at TEXT NOT NULL,
			archived_at TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_apps (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			client_id TEXT NOT NULL UNIQUE,
			client_secret_hash TEXT NOT NULL,
			redirect_uris_json TEXT NOT NULL DEFAULT '[]',
			scopes_json TEXT NOT NULL DEFAULT '[]',
			description TEXT NOT NULL DEFAULT '',
			logo_url TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_codes (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			code_hash TEXT NOT NULL UNIQUE,
			scopes_json TEXT NOT NULL DEFAULT '[]',
			redirect_uri TEXT NOT NULL,
			code_challenge TEXT NOT NULL DEFAULT '',
			code_challenge_method TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (app_id) REFERENCES oauth_apps(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_grants (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			scopes_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			UNIQUE(app_id, user_id),
			FOREIGN KEY (app_id) REFERENCES oauth_apps(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS scoped_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			token_prefix TEXT NOT NULL,
			scopes_json TEXT NOT NULL,
			max_trust_level INTEGER NOT NULL,
			expires_at TEXT NOT NULL,
			rate_limit INTEGER NOT NULL DEFAULT 1000,
			request_count INTEGER NOT NULL DEFAULT 0,
			rate_limit_reset_at TEXT NOT NULL,
			last_used_at TEXT,
			last_used_ip TEXT,
			created_at TEXT NOT NULL,
			revoked_at TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS file_tree (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			path TEXT NOT NULL,
			kind TEXT NOT NULL,
			is_directory INTEGER NOT NULL DEFAULT 0,
			content TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT 'text/plain',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			checksum TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1,
			min_trust_level INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT,
			UNIQUE(user_id, path),
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS file_blobs (
			entry_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			data BLOB NOT NULL,
			size_bytes INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY (entry_id) REFERENCES file_tree(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS sync_jobs (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			session_id TEXT,
			direction TEXT NOT NULL,
			transport TEXT NOT NULL,
			status TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			mode TEXT NOT NULL DEFAULT '',
			filters_json TEXT NOT NULL DEFAULT '{}',
			summary_json TEXT NOT NULL DEFAULT '{}',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sync_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			job_id TEXT NOT NULL,
			status TEXT NOT NULL,
			format TEXT NOT NULL,
			mode TEXT NOT NULL,
			manifest_json TEXT NOT NULL,
			archive_size_bytes INTEGER NOT NULL,
			archive_sha256 TEXT NOT NULL,
			chunk_size_bytes INTEGER NOT NULL,
			total_parts INTEGER NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			committed_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sync_session_parts (
			session_id TEXT NOT NULL,
			part_index INTEGER NOT NULL,
			part_hash TEXT NOT NULL,
			data BLOB NOT NULL,
			size_bytes INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (session_id, part_index),
			FOREIGN KEY (session_id) REFERENCES sync_sessions(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_file_tree_user_path ON file_tree(user_id, path)`,
		`CREATE INDEX IF NOT EXISTS idx_file_tree_user_updated ON file_tree(user_id, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_credentials_email ON credentials(email)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_connections_user_created ON connections(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_vault_entries_user_scope ON vault_entries(user_id, scope)`,
		`CREATE INDEX IF NOT EXISTS idx_roles_user_name ON roles(user_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_user_name ON devices(user_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_inbox_messages_user_created ON inbox_messages(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_inbox_messages_user_status ON inbox_messages(user_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_inbox_messages_user_role_status ON inbox_messages(user_id, to_address, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_apps_user_created ON oauth_apps(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_codes_hash ON oauth_codes(code_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_grants_user_created ON oauth_grants(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_jobs_user_created ON sync_jobs(user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_sessions_user_updated ON sync_sessions(user_id, updated_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			if strings.Contains(stmt, "ALTER TABLE users ADD COLUMN") && strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("sqlite.init: %w", err)
		}
	}
	return nil
}

func (s *Store) EnsureOwner(ctx context.Context) (*models.User, error) {
	if user, err := s.firstUser(ctx); err == nil {
		return user, nil
	}
	now := time.Now().UTC()
	user := &models.User{
		ID:          uuid.New(),
		Slug:        "local",
		DisplayName: "Local Owner",
		Email:       "",
		Timezone:    "UTC",
		Language:    "en",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID.String(),
		user.Slug,
		user.DisplayName,
		user.Email,
		user.AvatarURL,
		user.Bio,
		user.Timezone,
		user.Language,
		timeText(user.CreatedAt),
		timeText(user.UpdatedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite.EnsureOwner: %w", err)
	}
	return user, nil
}

func (s *Store) FirstUserID(ctx context.Context) (uuid.UUID, error) {
	user, err := s.firstUser(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return user.ID, nil
}

func (s *Store) firstUser(ctx context.Context) (*models.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at
		 FROM users ORDER BY created_at ASC LIMIT 1`)
	var (
		id        string
		slug      string
		name      string
		email     string
		avatarURL string
		bio       string
		timezone  string
		language  string
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&id, &slug, &name, &email, &avatarURL, &bio, &timezone, &language, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no users found")
		}
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return &models.User{
		ID:          parsedID,
		Slug:        slug,
		DisplayName: name,
		Email:       email,
		AvatarURL:   avatarURL,
		Bio:         bio,
		Timezone:    timezone,
		Language:    language,
		CreatedAt:   mustParseTime(createdAt),
		UpdatedAt:   mustParseTime(updatedAt),
	}, nil
}

func (s *Store) UserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at
		 FROM users WHERE id = ?`,
		userID.String(),
	)
	var (
		id        string
		slug      string
		name      string
		email     string
		avatarURL string
		bio       string
		timezone  string
		language  string
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&id, &slug, &name, &email, &avatarURL, &bio, &timezone, &language, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return &models.User{
		ID:          parsedID,
		Slug:        slug,
		DisplayName: name,
		Email:       email,
		AvatarURL:   avatarURL,
		Bio:         bio,
		Timezone:    timezone,
		Language:    language,
		CreatedAt:   mustParseTime(createdAt),
		UpdatedAt:   mustParseTime(updatedAt),
	}, nil
}

func (s *Store) UserBySlug(ctx context.Context, slug string) (*models.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, slug, display_name, email, avatar_url, bio, timezone, language, created_at, updated_at
		 FROM users WHERE slug = ?`,
		strings.TrimSpace(slug),
	)
	var (
		id        string
		userSlug  string
		name      string
		email     string
		avatarURL string
		bio       string
		timezone  string
		language  string
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&id, &userSlug, &name, &email, &avatarURL, &bio, &timezone, &language, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return &models.User{
		ID:          parsedID,
		Slug:        userSlug,
		DisplayName: name,
		Email:       email,
		AvatarURL:   avatarURL,
		Bio:         bio,
		Timezone:    timezone,
		Language:    language,
		CreatedAt:   mustParseTime(createdAt),
		UpdatedAt:   mustParseTime(updatedAt),
	}, nil
}

func timeText(ts time.Time) string {
	return ts.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return ts.UTC()
}

func encodeJSON(value any) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func decodeJSONMap(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return map[string]interface{}{}
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil || result == nil {
		return map[string]interface{}{}
	}
	return result
}

func decodeJSONStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result []string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	return result
}

func encodeStringSlice(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	clone := append([]string{}, items...)
	sort.Strings(clone)
	return encodeJSON(clone)
}
