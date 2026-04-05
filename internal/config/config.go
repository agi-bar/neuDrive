package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL        string
	Port               string
	JWTSecret          string
	GithubClientID     string
	GithubClientSecret string
	VaultMasterKey     string
	PublicBaseURL      string
	CORSOrigins        []string
	RateLimit          int   // max requests per minute
	MaxBodySize        int64 // max request body in bytes
	LogLevel           string
	LogFormat          string
	CaptureOAuth       bool
	CaptureDir         string
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://localhost:5432/agenthub?sslmode=disable"),
		Port:               getEnv("PORT", "8080"),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		GithubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GithubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		VaultMasterKey:     getEnv("VAULT_MASTER_KEY", ""),
		PublicBaseURL:      strings.TrimRight(getEnv("PUBLIC_BASE_URL", ""), "/"),
		CORSOrigins:        strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),
		RateLimit:          getEnvInt("RATE_LIMIT", 100),
		MaxBodySize:        int64(getEnvInt("MAX_BODY_SIZE", 10*1024*1024)),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		LogFormat:          getEnv("LOG_FORMAT", "text"),
		CaptureOAuth:       getEnvBool("AGENTHUB_CAPTURE_OAUTH", false),
		CaptureDir:         getEnv("AGENTHUB_CAPTURE_DIR", "tmp/oauth-captures"),
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
