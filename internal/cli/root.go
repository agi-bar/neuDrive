package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/agi-bar/neudrive/internal/api"
	"github.com/agi-bar/neudrive/internal/app/appcore"
	"github.com/agi-bar/neudrive/internal/app/mcpapp"
	"github.com/agi-bar/neudrive/internal/app/serverapp"
	"github.com/agi-bar/neudrive/internal/localgitsync"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/platforms"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
	"github.com/agi-bar/neudrive/internal/synccli"
)

func Run(args []string) int {
	if len(args) == 0 {
		printRootUsage()
		return 0
	}

	switch args[0] {
	case "help":
		return runHelp(args[1:])
	case "--help", "-h":
		printRootUsage()
		return 0
	case "server":
		return runServer(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "sync":
		return runSync(args[1:])
	case "login":
		return runLogin(args[1:])
	case "logout":
		return runLogout(args[1:])
	case "use":
		return runUse(args[1:])
	case "whoami":
		return runWhoAmICommand(args[1:])
	case "profiles":
		return runProfilesCommand(args[1:])
	case "browse":
		return runBrowse(args[1:])
	case "status":
		return runStatus(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "platform":
		return runPlatform(args[1:])
	case "ls":
		return runHubLS(args[1:])
	case "read":
		return runHubRead(args[1:])
	case "write":
		return runHubWrite(args[1:])
	case "search":
		return runHubSearch(args[1:])
	case "create":
		return runHubCreate(args[1:])
	case "log":
		return runHubLog(args[1:])
	case "connect":
		return runConnect(args[1:])
	case "disconnect":
		return runDisconnect(args[1:])
	case "import":
		return runHubImport(args[1:])
	case "export":
		return runExport(args[1:])
	case "token":
		return runHubToken(args[1:])
	case "stats":
		return runHubStats(args[1:])
	case "daemon":
		return runDaemon(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printRootUsage()
		return 2
	}
}

func runServer(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("server")
		return 0
	}
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listen := fs.String("listen", "127.0.0.1:42690", "listen address")
	storage := fs.String("storage", "", "storage backend: sqlite or postgres (default: postgres)")
	sqlitePath := fs.String("sqlite-path", "", "sqlite database path")
	databaseURL := fs.String("database-url", "", "database URL override")
	jwtSecret := fs.String("jwt-secret", "", "JWT secret override")
	vaultKey := fs.String("vault-master-key", "", "vault master key override")
	publicBaseURL := fs.String("public-base-url", "", "public base URL override")
	localMode := fs.Bool("local-mode", false, "run the server in local mode")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	selectedStorage := chooseStorageBackend(appcore.DefaultServerStorage, *storage, *sqlitePath, *databaseURL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serverapp.Run(ctx, serverapp.Options{
		Storage:        selectedStorage,
		LocalMode:      *localMode,
		SQLitePath:     *sqlitePath,
		ListenAddr:     *listen,
		DatabaseURL:    *databaseURL,
		JWTSecret:      *jwtSecret,
		VaultMasterKey: *vaultKey,
		PublicBaseURL:  *publicBaseURL,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%s server failed: %v\n", rootCommand(), err)
		return 1
	}
	return 0
}

func runMCP(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelpTopic("mcp")
		return 0
	}
	if args[0] != "stdio" {
		fmt.Fprintf(os.Stderr, "unknown mcp subcommand %q\n", args[0])
		return 2
	}
	fs := flag.NewFlagSet("mcp stdio", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	token := fs.String("token", "", "scoped access token")
	tokenEnv := fs.String("token-env", mcpapp.DefaultTokenEnvVar, "environment variable containing the scoped access token")
	storage := fs.String("storage", "", "storage backend: sqlite or postgres (default: sqlite)")
	sqlitePath := fs.String("sqlite-path", "", "sqlite database path")
	databaseURL := fs.String("database-url", "", "database URL override")
	jwtSecret := fs.String("jwt-secret", "", "JWT secret override")
	vaultKey := fs.String("vault-master-key", "", "vault master key override")
	publicBaseURL := fs.String("public-base-url", "", "public base URL override")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	selectedStorage := chooseStorageBackend(appcore.DefaultLocalStorage, *storage, *sqlitePath, *databaseURL)
	if err := mcpapp.RunStdio(context.Background(), mcpapp.Options{
		Storage:        selectedStorage,
		LocalMode:      true,
		SQLitePath:     *sqlitePath,
		Token:          *token,
		TokenEnv:       *tokenEnv,
		DatabaseURL:    *databaseURL,
		JWTSecret:      *jwtSecret,
		VaultMasterKey: *vaultKey,
		PublicBaseURL:  *publicBaseURL,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%s mcp stdio failed: %v\n", rootCommand(), err)
		return 1
	}
	return 0
}

func chooseStorageBackend(defaultStorage, explicitStorage, explicitSQLitePath, explicitDatabaseURL string) string {
	return appcore.ResolveStorageBackend(explicitStorage, explicitSQLitePath, explicitDatabaseURL, defaultStorage)
}

func runStatus(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("status")
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, usageLine("status"))
		return 2
	}
	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	_, state, err := runtimecfg.LoadState("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load runtime state: %v\n", err)
		return 1
	}
	daemonLine := "stopped"
	daemonURL := ""
	if state != nil {
		daemonURL = state.APIBase
		status := "unhealthy"
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := runtimecfg.HealthCheck(ctx, state.APIBase); err == nil {
			status = "running"
		}
		cancel()
		daemonLine = fmt.Sprintf("%s (%s, pid %d)", status, state.APIBase, state.PID)
	}
	fmt.Printf("Config: %s\n", configPath)
	fmt.Printf("Local daemon: %s\n", daemonLine)
	fmt.Printf("Local storage: %s\n", cfg.Local.Storage)
	if cfg.Local.Storage == "sqlite" && cfg.Local.SQLitePath != "" {
		fmt.Printf("Local SQLite DB: %s\n", cfg.Local.SQLitePath)
	}
	if cfg.Local.Storage != "sqlite" && cfg.Local.DatabaseURL != "" {
		fmt.Printf("Local database: %s\n", cfg.Local.DatabaseURL)
	}
	currentTarget := runtimecfg.SelectedTarget(cfg)
	fmt.Printf("Current target: %s\n", currentTarget)
	if profileName := runtimecfg.TargetProfileName(currentTarget); profileName != "" {
		fmt.Printf("Current hosted profile: %s\n", profileName)
		if profile, ok := cfg.Profiles[profileName]; ok {
			if strings.TrimSpace(profile.APIBase) != "" {
				fmt.Printf("Hosted API base: %s\n", strings.TrimRight(profile.APIBase, "/"))
			}
			fmt.Printf("Hosted auth mode: %s\n", defaultText(profile.AuthMode, runtimecfg.AuthModeScopedToken))
			if strings.TrimSpace(profile.ExpiresAt) != "" {
				fmt.Printf("Hosted token expires at: %s\n", profile.ExpiresAt)
			}
		}
	} else {
		fmt.Println("Current hosted profile: none")
	}
	fmt.Println()
	fmt.Println("Platforms:")
	for _, status := range platforms.AllStatuses(cfg, daemonURL) {
		fmt.Printf("- %s: installed=%t connected=%t\n", status.ID, status.Installed, status.Connected)
	}
	return 0
}

func runDoctor(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("doctor")
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, usageLine("doctor"))
		return 2
	}
	_, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	_, state, _ := runtimecfg.LoadState("")
	fmt.Println("Doctor:")
	fmt.Printf("- config file: %s\n", runtimecfg.DefaultConfigPath())
	if state != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := runtimecfg.HealthCheck(ctx, state.APIBase)
		cancel()
		if err == nil {
			fmt.Printf("- local daemon: healthy at %s\n", state.APIBase)
		} else {
			fmt.Printf("- local daemon: not healthy (%v)\n", err)
		}
	} else {
		fmt.Println("- local daemon: not running")
	}
	fmt.Printf("- local storage: %s\n", cfg.Local.Storage)
	if cfg.Local.Storage == "sqlite" && cfg.Local.SQLitePath != "" {
		fmt.Printf("- local sqlite path: %s\n", cfg.Local.SQLitePath)
	}
	if cfg.Local.Storage != "sqlite" && cfg.Local.DatabaseURL != "" {
		fmt.Printf("- local database url: %s\n", cfg.Local.DatabaseURL)
	} else {
		if cfg.Local.Storage != "sqlite" {
			fmt.Println("- local database url: not configured")
		}
	}
	if err := synccli.CheckDependencies(); err != nil {
		fmt.Printf("- sync runtime: %v\n", err)
	} else {
		fmt.Println("- sync runtime: native Go runtime available")
	}
	for _, status := range platforms.AllStatuses(cfg, "") {
		state := "missing"
		if status.Installed {
			state = status.BinaryPath
		}
		fmt.Printf("- platform %s: %s\n", status.ID, state)
	}
	if cfg.Local.Storage == "sqlite" && cfg.Local.DatabaseURL != "" {
		fmt.Println("- note: detected legacy local Postgres configuration; local SQLite starts from a new empty database unless you import/sync data explicitly")
	}
	return 0
}

