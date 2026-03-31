package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL        string
	Port               string
	JWTSecret          string
	GithubClientID     string
	GithubClientSecret string
	VaultMasterKey     string
	CORSOrigins        []string
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://localhost:5432/agenthub?sslmode=disable"),
		Port:               getEnv("PORT", "8080"),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		GithubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GithubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		VaultMasterKey:     getEnv("VAULT_MASTER_KEY", ""),
		CORSOrigins:        strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),
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
