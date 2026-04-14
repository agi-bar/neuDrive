package runtimecfg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultLocalHost      = "127.0.0.1"
	DefaultPortStart      = 42690
	DefaultPortEnd        = 42719
	DefaultDatabaseURL    = "postgres://neudrive:neudrive_dev@localhost:5432/neudrive?sslmode=disable"
	DefaultStorage        = "sqlite"
	DefaultGitMirrorPath  = "./neudrive-export/git-mirror"
	DefaultRemoteOfficial = "https://neudrive.ai"
	ConfigEnv             = "NEUDRIVE_CONFIG"
)

type SyncProfile struct {
	APIBase   string   `json:"api_base,omitempty"`
	Token     string   `json:"token,omitempty"`
	ExpiresAt string   `json:"expires_at,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	Source    string   `json:"source,omitempty"`
}

type LocalConnection struct {
	Token           string   `json:"token,omitempty"`
	TokenID         string   `json:"token_id,omitempty"`
	TokenPrefix     string   `json:"token_prefix,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	MaxTrustLevel   int      `json:"max_trust_level,omitempty"`
	ConfigPath      string   `json:"config_path,omitempty"`
	Transport       string   `json:"transport,omitempty"`
	EntrypointType  string   `json:"entrypoint_type,omitempty"`
	EntrypointPath  string   `json:"entrypoint_path,omitempty"`
	ManagedPaths    []string `json:"managed_paths,omitempty"`
	ChatUsage       []string `json:"chat_usage,omitempty"`
	ConnectedAt     string   `json:"connected_at,omitempty"`
	LastPlatformURL string   `json:"last_platform_url,omitempty"`
}

type LocalConfig struct {
	ListenAddr     string                     `json:"listen_addr,omitempty"`
	Storage        string                     `json:"storage,omitempty"`
	SQLitePath     string                     `json:"sqlite_path,omitempty"`
	DatabaseURL    string                     `json:"database_url,omitempty"`
	GitMirrorPath  string                     `json:"git_mirror_path,omitempty"`
	JWTSecret      string                     `json:"jwt_secret,omitempty"`
	VaultMasterKey string                     `json:"vault_master_key,omitempty"`
	PublicBaseURL  string                     `json:"public_base_url,omitempty"`
	OwnerToken     string                     `json:"owner_token,omitempty"`
	OwnerTokenID   string                     `json:"owner_token_id,omitempty"`
	OwnerExpiresAt string                     `json:"owner_expires_at,omitempty"`
	Connections    map[string]LocalConnection `json:"connections,omitempty"`
}

type CLIConfig struct {
	Version        int                    `json:"version"`
	CurrentProfile string                 `json:"current_profile,omitempty"`
	Profiles       map[string]SyncProfile `json:"profiles,omitempty"`
	Local          LocalConfig            `json:"local,omitempty"`
}