func runPlatform(args []string) int {
	if len(args) == 0 || isHelpArg(args) {
		printHelpTopic("platform")
		return 0
	}
	switch args[0] {
	case "ls":
		return runPlatformLS(args[1:])
	case "show":
		return runPlatformShow(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown platform subcommand %q\n", args[0])
		return 2
	}
}

func runPlatformLS(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("platform ls")
		return 0
	}
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, usageLine("platform ls"))
		return 2
	}
	_, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	_, state, _ := runtimecfg.LoadState("")
	daemonURL := ""
	if state != nil {
		daemonURL = state.APIBase
	}
	for _, status := range platforms.AllStatuses(cfg, daemonURL) {
		fmt.Printf("%s\tinstalled=%t\tconnected=%t\tconfig=%s\n", status.ID, status.Installed, status.Connected, status.ConfigPath)
	}
	return 0
}

func runPlatformShow(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("platform show")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("platform show <platform>"))
		return 2
	}
	_, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	_, state, _ := runtimecfg.LoadState("")
	daemonURL := ""
	if state != nil {
		daemonURL = state.APIBase
	}
	adapter, err := platforms.Resolve(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	status := adapter.Detect(cfg, daemonURL)
	fmt.Printf("Platform: %s\n", status.DisplayName)
	fmt.Printf("ID: %s\n", status.ID)
	fmt.Printf("Installed: %t\n", status.Installed)
	fmt.Printf("Connected: %t\n", status.Connected)
	fmt.Printf("MCP installed: %t\n", status.MCPInstalled)
	if status.BinaryPath != "" {
		fmt.Printf("Binary: %s\n", status.BinaryPath)
	}
	if status.ConfigPath != "" {
		fmt.Printf("Config path: %s\n", status.ConfigPath)
	}
	if status.DaemonTarget != "" {
		fmt.Printf("Local daemon target: %s\n", status.DaemonTarget)
	}
	if status.EntrypointType != "" {
		fmt.Printf("Entrypoint installed: %t\n", status.EntrypointInstalled)
		fmt.Printf("Entrypoint type: %s\n", status.EntrypointType)
		if status.EntrypointPath != "" {
			fmt.Printf("Entrypoint path: %s\n", status.EntrypointPath)
		}
	}
	if len(status.ChatUsage) > 0 {
		fmt.Printf("Chat usage: %s\n", strings.Join(status.ChatUsage, ", "))
	}
	if status.AgentMediated != "" {
		fmt.Printf("Agent-mediated export: %s\n", status.AgentMediated)
	}
	fmt.Printf("Supported domains: %s\n", strings.Join(status.SupportedDomains, ", "))
	fmt.Println("Discovered sources:")
	if len(status.Sources) == 0 {
		fmt.Println("- none")
	} else {
		for _, source := range status.Sources {
			kind := "file"
			if source.IsDir {
				kind = "dir"
			}
			fmt.Printf("- [%s] %s (%s)\n", source.Domain, source.Path, kind)
		}
	}
	return 0
}

