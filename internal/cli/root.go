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

	"github.com/agi-bar/agenthub/internal/app/mcpapp"
	"github.com/agi-bar/agenthub/internal/app/serverapp"
	"github.com/agi-bar/agenthub/internal/localhub"
	"github.com/agi-bar/agenthub/internal/localruntime"
	"github.com/agi-bar/agenthub/internal/localserver"
	"github.com/agi-bar/agenthub/internal/platforms"
	"github.com/agi-bar/agenthub/internal/synccli"
)

func Run(args []string) int {
	if len(args) == 0 {
		printRootUsage()
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		printRootUsage()
		return 0
	case "server":
		return runServer(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "sync":
		return runSync(args[1:])
	case "remote":
		return runRemote(args[1:])
	case "browse":
		return runBrowse(args[1:])
	case "status":
		return runStatus(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "platform":
		return runPlatform(args[1:])
	case "ls":
		if len(args) == 1 {
			return runPlatformLS(nil)
		}
		return runPlatformShow(args[1:])
	case "connect":
		return runConnect(args[1:])
	case "disconnect":
		return runDisconnect(args[1:])
	case "import":
		return runImport(args[1:])
	case "export":
		return runExport(args[1:])
	case "files":
		return runFiles(args[1:])
	case "daemon":
		return runDaemon(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printRootUsage()
		return 2
	}
}

func printRootUsage() {
	fmt.Print(`Agent Hub

Local-first AI hub and platform sync tool.

Usage:
  agenthub browse [/route]
  agenthub status
  agenthub doctor
  agenthub platform ls
  agenthub platform show <platform>
  agenthub ls [platform]
  agenthub connect <platform>
  agenthub disconnect <platform>
  agenthub import <platform> [--mode agent|files|all] [--zip FILE]
  agenthub export <platform> [--output DIR]
  agenthub files ls [path]
  agenthub files cat <path>
  agenthub sync <subcommand>
  agenthub remote <subcommand>
  agenthub daemon status|stop|logs
  agenthub server [flags]
  agenthub mcp stdio [flags]
`)
}

func runServer(args []string) int {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listen := fs.String("listen", "127.0.0.1:42690", "listen address")
	storage := fs.String("storage", "", "storage backend: sqlite or postgres")
	sqlitePath := fs.String("sqlite-path", "", "sqlite database path")
	databaseURL := fs.String("database-url", "", "database URL override")
	jwtSecret := fs.String("jwt-secret", "", "JWT secret override")
	vaultKey := fs.String("vault-master-key", "", "vault master key override")
	publicBaseURL := fs.String("public-base-url", "", "public base URL override")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	selectedStorage := chooseStorageBackend(*storage, *databaseURL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serverapp.Run(ctx, serverapp.Options{
		Storage:        selectedStorage,
		SQLitePath:     *sqlitePath,
		ListenAddr:     *listen,
		DatabaseURL:    *databaseURL,
		JWTSecret:      *jwtSecret,
		VaultMasterKey: *vaultKey,
		PublicBaseURL:  *publicBaseURL,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "agenthub server failed: %v\n", err)
		return 1
	}
	return 0
}

func runMCP(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: agenthub mcp stdio [--token TOKEN|--token-env ENV] [--database-url URL] [--jwt-secret SECRET] [--vault-master-key KEY] [--public-base-url URL]")
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
	storage := fs.String("storage", "", "storage backend: sqlite or postgres")
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
	selectedStorage := chooseStorageBackend(*storage, *databaseURL)
	if err := mcpapp.RunStdio(context.Background(), mcpapp.Options{
		Storage:        selectedStorage,
		SQLitePath:     *sqlitePath,
		Token:          *token,
		TokenEnv:       *tokenEnv,
		DatabaseURL:    *databaseURL,
		JWTSecret:      *jwtSecret,
		VaultMasterKey: *vaultKey,
		PublicBaseURL:  *publicBaseURL,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "agenthub mcp stdio failed: %v\n", err)
		return 1
	}
	return 0
}

func chooseStorageBackend(explicitStorage, explicitDatabaseURL string) string {
	selectedStorage := strings.ToLower(strings.TrimSpace(explicitStorage))
	if selectedStorage != "" {
		return selectedStorage
	}
	if strings.TrimSpace(explicitDatabaseURL) != "" {
		return "postgres"
	}
	if envDatabaseURL, ok := os.LookupEnv("DATABASE_URL"); ok && strings.TrimSpace(envDatabaseURL) != "" {
		return "postgres"
	}
	return "sqlite"
}

func runStatus(args []string) int {
	if isHelpArg(args) {
		fmt.Println("usage: agenthub status")
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: agenthub status")
		return 2
	}
	configPath, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := localruntime.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	_, state, err := localruntime.LoadState("")
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
		if err := localruntime.HealthCheck(ctx, state.APIBase); err == nil {
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
	if cfg.CurrentProfile != "" {
		fmt.Printf("Current remote profile: %s\n", cfg.CurrentProfile)
	} else {
		fmt.Println("Current remote profile: none")
	}
	fmt.Println()
	fmt.Println("Platforms:")
	for _, status := range platforms.AllStatuses(cfg, daemonURL) {
		fmt.Printf("- %s: installed=%t connected=%t\n", status.ID, status.Installed, status.Connected)
	}
	return 0
}

func runDoctor(args []string) int {
	if isHelpArg(args) {
		fmt.Println("usage: agenthub doctor")
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: agenthub doctor")
		return 2
	}
	_, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := localruntime.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	_, state, _ := localruntime.LoadState("")
	fmt.Println("Doctor:")
	fmt.Printf("- config file: %s\n", localruntime.DefaultConfigPath())
	if state != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := localruntime.HealthCheck(ctx, state.APIBase)
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
		fmt.Println("Usage: agenthub platform ls | show <platform>")
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
	if isHelpArg(args) {
		fmt.Println("usage: agenthub platform ls")
		return 0
	}
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: agenthub platform ls")
		return 2
	}
	_, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	_, state, _ := localruntime.LoadState("")
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
	if isHelpArg(args) {
		fmt.Println("usage: agenthub platform show <platform>")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: agenthub platform show <platform>")
		return 2
	}
	_, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	_, state, _ := localruntime.LoadState("")
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
	if isHelpArg(args) {
		fmt.Println("usage: agenthub connect <platform>")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: agenthub connect <platform>")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable: %v\n", err)
		return 1
	}
	configPath, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := localruntime.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	cfg, state, err := localruntime.EnsureLocalDaemon(ctx, executable, nil)
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
	if isHelpArg(args) {
		fmt.Println("usage: agenthub disconnect <platform>")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: agenthub disconnect <platform>")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	configPath, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
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
	fmt.Printf("Disconnected %s and removed Agent Hub managed entrypoints.\n", args[0])
	return 0
}

func runImport(args []string) int {
	if isHelpArg(args) || len(args) == 0 {
		fmt.Println("usage: agenthub import <platform> [--mode agent|files|all] [--zip FILE]")
		return 0
	}
	platform := strings.TrimSpace(args[0])
	if platform == "" || strings.HasPrefix(platform, "-") {
		fmt.Fprintln(os.Stderr, "usage: agenthub import <platform> [--mode agent|files|all] [--zip FILE]")
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
		fmt.Fprintln(os.Stderr, "usage: agenthub import <platform> [--mode agent|files|all] [--zip FILE]")
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
	configPath, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := localruntime.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	cfg, _, err = localruntime.EnsureLocalDaemon(ctx, executable, nil)
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
	if strings.TrimSpace(*zipPath) != "" && result.Files != nil {
		fmt.Printf("Imported %d files (%d bytes) from %s into /skills using %s.\n",
			result.Files.Files,
			result.Files.Bytes,
			*zipPath,
			platform,
		)
		return 0
	}
	switch {
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
	return 0
}

func runExport(args []string) int {
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
		fmt.Fprintln(os.Stderr, "usage: agenthub export <platform> [--output DIR]")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable: %v\n", err)
		return 1
	}
	configPath, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if err := localruntime.EnsureLocalDefaults(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "prepare local defaults: %v\n", err)
		return 1
	}
	if err := saveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	cfg, _, err = localruntime.EnsureLocalDaemon(ctx, executable, nil)
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
		fmt.Fprintln(os.Stderr, "usage: agenthub browse [--print-url] [/route]")
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
	fmt.Printf("Opening Agent Hub dashboard:\n%s\n", target)
	if err := openBrowser(target); err != nil {
		fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
		fmt.Println(target)
		return 1
	}
	return 0
}

func runFiles(args []string) int {
	if len(args) == 0 || isHelpArg(args) {
		fmt.Println("Usage: agenthub files ls [path]\n       agenthub files cat <path>")
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
		fmt.Println("usage: agenthub files ls [path]")
		return 0
	}
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: agenthub files ls [path]")
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
	var node localserver.FileNode
	if err := localAPIGet(ctx, state.APIBase, token, "/agent/tree"+targetPath, &node); err != nil {
		fmt.Fprintf(os.Stderr, "files ls: %v\n", err)
		return 1
	}
	entries := node.Children
	if !node.IsDir {
		entries = []*localserver.FileNode{&node}
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
		fmt.Println("usage: agenthub files cat <path>")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: agenthub files cat <path>")
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
	var node localserver.FileNode
	if err := localAPIGet(ctx, state.APIBase, token, "/agent/tree"+targetPath, &node); err != nil {
		fmt.Fprintf(os.Stderr, "files cat: %v\n", err)
		return 1
	}
	if node.IsDir {
		fmt.Fprintln(os.Stderr, "files cat expects a file path, not a directory")
		return 2
	}
	if node.Content == "" && !isTextLikeContent(node.MimeType) {
		fmt.Fprintf(os.Stderr, "files cat: %s is a binary file (%s); use agenthub browse or export instead\n", node.Path, node.MimeType)
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
		fmt.Println("Usage: agenthub daemon status|stop|logs")
		return 0
	}
	switch args[0] {
	case "status":
		_, state, err := localruntime.LoadState("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "load runtime state: %v\n", err)
			return 1
		}
		if state == nil {
			fmt.Println("Local daemon is stopped.")
			return 0
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = localruntime.HealthCheck(ctx, state.APIBase)
		cancel()
		status := "unhealthy"
		if err == nil {
			status = "running"
		}
		fmt.Printf("Local daemon %s at %s (pid %d)\n", status, state.APIBase, state.PID)
		return 0
	case "stop":
		if err := localruntime.StopLocalDaemon(); err != nil {
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
		_, state, err := localruntime.LoadState("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "load runtime state: %v\n", err)
			return 1
		}
		logPath := localruntime.DefaultLogPath()
		if state != nil && state.LogPath != "" {
			logPath = state.LogPath
		}
		content, err := localruntime.TailLog(logPath, *tail)
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
	if len(args) == 0 {
		fmt.Println("Usage: agenthub sync login|profiles|use|whoami|logout|export|preview|push|pull|resume|history|diff")
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
		if err == nil {
			configPath, cfg, loadErr := localruntime.LoadConfig("")
			if loadErr == nil {
				if err := localruntime.EnsureLocalDefaults(cfg); err == nil {
					_ = saveConfig(configPath, cfg)
					cfg, state, ensureErr := ensureCurrentLocalDaemon(ctx, executable, configPath)
					if ensureErr == nil {
						envRestore = append(envRestore, setTempEnv("AGENTHUB_SYNC_API_BASE", state.APIBase))
						envRestore = append(envRestore, setTempEnv("AGENTHUB_API_BASE", state.APIBase))
						envRestore = append(envRestore, setTempEnv("AGENTHUB_SYNC_TOKEN", cfg.Local.OwnerToken))
						envRestore = append(envRestore, setTempEnv("AGENTHUB_TOKEN", cfg.Local.OwnerToken))
					}
				}
			}
		}
	}

	if err := synccli.Run(args); err != nil {
		var exitErr *synccli.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}
		fmt.Fprintf(os.Stderr, "agenthub sync failed: %v\n", err)
		return 1
	}
	return 0
}

func runRemote(args []string) int {
	if len(args) == 0 || isHelpArg(args) {
		fmt.Println("Usage: agenthub remote ls|login|use|logout|whoami")
		return 0
	}
	switch args[0] {
	case "ls":
		_, cfg, err := localruntime.LoadConfig("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "load config: %v\n", err)
			return 1
		}
		if len(cfg.Profiles) == 0 {
			fmt.Println("No remote profiles configured.")
			return 0
		}
		for name, profile := range cfg.Profiles {
			marker := " "
			if name == cfg.CurrentProfile {
				marker = "*"
			}
			fmt.Printf("%s %s\t%s\n", marker, name, profile.APIBase)
		}
		return 0
	case "login":
		if len(args) > 1 && isHelpArg(args[1:]) {
			fmt.Println("usage: agenthub remote login <profile> [--url URL] [--token TOKEN]")
			return 0
		}
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: agenthub remote login <profile> [--url URL] [--token TOKEN]")
			return 2
		}
		profileName := args[1]
		loginArgs := []string{"login", "--profile", profileName}
		if !containsFlag(args[2:], "--api-base") && !containsFlag(args[2:], "--url") {
			loginArgs = append(loginArgs, "--api-base", localruntime.DefaultRemoteOfficial)
		}
		loginArgs = append(loginArgs, normalizeRemoteArgs(args[2:])...)
		return runSync(loginArgs)
	case "use":
		if len(args) > 1 && isHelpArg(args[1:]) {
			fmt.Println("usage: agenthub remote use <profile>")
			return 0
		}
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: agenthub remote use <profile>")
			return 2
		}
		return runSync([]string{"use", "--profile", args[1]})
	case "logout":
		if len(args) > 1 && isHelpArg(args[1:]) {
			fmt.Println("usage: agenthub remote logout [profile]")
			return 0
		}
		logoutArgs := []string{"logout"}
		if len(args) == 2 {
			logoutArgs = append(logoutArgs, "--profile", args[1])
		}
		return runSync(logoutArgs)
	case "whoami":
		if len(args) > 1 && isHelpArg(args[1:]) {
			fmt.Println("usage: agenthub remote whoami [profile]")
			return 0
		}
		whoamiArgs := []string{"whoami"}
		if len(args) == 2 {
			whoamiArgs = append(whoamiArgs, "--profile", args[1])
		}
		return runSync(whoamiArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown remote subcommand %q\n", args[0])
		return 2
	}
}

func saveConfig(path string, cfg *localruntime.CLIConfig) error {
	return localruntime.SaveConfig(path, cfg)
}

func ensureLocalOwnerAccess(ctx context.Context) (*localruntime.CLIConfig, *localruntime.RuntimeState, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	configPath, cfg, err := localruntime.LoadConfig("")
	if err != nil {
		return nil, nil, err
	}
	if err := localruntime.EnsureLocalDefaults(cfg); err != nil {
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

func ensureLocalOwnerAccessForAPI(ctx context.Context) (*localruntime.CLIConfig, *localruntime.RuntimeState, string, error) {
	cfg, state, err := ensureLocalOwnerAccess(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, state, cfg.Local.OwnerToken, nil
}

func ensureOwnerToken(ctx context.Context, configPath string, cfg *localruntime.CLIConfig) error {
	if strings.TrimSpace(cfg.Local.OwnerToken) != "" {
		return nil
	}
	hub, err := localhub.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer hub.Close()
	tokenResp, err := hub.CreateOwnerToken(ctx)
	if err != nil {
		return err
	}
	cfg.Local.OwnerToken = tokenResp.Token
	cfg.Local.OwnerTokenID = tokenResp.ScopedToken.ID.String()
	cfg.Local.OwnerExpiresAt = tokenResp.ScopedToken.ExpiresAt.Format(time.RFC3339)
	return saveConfig(configPath, cfg)
}

func ensureUsableOwnerToken(ctx context.Context, configPath string, cfg *localruntime.CLIConfig, apiBase string) error {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ensureOwnerToken(ctx, configPath, cfg); err != nil {
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
	if err := ensureOwnerToken(ctx, configPath, cfg); err != nil {
		return err
	}
	return validateOwnerToken(ctx, apiBase, cfg.Local.OwnerToken)
}

func validateOwnerToken(ctx context.Context, apiBase, token string) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("missing local owner token")
	}
	return localAPIGet(ctx, apiBase, token, "/agent/auth/whoami", nil)
}

func ensureCurrentLocalDaemon(ctx context.Context, executable, configPath string) (*localruntime.CLIConfig, *localruntime.RuntimeState, error) {
	cfg, state, err := localruntime.EnsureLocalDaemon(ctx, executable, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := ensureUsableOwnerToken(ctx, configPath, cfg, state.APIBase); err == nil {
		return cfg, state, nil
	} else if !isLocalDaemonCompatibilityError(err) {
		return nil, nil, err
	}
	if err := localruntime.StopLocalDaemon(); err != nil {
		return nil, nil, fmt.Errorf("restart outdated local daemon: %w", err)
	}
	cfg, state, err = localruntime.EnsureLocalDaemon(ctx, executable, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := ensureUsableOwnerToken(ctx, configPath, cfg, state.APIBase); err != nil {
		return nil, nil, err
	}
	return cfg, state, nil
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
	switch args[0] {
	case "login", "profiles", "use", "logout":
		return false
	case "whoami":
		return !containsFlag(args[1:], "--profile")
	default:
		if containsFlag(args[1:], "--help", "-h") {
			return false
		}
		return !containsFlag(args[1:], "--profile") && !containsFlag(args[1:], "--token") && !containsFlag(args[1:], "--api-base")
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

func normalizeRemoteArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--url" {
			out = append(out, "--api-base")
			continue
		}
		out = append(out, args[i])
	}
	return out
}

type localAPIEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func localAPIGet(ctx context.Context, apiBase, token, apiPath string, out any) error {
	fullURL, err := joinAPIURL(apiBase, apiPath)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var envelope localAPIEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		if snippet == "" {
			snippet = resp.Status
		}
		return fmt.Errorf("unexpected API response (%s): %s", resp.Status, snippet)
	}
	if !envelope.OK {
		if envelope.Error.Message != "" {
			return errors.New(envelope.Error.Message)
		}
		return fmt.Errorf("unexpected API error (%s)", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

func joinAPIURL(apiBase, apiPath string) (string, error) {
	base, err := url.Parse(strings.TrimRight(apiBase, "/"))
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + apiPath
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
		return "agenthub"
	}
	return filepath.Clean(exe)
}
