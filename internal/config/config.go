package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL             string
	Port                    string
	JWTSecret               string
	GithubClientID          string
	GithubClientSecret      string
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
		CaptureOAuth:            getEnvBool("NEUDRIVE_CAPTURE_OAUTH", false),
		CaptureDir:              envOrOverride("NEUDRIVE_CAPTURE_DIR", "tmp/oauth-captures"),
	}

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