func runConnect(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("connect")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("connect <platform>"))
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable: %v\n", err)
		return 1
	}
	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	cfg, state, err := ensureCurrentLocalDaemon(ctx, executable, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start local daemon: %v\n", err)
		return 1
	}
	connection, err := platforms.EnsureConnection(ctx, cfg, args[0], executable, state.APIBase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect %s: %v\n", args[0], err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	adapter, _ := platforms.Resolve(args[0])
	status := adapter.Detect(cfg, state.APIBase)
	fmt.Printf("Connected %s to %s using %s transport.\n", args[0], state.APIBase, connection.Transport)
	if status.EntrypointType != "" {
		fmt.Printf("Entrypoint installed: %t", status.EntrypointInstalled)
		if status.EntrypointPath != "" {
			fmt.Printf(" (%s at %s)", status.EntrypointType, status.EntrypointPath)
		}
		fmt.Println()
	}
	if len(status.ChatUsage) > 0 {
		fmt.Printf("Chat usage: %s\n", strings.Join(status.ChatUsage, ", "))
	}
	return 0
}

func runDisconnect(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("disconnect")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("disconnect <platform>"))
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable: %v\n", err)
		return 1
	}
	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	cfg, _, err = ensureCurrentLocalDaemon(ctx, executable, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start local daemon: %v\n", err)
		return 1
	}
	if err := platforms.Disconnect(ctx, cfg, args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "disconnect %s: %v\n", args[0], err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	fmt.Printf("Disconnected %s and removed neuDrive managed entrypoints.\n", args[0])
	return 0
}