type RuntimeState struct {
	PID        int    `json:"pid"`
	ListenAddr string `json:"listen_addr"`
	APIBase    string `json:"api_base"`
	LogPath    string `json:"log_path,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
}

func DefaultConfigPath() string {
	if override := strings.TrimSpace(os.Getenv(ConfigEnv)); override != "" {
		return expandUser(override)
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "NeuDrive", "config.json")
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			return filepath.Join(appData, "NeuDrive", "config.json")
		}
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(expandUser(xdg), "neudrive", "config.json")
	}
	return filepath.Join(home, ".config", "neudrive", "config.json")
}

func DefaultStatePath() string {
	dir := filepath.Dir(DefaultConfigPath())
	return filepath.Join(dir, "runtime.json")
}

func DefaultLogPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Logs", "NeuDrive", "daemon.log")
	case "windows":
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			return filepath.Join(localAppData, "NeuDrive", "daemon.log")
		}
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
		return filepath.Join(expandUser(xdg), "neudrive", "daemon.log")
	}
	return filepath.Join(home, ".local", "state", "neudrive", "daemon.log")
}

func DefaultSQLitePath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "NeuDrive", "local.db")
	case "windows":
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			return filepath.Join(localAppData, "NeuDrive", "local.db")
		}
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(expandUser(xdg), "neudrive", "local.db")
	}
	return filepath.Join(home, ".local", "share", "neudrive", "local.db")
}

func LoadConfig(path string) (string, *CLIConfig, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	cfg := &CLIConfig{
		Version:  2,
		Profiles: map[string]SyncProfile{},
		Local: LocalConfig{
			Connections: map[string]LocalConnection{},
		},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, cfg, nil
		}
		return path, nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return path, nil, err
	}
	if cfg.Version == 0 {
		cfg.Version = 2
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]SyncProfile{}
	}
	if cfg.Local.Connections == nil {
		cfg.Local.Connections = map[string]LocalConnection{}
	}
	return path, cfg, nil
}

func SaveConfig(path string, cfg *CLIConfig) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]SyncProfile{}
	}
	if cfg.Local.Connections == nil {
		cfg.Local.Connections = map[string]LocalConnection{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func LoadState(path string) (string, *RuntimeState, error) {
	if path == "" {
		path = DefaultStatePath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, nil, nil
		}
		return path, nil, err
	}
	var state RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return path, nil, err
	}
	return path, &state, nil
}

func SaveState(path string, state *RuntimeState) error {
	if state == nil {
		return fmt.Errorf("nil runtime state")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ClearState(path string) error {
	if path == "" {
		path = DefaultStatePath()
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func EnsureLocalDefaults(cfg *CLIConfig) error {
	if strings.TrimSpace(cfg.Local.Storage) == "" {
		cfg.Local.Storage = DefaultStorage
	}
	if cfg.Local.Storage == "sqlite" && strings.TrimSpace(cfg.Local.SQLitePath) == "" {
		cfg.Local.SQLitePath = DefaultSQLitePath()
	}
	if cfg.Local.Storage != "sqlite" && cfg.Local.DatabaseURL == "" {
		cfg.Local.DatabaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
		if cfg.Local.DatabaseURL == "" {
			cfg.Local.DatabaseURL = DefaultDatabaseURL
		}
	}
	if strings.TrimSpace(cfg.Local.GitMirrorPath) == "" {
		cfg.Local.GitMirrorPath = DefaultGitMirrorPath
	}
	if cfg.Local.JWTSecret == "" {
		secret, err := randomHex(32)
		if err != nil {
			return err
		}
		cfg.Local.JWTSecret = secret
	}
	if cfg.Local.VaultMasterKey == "" {
		key, err := randomHex(32)
		if err != nil {
			return err
		}
		cfg.Local.VaultMasterKey = key
	}
	if cfg.Local.Connections == nil {
		cfg.Local.Connections = map[string]LocalConnection{}
	}
	return nil
}

func EnsureLocalDaemon(ctx context.Context, executable string, extraEnv map[string]string) (*CLIConfig, *RuntimeState, error) {
	configPath, cfg, err := LoadConfig("")
	if err != nil {
		return nil, nil, err
	}
	if err := EnsureLocalDefaults(cfg); err != nil {
		return nil, nil, err
	}

	statePath, state, err := LoadState("")
	if err != nil {
		return nil, nil, err
	}
	if state != nil && state.APIBase != "" && HealthCheck(ctx, state.APIBase) == nil {
		return cfg, state, nil
	}

	port, err := choosePort(cfg.Local.ListenAddr)
	if err != nil {
		return nil, nil, err
	}
	listenAddr := fmt.Sprintf("%s:%d", DefaultLocalHost, port)
	apiBase := fmt.Sprintf("http://%s", listenAddr)
	cfg.Local.ListenAddr = listenAddr
	cfg.Local.PublicBaseURL = apiBase
	if err := SaveConfig(configPath, cfg); err != nil {
		return nil, nil, err
	}

	logPath := DefaultLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	defer logFile.Close()

	cmd := exec.Command(executable,
		"server",
		"--local-mode",
		"--listen", listenAddr,
		"--storage", cfg.Local.Storage,
		"--sqlite-path", cfg.Local.SQLitePath,
		"--database-url", cfg.Local.DatabaseURL,
		"--jwt-secret", cfg.Local.JWTSecret,
		"--vault-master-key", cfg.Local.VaultMasterKey,
		"--public-base-url", apiBase,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Env = append(cmd.Env,
		"CORS_ORIGINS="+apiBase,
		"PORT="+fmt.Sprintf("%d", port),
		"NEUDRIVE_LOCAL_MODE=1",
	)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)
	state = &RuntimeState{
		PID:        cmd.Process.Pid,
		ListenAddr: listenAddr,
		APIBase:    apiBase,
		LogPath:    logPath,
		StartedAt:  startedAt,
	}
	if err := SaveState(statePath, state); err != nil {
		return nil, nil, err
	}
	if err := waitForHealth(ctx, apiBase, 20*time.Second); err != nil {
		logTail, _ := TailLog(logPath, 12)
		if strings.TrimSpace(logTail) != "" {
			return nil, nil, fmt.Errorf("local daemon failed to become healthy at %s: %w\n\nRecent daemon log:\n%s", apiBase, err, logTail)
		}
		return nil, nil, fmt.Errorf("local daemon failed to become healthy at %s: %w", apiBase, err)
	}
	return cfg, state, nil
}

func StopLocalDaemon() error {
	statePath, state, err := LoadState("")
	if err != nil {
		return err
	}
	if state == nil || state.PID <= 0 {
		return nil
	}
	process, err := os.FindProcess(state.PID)
	if err == nil {
		if runtime.GOOS != "windows" {
			_ = process.Signal(syscall.SIGTERM)
		} else {
			_ = process.Kill()
		}
	}
	_ = ClearState(statePath)
	return nil
}

func HealthCheck(ctx context.Context, apiBase string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/api/health", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

func TailLog(path string, lines int) (string, error) {
	if lines <= 0 {
		lines = 50
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	chunks := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(chunks) > 0 && chunks[len(chunks)-1] == "" {
		chunks = chunks[:len(chunks)-1]
	}
	if len(chunks) > lines {
		chunks = chunks[len(chunks)-lines:]
	}
	return strings.Join(chunks, "\n"), nil
}

func waitForHealth(ctx context.Context, apiBase string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := HealthCheck(ctx, apiBase); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timeout waiting for health")
	}
	return lastErr
}

func choosePort(savedListenAddr string) (int, error) {
	if savedListenAddr != "" {
		if _, port, err := net.SplitHostPort(savedListenAddr); err == nil {
			if portNum, err := parsePort(port); err == nil && portAvailable(portNum) {
				return portNum, nil
			}
		}
	}
	for port := DefaultPortStart; port <= DefaultPortEnd; port++ {
		if portAvailable(port) {
			return port, nil
		}
	}
	port, err := chooseEphemeralPort()
	if err != nil {
		return 0, fmt.Errorf("no free local port in %d-%d and ephemeral fallback failed: %w", DefaultPortStart, DefaultPortEnd, err)
	}
	return port, nil
}

func portAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", DefaultLocalHost, port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func chooseEphemeralPort() (int, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:0", DefaultLocalHost))
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	_, rawPort, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return 0, err
	}
	return parsePort(rawPort)
}

func parsePort(raw string) (int, error) {
	var port int
	_, err := fmt.Sscanf(raw, "%d", &port)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func expandUser(value string) string {
	if strings.HasPrefix(value, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}
