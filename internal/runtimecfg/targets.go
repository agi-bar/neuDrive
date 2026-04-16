package runtimecfg

import "strings"

const (
	ConfigVersionCurrent = 3
	TargetLocal          = "local"
	TargetProfilePrefix  = "profile:"
	AuthModeScopedToken  = "scoped_token"
	AuthModeOAuthSession = "oauth_session"
)

func NormalizeTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return TargetLocal
	}
	if target == TargetLocal {
		return TargetLocal
	}
	if name := TargetProfileName(target); name != "" {
		return ProfileTarget(name)
	}
	return TargetLocal
}

func ProfileTarget(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return TargetLocal
	}
	return TargetProfilePrefix + name
}

func TargetProfileName(target string) string {
	target = strings.TrimSpace(target)
	if !strings.HasPrefix(target, TargetProfilePrefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(target, TargetProfilePrefix))
}

func SelectedTarget(cfg *CLIConfig) string {
	if cfg == nil {
		return TargetLocal
	}
	return NormalizeTarget(cfg.CurrentTarget)
}

func normalizeCLIConfig(cfg *CLIConfig) {
	if cfg == nil {
		return
	}
	if cfg.Version < ConfigVersionCurrent {
		cfg.Version = ConfigVersionCurrent
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]SyncProfile{}
	}
	if cfg.Local.Connections == nil {
		cfg.Local.Connections = map[string]LocalConnection{}
	}
	if strings.TrimSpace(cfg.CurrentTarget) == "" && strings.TrimSpace(cfg.CurrentProfile) != "" {
		cfg.CurrentTarget = ProfileTarget(cfg.CurrentProfile)
	}
	cfg.CurrentTarget = NormalizeTarget(cfg.CurrentTarget)
	if profileName := TargetProfileName(cfg.CurrentTarget); profileName != "" {
		cfg.CurrentProfile = profileName
	} else {
		cfg.CurrentProfile = ""
	}
}