func runLegacyImport(args []string) int {
	if isHelpArg(args) || len(args) == 0 {
		fmt.Println(usageLine("import <platform> [--mode agent|files|all] [--zip FILE]"))
		return 0
	}
	platform := strings.TrimSpace(args[0])
	if platform == "" || strings.HasPrefix(platform, "-") {
		fmt.Fprintln(os.Stderr, usageLine("import <platform> [--mode agent|files|all] [--zip FILE]"))
		return 2
	}
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	mode := fs.String("mode", "", "import mode: agent, files, all")
	zipPath := fs.String("zip", "", "Claude skills zip exported from the web app")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, usageLine("import <platform> [--mode agent|files|all] [--zip FILE]"))
		return 2
	}
	if strings.TrimSpace(*zipPath) != "" && strings.TrimSpace(*mode) != "" && strings.TrimSpace(*mode) != string(platforms.ImportModeFiles) {
		fmt.Fprintln(os.Stderr, "--zip can only be combined with --mode files")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable: %v\n", err)
		return 1
	}
	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	cfg, _, err = ensureCurrentLocalDaemon(ctx, executable, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start local daemon: %v\n", err)
		return 1
	}
	var result *platforms.ImportSummary
	if strings.TrimSpace(*zipPath) != "" {
		result, err = platforms.ImportSkillsZip(ctx, cfg, platform, *zipPath)
	} else {
		result, err = platforms.Import(ctx, cfg, platform, *mode)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "import %s: %v\n", platform, err)
		return 1
	}
	switch {
	case strings.TrimSpace(*zipPath) != "" && result.Files != nil:
		fmt.Printf("Imported %d files (%d bytes) from %s into /skills using %s.\n",
			result.Files.Files,
			result.Files.Bytes,
			*zipPath,
			platform,
		)
	case result.Agent != nil && result.Files != nil:
		fmt.Printf("Imported %s using mode=%s: %d profile categories, %d memory items, %d projects, %d agent artifacts, plus %d files (%d bytes) into /platforms/%s.\n",
			platform,
			result.Mode,
			result.Agent.ProfileCategories,
			result.Agent.MemoryItems,
			result.Agent.Projects,
			result.Agent.Artifacts,
			result.Files.Files,
			result.Files.Bytes,
			result.Platform,
		)
	case result.Agent != nil:
		fmt.Printf("Imported %s using mode=%s: %d profile categories, %d memory items, %d projects, %d agent artifacts.\n",
			platform,
			result.Mode,
			result.Agent.ProfileCategories,
			result.Agent.MemoryItems,
			result.Agent.Projects,
			result.Agent.Artifacts,
		)
	case result.Files != nil:
		fmt.Printf("Imported %d files (%d bytes) from %s into /platforms/%s using mode=%s.\n",
			result.Files.Files,
			result.Files.Bytes,
			platform,
			result.Platform,
			result.Mode,
		)
	}
	printLocalGitSyncMessage(result.LocalGit)
	return 0
}

