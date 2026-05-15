package config

import "testing"

func TestLoadWithOverridesParsesUserStorageQuotaUnits(t *testing.T) {
	cfg, err := LoadWithOverrides(map[string]string{
		"JWT_SECRET":               "secret",
		"VAULT_MASTER_KEY":         "vault",
		"USER_STORAGE_QUOTA_BYTES": "10GB",
	})
	if err != nil {
		t.Fatalf("LoadWithOverrides: %v", err)
	}
	const want = 10 * 1024 * 1024 * 1024
	if cfg.UserStorageQuotaBytes != want {
		t.Fatalf("UserStorageQuotaBytes = %d, want %d", cfg.UserStorageQuotaBytes, want)
	}
}

func TestLoadWithOverridesRejectsInvalidUserStorageQuota(t *testing.T) {
	_, err := LoadWithOverrides(map[string]string{
		"JWT_SECRET":               "secret",
		"VAULT_MASTER_KEY":         "vault",
		"USER_STORAGE_QUOTA_BYTES": "10XB",
	})
	if err == nil {
		t.Fatal("LoadWithOverrides succeeded for invalid storage quota")
	}
}

func TestLoadWithOverridesParsesGitMirrorManualSyncCooldown(t *testing.T) {
	cfg, err := LoadWithOverrides(map[string]string{
		"JWT_SECRET":       "secret",
		"VAULT_MASTER_KEY": "vault",
		"GIT_MIRROR_MANUAL_SYNC_COOLDOWN_SECONDS": "45",
	})
	if err != nil {
		t.Fatalf("LoadWithOverrides: %v", err)
	}
	if !cfg.GitMirrorManualSyncCooldownConfigured || cfg.GitMirrorManualSyncCooldownSeconds != 45 {
		t.Fatalf("cooldown = configured:%v seconds:%d, want configured:true seconds:45", cfg.GitMirrorManualSyncCooldownConfigured, cfg.GitMirrorManualSyncCooldownSeconds)
	}
}

func TestLoadWithOverridesRejectsInvalidGitMirrorManualSyncCooldown(t *testing.T) {
	_, err := LoadWithOverrides(map[string]string{
		"JWT_SECRET":       "secret",
		"VAULT_MASTER_KEY": "vault",
		"GIT_MIRROR_MANUAL_SYNC_COOLDOWN_SECONDS": "-1",
	})
	if err == nil {
		t.Fatal("LoadWithOverrides succeeded for invalid Git Mirror manual sync cooldown")
	}
}

func TestLoadWithOverridesParsesFeedbackLaunchConfig(t *testing.T) {
	cfg, err := LoadWithOverrides(map[string]string{
		"JWT_SECRET":                  "secret",
		"VAULT_MASTER_KEY":            "vault",
		"PUBLIC_BASE_URL":             "https://www.neudrive.ai/",
		"FEEDBACK_ENABLED":            "1",
		"FEEDBACK_LAUNCH_URL":         "https://triage.example.com/feedback/start",
		"FEEDBACK_LAUNCH_SECRET":      "launch-secret",
		"FEEDBACK_LAUNCH_AUDIENCE":    "triage.feedback",
		"FEEDBACK_LAUNCH_PROJECT_ID":  "neudrive",
		"FEEDBACK_LAUNCH_TTL_SECONDS": "120",
	})
	if err != nil {
		t.Fatalf("LoadWithOverrides: %v", err)
	}
	if !cfg.FeedbackEnabled {
		t.Fatal("FeedbackEnabled = false, want true")
	}
	if cfg.FeedbackLaunchIssuer != "https://www.neudrive.ai" {
		t.Fatalf("FeedbackLaunchIssuer = %q, want public base URL without trailing slash", cfg.FeedbackLaunchIssuer)
	}
	if cfg.FeedbackLaunchURL != "https://triage.example.com/feedback/start" {
		t.Fatalf("FeedbackLaunchURL = %q", cfg.FeedbackLaunchURL)
	}
	if cfg.FeedbackLaunchSecret != "launch-secret" || cfg.FeedbackLaunchAudience != "triage.feedback" || cfg.FeedbackLaunchProjectID != "neudrive" {
		t.Fatalf("feedback launch config not parsed correctly: %+v", cfg)
	}
	if cfg.FeedbackLaunchTTLSeconds != 120 {
		t.Fatalf("FeedbackLaunchTTLSeconds = %d, want 120", cfg.FeedbackLaunchTTLSeconds)
	}
}

func TestLoadWithOverridesDefaultsFeedbackEnabledFromSecretAndClampsTTL(t *testing.T) {
	cfg, err := LoadWithOverrides(map[string]string{
		"JWT_SECRET":                  "secret",
		"VAULT_MASTER_KEY":            "vault",
		"FEEDBACK_LAUNCH_SECRET":      "launch-secret",
		"FEEDBACK_LAUNCH_TTL_SECONDS": "999",
	})
	if err != nil {
		t.Fatalf("LoadWithOverrides: %v", err)
	}
	if !cfg.FeedbackEnabled {
		t.Fatal("FeedbackEnabled = false, want true when launch secret is configured")
	}
	if cfg.FeedbackLaunchTTLSeconds != 300 {
		t.Fatalf("FeedbackLaunchTTLSeconds = %d, want clamp to 300", cfg.FeedbackLaunchTTLSeconds)
	}
}
