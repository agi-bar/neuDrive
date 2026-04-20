package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL             string
	Port                    string
	JWTSecret               string
	UserStorageQuotaBytes   int64
	GithubClientID          string
	GithubClientSecret      string
	PocketProviderID        string
	PocketIssuer            string
	PocketDiscoveryURL      string
	PocketClientID          string
	PocketClientSecret      string
	PocketScopes            []string
	GitHubAppClientID       string
	GitHubAppClientSecret   string
	GitHubAppSlug           string
	GitMirrorHostedRoot     string
	FeishuAppID             string
	FeishuAppSecret         string
	FeishuVerificationToken string
	FeishuEncryptKey        string
	VaultMasterKey          string
	PublicBaseURL           string
	CORSOrigins             []string
	RateLimit               int   // max requests per minute
	MaxBodySize             int64 // max request body in bytes
	LogLevel                string
	LogFormat               string
	EnableSystemSettings    bool
	EnableBilling           bool
	CaptureOAuth            bool
	CaptureDir              string
}

func Load() (*Config, error) {
	return LoadWithOverrides(nil)
}

func LoadWithOverrides(overrides map[string]string) (*Config, error) {
	envOrOverride := func(key, fallback string) string {
		if overrides != nil {
			if value, ok := overrides[key]; ok {
				return value
			}
		}
		return getEnv(key, fallback)
	}

	cfg := &Config{
		DatabaseURL:             envOrOverride("DATABASE_URL", "postgres://localhost:5432/neudrive?sslmode=disable"),
		Port:                    envOrOverride("PORT", "8080"),
		JWTSecret:               envOrOverride("JWT_SECRET", ""),
		GithubClientID:          envOrOverride("GITHUB_CLIENT_ID", ""),
		GithubClientSecret:      envOrOverride("GITHUB_CLIENT_SECRET", ""),
		PocketProviderID:        envOrOverride("POCKET_ID_PROVIDER_ID", "pocket"),
		PocketIssuer:            strings.TrimRight(envOrOverride("POCKET_ID_ISSUER", ""), "/"),
		PocketDiscoveryURL:      strings.TrimSpace(envOrOverride("POCKET_ID_DISCOVERY_URL", "")),
		PocketClientID:          envOrOverride("POCKET_ID_CLIENT_ID", ""),
		PocketClientSecret:      envOrOverride("POCKET_ID_CLIENT_SECRET", ""),
		PocketScopes:            splitScopes(envOrOverride("POCKET_ID_SCOPES", "openid profile email")),
		GitHubAppClientID:       envOrOverride("GITHUB_APP_CLIENT_ID", ""),
		GitHubAppClientSecret:   envOrOverride("GITHUB_APP_CLIENT_SECRET", ""),
		GitHubAppSlug:           envOrOverride("GITHUB_APP_SLUG", ""),
		GitMirrorHostedRoot:     envOrOverride("GIT_MIRROR_HOSTED_ROOT", ""),
		FeishuAppID:             envOrOverride("FEISHU_APP_ID", ""),
		FeishuAppSecret:         envOrOverride("FEISHU_APP_SECRET", ""),
		FeishuVerificationToken: envOrOverride("FEISHU_VERIFICATION_TOKEN", ""),
		FeishuEncryptKey:        envOrOverride("FEISHU_ENCRYPT_KEY", ""),
		VaultMasterKey:          envOrOverride("VAULT_MASTER_KEY", ""),
		PublicBaseURL:           strings.TrimRight(envOrOverride("PUBLIC_BASE_URL", ""), "/"),
		CORSOrigins:             strings.Split(envOrOverride("CORS_ORIGINS", "http://localhost:3000"), ","),
		RateLimit:               getEnvInt("RATE_LIMIT", 100),
		MaxBodySize:             int64(getEnvInt("MAX_BODY_SIZE", 10*1024*1024)),
		LogLevel:                envOrOverride("LOG_LEVEL", "info"),
		LogFormat:               envOrOverride("LOG_FORMAT", "text"),
		EnableSystemSettings:    getEnvBool("NEUDRIVE_ENABLE_SYSTEM_SETTINGS", true),
		EnableBilling:           getEnvBool("NEUDRIVE_ENABLE_BILLING", false),
		CaptureOAuth:            getEnvBool("NEUDRIVE_CAPTURE_OAUTH", false),
		CaptureDir:              envOrOverride("NEUDRIVE_CAPTURE_DIR", "tmp/oauth-captures"),
	}

	quotaBytes, err := parseByteSize(envOrOverride("USER_STORAGE_QUOTA_BYTES", "0"))
	if err != nil {
		return nil, fmt.Errorf("invalid USER_STORAGE_QUOTA_BYTES: %w", err)
	}
	cfg.UserStorageQuotaBytes = quotaBytes

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	if cfg.VaultMasterKey == "" {
		return nil, fmt.Errorf("VAULT_MASTER_KEY environment variable is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	s := getEnv(key, "")
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func getEnvBool(key string, fallback bool) bool {
	s := strings.TrimSpace(strings.ToLower(getEnv(key, "")))
	if s == "" {
		return fallback
	}
	switch s {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitScopes(value string) []string {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) == 0 {
		return []string{}
	}
	return parts
}

func parseByteSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	upper := strings.ToUpper(value)
	split := len(upper)
	for i, r := range upper {
		if r < '0' || r > '9' {
			split = i
			break
		}
	}

	numberPart := strings.TrimSpace(upper[:split])
	unitPart := strings.TrimSpace(upper[split:])
	if numberPart == "" {
		return 0, fmt.Errorf("missing numeric value")
	}

	number, err := strconv.ParseInt(numberPart, 10, 64)
	if err != nil {
		return 0, err
	}
	if number < 0 {
		return 0, fmt.Errorf("must be non-negative")
	}

	multiplier, ok := map[string]int64{
		"":      1,
		"B":     1,
		"BYTE":  1,
		"BYTES": 1,
		"K":     1024,
		"KB":    1024,
		"KIB":   1024,
		"M":     1024 * 1024,
		"MB":    1024 * 1024,
		"MIB":   1024 * 1024,
		"G":     1024 * 1024 * 1024,
		"GB":    1024 * 1024 * 1024,
		"GIB":   1024 * 1024 * 1024,
		"T":     1024 * 1024 * 1024 * 1024,
		"TB":    1024 * 1024 * 1024 * 1024,
		"TIB":   1024 * 1024 * 1024 * 1024,
		"P":     1024 * 1024 * 1024 * 1024 * 1024,
		"PB":    1024 * 1024 * 1024 * 1024 * 1024,
		"PIB":   1024 * 1024 * 1024 * 1024 * 1024,
	}[unitPart]
	if !ok {
		return 0, fmt.Errorf("unsupported size suffix %q", unitPart)
	}
	if number > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("value too large")
	}
	return number * multiplier, nil
}