func runExport(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("export")
		return 0
	}
	platform := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && !isHelpArg(args[:1]) {
		platform = strings.TrimSpace(args[0])
		args = args[1:]
	}
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	output := fs.String("output", "", "output directory for staged platform materials")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if platform != "" {
		rest = append([]string{platform}, rest...)
	}
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("export <platform> [--output DIR]"))
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable: %v\n", err)
		return 1
	}
	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	cfg, _, err = ensureCurrentLocalDaemon(ctx, executable, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start local daemon: %v\n", err)
		return 1
	}
	result, err := platforms.ExportFromLocalHub(ctx, cfg, rest[0], *output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "export %s: %v\n", rest[0], err)
		return 1
	}
	fmt.Printf("Exported %d files (%d bytes) from /platforms/%s to %s.\n", result.Files, result.Bytes, result.Platform, result.OutputRoot)
	return 0
}

func runBrowse(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("browse")
		return 0
	}
	fs := flag.NewFlagSet("browse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	printURL := fs.Bool("print-url", false, "print the dashboard URL instead of opening a browser")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	route := "/"
	if fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, usageLine("browse [--print-url] [/route]"))
		return 2
	}
	if fs.NArg() == 1 {
		route = fs.Arg(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, state, err := ensureLocalOwnerAccess(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare local dashboard: %v\n", err)
		return 1
	}
	target, err := buildBrowseURL(state.APIBase, route, cfg.Local.OwnerToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build dashboard URL: %v\n", err)
		return 1
	}
	if *printURL {
		fmt.Println(target)
		return 0
	}
	fmt.Printf("Opening neuDrive dashboard:\n%s\n", target)
	if err := openBrowser(target); err != nil {
		fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		fmt.Println(target)
		return 1
	}
	return 0
}

func runFiles(args []string) int {
	if len(args) == 0 || isHelpArg(args) {
		fmt.Println(renderCLIText("Usage: neudrive files ls [path]\n       neudrive files cat <path>"))
		return 0
	}
	switch args[0] {
	case "ls":
		return runFilesLS(args[1:])
	case "cat":
		return runFilesCat(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown files subcommand %q\n", args[0])
		return 2
	}
}

func runFilesLS(args []string) int {
	if isHelpArg(args) {
		fmt.Println(usageLine("files ls [path]"))
		return 0
	}
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, usageLine("files ls [path]"))
		return 2
	}
	targetPath := "/"
	if len(args) == 1 {
		targetPath = normalizeHubPath(args[0])
	}
	if targetPath != "/" && !strings.HasSuffix(targetPath, "/") {
		targetPath += "/"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, state, token, err := ensureLocalOwnerAccessForAPI(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare local files view: %v\n", err)
		return 1
	}
	var node api.FileNode
	if err := localAPIGet(ctx, state.APIBase, token, "/agent/tree"+targetPath, &node); err != nil {
		fmt.Fprintf(os.Stderr, "files ls: %v\n", err)
		return 1
	}
	entries := node.Children
	if !node.IsDir {
		entries = []*api.FileNode{&node}
	}
	for _, entry := range entries {
		kind := "file"
		if entry.IsDir {
			kind = "dir"
		}
		fmt.Printf("%s\t%s\n", kind, entry.Path)
	}
	return 0
}

func runFilesCat(args []string) int {
	if isHelpArg(args) {
		fmt.Println(usageLine("files cat <path>"))
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, usageLine("files cat <path>"))
		return 2
	}
	targetPath := normalizeHubPath(args[0])
	if targetPath == "/" || strings.HasSuffix(targetPath, "/") {
		fmt.Fprintln(os.Stderr, "files cat expects a file path, not a directory")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, state, token, err := ensureLocalOwnerAccessForAPI(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare local files view: %v\n", err)
		return 1
	}
	var node api.FileNode
	if err := localAPIGet(ctx, state.APIBase, token, "/agent/tree"+targetPath, &node); err != nil {
		fmt.Fprintf(os.Stderr, "files cat: %v\n", err)
		return 1
	}
	if node.IsDir {
		fmt.Fprintln(os.Stderr, "files cat expects a file path, not a directory")
		return 2
	}
	if node.Content == "" && !isTextLikeContent(node.MimeType) {
		fmt.Fprintf(os.Stderr, "%s\n", renderCLIText(fmt.Sprintf("files cat: %s is a binary file (%s); use neudrive browse or export instead", node.Path, node.MimeType)))
		return 1
	}
	fmt.Print(node.Content)
	if node.Content != "" && !strings.HasSuffix(node.Content, "\n") {
		fmt.Println()
	}
	return 0
}

func runDaemon(args []string) int {
	if len(args) == 0 || isHelpArg(args) {
		printHelpTopic("daemon")
		return 0
	}
	switch args[0] {
	case "status":
		_, state, err := runtimecfg.LoadState("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "load runtime state: %v\n", err)
			return 1
		}
		if state == nil {
			fmt.Println("Local daemon is stopped.")
			return 0
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = runtimecfg.HealthCheck(ctx, state.APIBase)
		cancel()
		status := "unhealthy"
		if err == nil {
			status = "running"
		}
		fmt.Printf("Local daemon %s at %s (pid %d)\n", status, state.APIBase, state.PID)
		return 0
	case "stop":
		if err := runtimecfg.StopLocalDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "stop daemon: %v\n", err)
			return 1
		}
		fmt.Println("Local daemon stopped.")
		return 0
	case "logs":
		fs := flag.NewFlagSet("daemon logs", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		tail := fs.Int("tail", 50, "number of lines to show")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		_, state, err := runtimecfg.LoadState("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "load runtime state: %v\n", err)
			return 1
		}
		logPath := runtimecfg.DefaultLogPath()
		if state != nil && state.LogPath != "" {
			logPath = state.LogPath
		}
		content, err := runtimecfg.TailLog(logPath, *tail)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read logs: %v\n", err)
			return 1
		}
		fmt.Println(content)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown daemon subcommand %q\n", args[0])
		return 2
	}
}

func runSync(args []string) int {
	if len(args) == 0 || isHelpArg(args[:1]) {
		printHelpTopic("sync")
		return 0
	}
	envRestore := []func(){}
	defer func() {
		for i := len(envRestore) - 1; i >= 0; i-- {
			envRestore[i]()
		}
	}()

	if shouldUseLocalSyncDefaults(args) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		executable, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "prepare local sync defaults: resolve executable: %v\n", err)
			return 1
		}
		configPath, cfg, err := runtimecfg.LoadConfig("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "prepare local sync defaults: load config: %v\n", err)
			return 1
		}
		if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "prepare local sync defaults: prepare local defaults: %v\n", err)
			return 1
		}
		if err := saveConfig(configPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "prepare local sync defaults: save config: %v\n", err)
			return 1
		}
		cfg, state, err := ensureCurrentLocalDaemon(ctx, executable, configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "prepare local sync defaults: %v\n", err)
			return 1
		}
		envRestore = append(envRestore, setTempEnv("NEUDRIVE_SYNC_API_BASE", state.APIBase))
		envRestore = append(envRestore, setTempEnv("NEUDRIVE_API_BASE", state.APIBase))
		envRestore = append(envRestore, setTempEnv("NEUDRIVE_SYNC_TOKEN", cfg.Local.OwnerToken))
		envRestore = append(envRestore, setTempEnv("NEUDRIVE_TOKEN", cfg.Local.OwnerToken))
	}

	if err := synccli.Run(args); err != nil {
		var exitErr *synccli.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}
		fmt.Fprintf(os.Stderr, "%s sync failed: %v\n", rootCommand(), err)
		return 1
	}
	return 0
}

func saveConfig(path string, cfg *runtimecfg.CLIConfig) error {
	return runtimecfg.SaveConfig(path, cfg)
}

func ensureLocalOwnerAccess(ctx context.Context) (*runtimecfg.CLIConfig, *runtimecfg.RuntimeState, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		return nil, nil, err
	}
	if err := runtimecfg.EnsureLocalDefaults(cfg); err != nil {
		return nil, nil, err
	}
	if err := saveConfig(configPath, cfg); err != nil {
		return nil, nil, err
	}
	cfg, state, err := ensureCurrentLocalDaemon(ctx, executable, configPath)
	if err != nil {
		return nil, nil, err
	}
	return cfg, state, nil
}

func ensureLocalOwnerAccessForAPI(ctx context.Context) (*runtimecfg.CLIConfig, *runtimecfg.RuntimeState, string, error) {
	cfg, state, err := ensureLocalOwnerAccess(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, state, cfg.Local.OwnerToken, nil
}

func ensureOwnerToken(ctx context.Context, configPath string, cfg *runtimecfg.CLIConfig, apiBase string) error {
	if strings.TrimSpace(cfg.Local.OwnerToken) != "" {
		return nil
	}
	tokenResp, err := bootstrapLocalOwnerToken(ctx, apiBase)
	if err != nil {
		return err
	}
	cfg.Local.OwnerToken = tokenResp.Token
	cfg.Local.OwnerTokenID = tokenResp.ScopedToken.ID.String()
	cfg.Local.OwnerExpiresAt = tokenResp.ScopedToken.ExpiresAt.Format(time.RFC3339)
	return saveConfig(configPath, cfg)
}

func ensureUsableOwnerToken(ctx context.Context, configPath string, cfg *runtimecfg.CLIConfig, apiBase string) error {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ensureOwnerToken(ctx, configPath, cfg, apiBase); err != nil {
			return err
		}
		if err := validateOwnerToken(ctx, apiBase, cfg.Local.OwnerToken); err == nil {
			return nil
		}
		cfg.Local.OwnerToken = ""
		cfg.Local.OwnerTokenID = ""
		cfg.Local.OwnerExpiresAt = ""
		if err := saveConfig(configPath, cfg); err != nil {
			return err
		}
	}
	if err := ensureOwnerToken(ctx, configPath, cfg, apiBase); err != nil {
		return err
	}
	return validateOwnerToken(ctx, apiBase, cfg.Local.OwnerToken)
}

func bootstrapLocalOwnerToken(ctx context.Context, apiBase string) (*models.CreateTokenResponse, error) {
	var tokenResp models.CreateTokenResponse
	if err := localAPIPostJSON(ctx, apiBase, "", "/api/local/owner-token", nil, &tokenResp); err != nil {
		return nil, err
	}
	return &tokenResp, nil
}

func validateOwnerToken(ctx context.Context, apiBase, token string) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("missing local owner token")
	}
	return localAPIGet(ctx, apiBase, token, "/agent/auth/whoami", nil)
}

func ensureCurrentLocalDaemon(ctx context.Context, executable, configPath string) (*runtimecfg.CLIConfig, *runtimecfg.RuntimeState, error) {
	cfg, state, err := runtimecfg.EnsureLocalDaemon(ctx, executable, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := ensureUsableOwnerToken(ctx, configPath, cfg, state.APIBase); err == nil {
		return cfg, state, nil
	} else if !isLocalDaemonCompatibilityError(err) {
		return nil, nil, err
	}
	if err := runtimecfg.StopLocalDaemon(); err != nil {
		return nil, nil, fmt.Errorf("restart outdated local daemon: %w", err)
	}
	cfg, state, err = runtimecfg.EnsureLocalDaemon(ctx, executable, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := ensureUsableOwnerToken(ctx, configPath, cfg, state.APIBase); err != nil {
		return nil, nil, err
	}
	return cfg, state, nil
}

func syncLocalGitMirrorIfConfigured(ctx context.Context, _ *runtimecfg.CLIConfig) (*localgitsync.SyncInfo, error) {
	_, state, token, err := ensureLocalOwnerAccessForAPI(ctx)
	if err != nil {
		return nil, err
	}
	var info localgitsync.SyncInfo
	if err := localAPIPostJSON(ctx, state.APIBase, token, "/agent/local-git-mirror/sync", nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func printLocalGitSyncMessage(info *localgitsync.SyncInfo) {
	if info == nil || !info.Enabled || strings.TrimSpace(info.Message) == "" {
		return
	}
	if info.Synced {
		fmt.Println(info.Message)
		return
	}
	fmt.Fprintln(os.Stderr, info.Message)
}

func isLocalDaemonCompatibilityError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "404") ||
		strings.Contains(msg, "unexpected api response") ||
		strings.Contains(msg, "cannot unmarshal") ||
		strings.Contains(msg, "invalid character")
}

func shouldUseLocalSyncDefaults(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return false
	}
	if containsFlag(args[1:], "--help", "-h") {
		return false
	}
	switch args[0] {
	case "preview", "push", "pull", "resume", "history":
		if containsFlag(args[1:], "--local") {
			return true
		}
		if containsFlag(args[1:], "--profile") || containsFlag(args[1:], "--token") || containsFlag(args[1:], "--api-base") {
			return false
		}
		_, cfg, err := runtimecfg.LoadConfig("")
		return err == nil && runtimecfg.SelectedTarget(cfg) == runtimecfg.TargetLocal
	default:
		return false
	}
}

func containsFlag(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				return true
			}
		}
	}
	return false
}

type localAPIEnvelope struct {
	OK           bool                   `json:"ok"`
	Data         json.RawMessage        `json:"data"`
	LocalGitSync *localgitsync.SyncInfo `json:"local_git_sync,omitempty"`
	Error        struct {
		Message string `json:"message"`
	} `json:"error"`
}

func localAPIGet(ctx context.Context, apiBase, token, apiPath string, out any) error {
	_, err := localAPIJSONWithSync(ctx, http.MethodGet, apiBase, token, apiPath, nil, out)
	return err
}

func localAPIPostJSON(ctx context.Context, apiBase, token, apiPath string, requestBody any, out any) error {
	_, err := localAPIJSONWithSync(ctx, http.MethodPost, apiBase, token, apiPath, requestBody, out)
	return err
}

func localAPIPutJSON(ctx context.Context, apiBase, token, apiPath string, requestBody any, out any) error {
	_, err := localAPIJSONWithSync(ctx, http.MethodPut, apiBase, token, apiPath, requestBody, out)
	return err
}

func localAPIJSON(ctx context.Context, method, apiBase, token, apiPath string, requestBody any, out any) error {
	_, err := localAPIJSONWithSync(ctx, method, apiBase, token, apiPath, requestBody, out)
	return err
}

func localAPIJSONWithSync(ctx context.Context, method, apiBase, token, apiPath string, requestBody any, out any) (*localgitsync.SyncInfo, error) {
	fullURL, err := joinAPIURL(apiBase, apiPath)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var envelope localAPIEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		snippet := strings.TrimSpace(string(respBody))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		if snippet == "" {
			snippet = resp.Status
		}
		return nil, fmt.Errorf("unexpected API response (%s): %s", resp.Status, snippet)
	}
	if !envelope.OK {
		if envelope.Error.Message != "" {
			return envelope.LocalGitSync, errors.New(envelope.Error.Message)
		}
		return envelope.LocalGitSync, fmt.Errorf("unexpected API error (%s)", resp.Status)
	}
	if out == nil {
		return envelope.LocalGitSync, nil
	}
	return envelope.LocalGitSync, json.Unmarshal(envelope.Data, out)
}

func joinAPIURL(apiBase, apiPath string) (string, error) {
	base, err := url.Parse(strings.TrimRight(apiBase, "/"))
	if err != nil {
		return "", err
	}
	parsedPath, err := url.Parse(apiPath)
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + parsedPath.Path
	base.RawQuery = parsedPath.RawQuery
	return base.String(), nil
}

func buildBrowseURL(apiBase, route, token string) (string, error) {
	if strings.TrimSpace(route) == "" {
		route = "/"
	}
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	target, err := url.Parse(strings.TrimRight(apiBase, "/") + route)
	if err != nil {
		return "", err
	}
	query := target.Query()
	query.Set("local_token", token)
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func openBrowser(target string) error {
	if browser := strings.TrimSpace(os.Getenv("BROWSER")); browser != "" {
		return exec.Command(browser, target).Start()
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func normalizeHubPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}

func isTextLikeContent(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return mimeType == "" ||
		strings.HasPrefix(mimeType, "text/") ||
		strings.Contains(mimeType, "json") ||
		strings.Contains(mimeType, "xml") ||
		strings.Contains(mimeType, "javascript")
}

func setTempEnv(key, value string) func() {
	previous, had := os.LookupEnv(key)
	_ = os.Setenv(key, value)
	return func() {
		if had {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	}
}

func isHelpArg(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "--help", "-h", "help":
		return true
	default:
		return false
	}
}

func SelfContainedBinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "neudrive"
	}
	return filepath.Clean(exe)
}
