package synccli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/runtimecfg"
)

const (
	fallbackAPIBase = "http://localhost:8080"
	loginTimeout    = 5 * time.Minute
	syncConfigEnv   = "AGENTHUB_SYNC_CONFIG"
	syncProfileEnv  = "AGENTHUB_SYNC_PROFILE"
	syncAPIBaseEnv  = "AGENTHUB_SYNC_API_BASE"
	syncTokenEnv    = "AGENTHUB_SYNC_TOKEN"
)

var (
	apiBaseEnvs = []string{syncAPIBaseEnv, "AGENTHUB_API_BASE"}
	tokenEnvs   = []string{syncTokenEnv, "AGENTHUB_TOKEN"}
)

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("sync command exited with code %d", e.Code)
}

type stringListFlag []string

func (s *stringListFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringListFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type commonOptions struct {
	Token      string
	APIBase    string
	Profile    string
	ConfigPath string
}

type sessionState struct {
	APIBase            string `json:"api_base"`
	BundlePath         string `json:"bundle_path"`
	SessionID          string `json:"session_id"`
	PreviewFingerprint string `json:"preview_fingerprint"`
	Profile            string `json:"profile"`
	CreatedAt          string `json:"created_at"`
}

type loginCallbackPayload struct {
	State     string   `json:"state"`
	Profile   string   `json:"profile"`
	Token     string   `json:"token"`
	ExpiresAt string   `json:"expires_at"`
	APIBase   string   `json:"api_base"`
	Scopes    []string `json:"scopes"`
	Usage     string   `json:"usage"`
}

func CheckDependencies() error {
	return nil
}

func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "help", "--help", "-h":
		printUsage()
		return nil
	case "login":
		return runLogin(args[1:])
	case "profiles":
		return runProfiles(args[1:])
	case "use":
		return runUse(args[1:])
	case "whoami":
		return runWhoAmI(args[1:])
	case "logout":
		return runLogout(args[1:])
	case "export":
		return runExport(args[1:])
	case "preview":
		return runPreview(args[1:])
	case "push":
		return runPush(args[1:])
	case "pull":
		return runPull(args[1:])
	case "resume":
		return runResume(args[1:])
	case "history":
		return runHistory(args[1:])
	case "diff":
		return runDiff(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown sync subcommand %q\n", args[0])
		printUsage()
		return &ExitError{Code: 2}
	}
}

func printUsage() {
	fmt.Println("Usage: agenthub sync login|profiles|use|whoami|logout|export|preview|push|pull|resume|history|diff")
}

func runLogin(args []string) error {
	fs := flag.NewFlagSet("sync login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	token := fs.String("token", "", "existing sync scoped token")
	apiBase := fs.String("api-base", "", "Agent Hub base URL")
	profile := fs.String("profile", "", "profile name to save")
	configPath := fs.String("config", "", "override config path")
	access := fs.String("access", "both", "push, pull, or both")
	ttlMinutes := fs.Int("ttl-minutes", 30, "token TTL in minutes")
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}
	if *ttlMinutes <= 0 {
		*ttlMinutes = 30
	}
	configPathValue, cfg, err := loadCLIConfig(*configPath)
	if err != nil {
		return err
	}
	apiBaseValue := resolveAPIBase(commonOptions{APIBase: *apiBase}, cfg, nil)
	profileName := pickProfileName(cfg, strings.TrimSpace(*profile), apiBaseValue)
	source := "manual"
	callbackPayload := loginCallbackPayload{}
	tokenValue := strings.TrimSpace(*token)
	if tokenValue == "" {
		source = "browser"
		callbackPayload, err = waitForBrowserLogin(apiBaseValue, profileName, strings.ToLower(strings.TrimSpace(*access)), *ttlMinutes)
		if err != nil {
			return err
		}
		tokenValue = strings.TrimSpace(callbackPayload.Token)
		if tokenValue == "" {
			return errors.New("browser login did not return a token")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := newClient(apiBaseValue, tokenValue).getAuthInfo(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(info.APIBase) == "" {
		info.APIBase = defaultString(callbackPayload.APIBase, apiBaseValue)
	}
	if info.ExpiresAt == nil && strings.TrimSpace(callbackPayload.ExpiresAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, callbackPayload.ExpiresAt); err == nil {
			info.ExpiresAt = &parsed
		}
	}
	if len(info.Scopes) == 0 && len(callbackPayload.Scopes) > 0 {
		info.Scopes = append([]string{}, callbackPayload.Scopes...)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]runtimecfg.SyncProfile{}
	}
	profileEntry := runtimecfg.SyncProfile{
		APIBase:   strings.TrimRight(info.APIBase, "/"),
		Token:     tokenValue,
		Scopes:    append([]string{}, info.Scopes...),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    source,
	}
	if info.ExpiresAt != nil {
		profileEntry.ExpiresAt = info.ExpiresAt.UTC().Format(time.RFC3339)
	}
	cfg.Profiles[profileName] = profileEntry
	cfg.CurrentProfile = profileName
	if err := runtimecfg.SaveConfig(configPathValue, cfg); err != nil {
		return err
	}
	printLoginSummary(profileName, profileEntry.APIBase, info)
	return nil
}

func runProfiles(args []string) error {
	fs := flag.NewFlagSet("sync profiles", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "override config path")
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}
	_, cfg, err := loadCLIConfig(*configPath)
	if err != nil {
		return err
	}
	fmt.Println(renderProfiles(cfg))
	return nil
}

func runUse(args []string) error {
	fs := flag.NewFlagSet("sync use", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profileFlag := fs.String("profile", "", "profile name")
	configPath := fs.String("config", "", "override config path")
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}
	profileName := strings.TrimSpace(*profileFlag)
	if profileName == "" && len(fs.Args()) > 0 {
		profileName = strings.TrimSpace(fs.Args()[0])
	}
	if profileName == "" {
		return errors.New("profile name is required")
	}
	configPathValue, cfg, err := loadCLIConfig(*configPath)
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[profileName]; !ok {
		return fmt.Errorf("profile %s does not exist", profileName)
	}
	cfg.CurrentProfile = profileName
	if err := runtimecfg.SaveConfig(configPathValue, cfg); err != nil {
		return err
	}
	fmt.Printf("Current profile: %s\n", profileName)
	return nil
}

func runWhoAmI(args []string) error {
	opts, err := parseCommonOptions("sync whoami", args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_, _, apiBaseValue, tokenValue, profileName, tokenSource, err := resolveRuntimeAuth(opts, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := newClient(apiBaseValue, tokenValue).getAuthInfo(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Current profile: %s\n", defaultString(profileName, "-"))
	fmt.Printf("API base: %s\n", strings.TrimRight(defaultString(info.APIBase, apiBaseValue), "/"))
	if strings.TrimSpace(info.UserSlug) != "" {
		fmt.Printf("User: %s\n", info.UserSlug)
	}
	fmt.Printf("Auth mode: %s\n", defaultString(info.AuthMode, "scoped_token"))
	fmt.Printf("Trust level: %d\n", info.TrustLevel)
	fmt.Printf("Token source: %s\n", tokenSource)
	if info.ExpiresAt != nil {
		fmt.Printf("Token expires at: %s\n", info.ExpiresAt.UTC().Format(time.RFC3339))
	} else {
		fmt.Println("Token expires at: -")
	}
	if len(info.Scopes) == 0 {
		fmt.Println("Scopes: -")
	} else {
		fmt.Printf("Scopes: %s\n", strings.Join(info.Scopes, ", "))
	}
	return nil
}

func runLogout(args []string) error {
	fs := flag.NewFlagSet("sync logout", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "profile name")
	configPath := fs.String("config", "", "override config path")
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}
	configPathValue, cfg, err := loadCLIConfig(*configPath)
	if err != nil {
		return err
	}
	profileName := strings.TrimSpace(*profile)
	if profileName == "" {
		profileName = selectedProfileName("", cfg)
	}
	if profileName == "" {
		return errors.New("no profile selected; pass --profile")
	}
	entry, ok := cfg.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %s does not exist", profileName)
	}
	entry.Token = ""
	entry.ExpiresAt = ""
	entry.Scopes = nil
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cfg.Profiles[profileName] = entry
	if err := runtimecfg.SaveConfig(configPathValue, cfg); err != nil {
		return err
	}
	fmt.Printf("Logged out profile %s\n", profileName)
	return nil
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("sync export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	source := fs.String("source", "", "directory containing skill subdirectories")
	mode := fs.String("mode", "merge", "merge or mirror")
	format := fs.String("format", "json", "json or archive")
	output := fs.String("output", "backup.ahub", "output bundle path")
	fs.StringVar(output, "o", "backup.ahub", "output bundle path")
	filters, err := addFilterFlags(fs)
	if err != nil {
		return err
	}
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}
	if strings.TrimSpace(*source) == "" {
		return errors.New("--source is required")
	}
	if !validMode(*mode) {
		return errors.New("mode must be merge or mirror")
	}
	bundle, err := buildBundle(*source, *mode)
	if err != nil {
		return err
	}
	filtered := applyFiltersToBundle(*bundle, filters.toModel())
	printBundleStats(filtered)
	outputPath := *output
	if *format == models.BundleFormatArchive {
		archive, manifest, err := buildArchive(filtered, filters.toModel())
		if err != nil {
			return err
		}
		if err := os.WriteFile(outputPath, archive, 0o644); err != nil {
			return err
		}
		printJSON(map[string]any{"manifest": manifest, "bytes": len(archive)})
		fmt.Printf("saved export to %s\n", outputPath)
		return nil
	}
	if *format != models.BundleFormatJSON {
		return errors.New("format must be json or archive")
	}
	if err := writePrettyJSON(outputPath, filtered); err != nil {
		return err
	}
	fmt.Printf("saved export to %s\n", outputPath)
	return nil
}

func runPreview(args []string) error {
	opts, filters, input, err := parseInputCommand("sync preview", args, true)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_, _, apiBaseValue, tokenValue, _, _, err := resolveRuntimeAuth(opts, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := newClient(apiBaseValue, tokenValue)
	var preview *models.BundlePreviewResult
	if input.Kind == models.BundleFormatArchive {
		preview, err = client.previewBundle(ctx, nil, input.Manifest)
	} else {
		bundle := applyFiltersToBundle(*input.Bundle, filters)
		preview, err = client.previewBundle(ctx, &bundle, nil)
	}
	if err != nil {
		return err
	}
	printJSON(preview)
	return nil
}

func runPush(args []string) error {
	opts, filters, input, err := parseInputCommand("sync push", args, true)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	fs := flag.NewFlagSet("sync push transport", flag.ContinueOnError)
	// no-op; parseInputCommand already consumed flags
	_ = fs
	transport := input.Transport
	if transport == "" {
		transport = models.SyncTransportAuto
	}
	_, _, apiBaseValue, tokenValue, profileName, _, err := resolveRuntimeAuth(opts, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := newClient(apiBaseValue, tokenValue)

	switch transport {
	case models.SyncTransportAuto:
		if input.Kind == models.BundleFormatArchive {
			transport = models.SyncTransportArchive
		} else {
			encoded, err := json.Marshal(input.Bundle)
			if err != nil {
				return err
			}
			if len(encoded) <= models.DefaultArchiveAutoThreshold {
				transport = models.SyncTransportJSON
			} else {
				transport = models.SyncTransportArchive
			}
		}
	case models.SyncTransportJSON, models.SyncTransportArchive:
	default:
		return errors.New("transport must be auto, json, or archive")
	}

	if transport == models.SyncTransportJSON {
		result, err := client.importBundle(ctx, applyFiltersToBundle(*input.Bundle, filters))
		if err != nil {
			return err
		}
		printJSON(result)
		return nil
	}

	archiveBytes := input.ArchiveBytes
	manifest := input.Manifest
	if len(archiveBytes) == 0 || manifest == nil {
		bundle := applyFiltersToBundle(*input.Bundle, filters)
		archiveBytes, manifest, err = buildArchive(bundle, filters)
		if err != nil {
			return err
		}
	}
	bundlePath := input.BundlePath
	if !strings.HasSuffix(strings.ToLower(bundlePath), ".ahubz") {
		stem := "bundle"
		if input.SourceDir != "" {
			stem = filepath.Base(filepath.Clean(input.SourceDir))
		} else if bundlePath != "" {
			stem = strings.TrimSuffix(filepath.Base(bundlePath), filepath.Ext(bundlePath))
		}
		bundlePath = filepath.Join(".", stem+".ahubz")
		if err := os.WriteFile(bundlePath, archiveBytes, 0o644); err != nil {
			return err
		}
	}
	sessionFile := input.SessionFile
	if sessionFile == "" {
		sessionFile = defaultSessionFile(bundlePath)
	}
	session, err := client.startSyncSession(ctx, models.SyncStartSessionRequest{
		TransportVersion: models.SyncTransportVersionV1,
		Format:           models.BundleFormatArchive,
		Mode:             defaultString(manifest.Mode, input.Mode),
		Manifest:         *manifest,
		ArchiveSizeBytes: int64(len(archiveBytes)),
		ArchiveSHA256:    manifest.ArchiveSHA256,
	})
	if err != nil {
		return err
	}
	if err := saveSessionFile(sessionFile, sessionState{
		APIBase:            apiBaseValue,
		BundlePath:         bundlePath,
		SessionID:          session.SessionID.String(),
		PreviewFingerprint: "",
		Profile:            profileName,
		CreatedAt:          time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return err
	}
	state, err := client.resumeSession(ctx, session.SessionID.String(), archiveBytes)
	if err != nil {
		return err
	}
	if state.Status != models.SyncSessionStatusReady {
		state, err = client.getSyncSession(ctx, session.SessionID.String())
		if err != nil {
			return err
		}
	}
	result, err := client.commitSession(ctx, session.SessionID.String(), "")
	if err != nil {
		return err
	}
	_ = os.Remove(sessionFile)
	printJSON(result)
	return nil
}

func runPull(args []string) error {
	opts, filters, err := parsePullFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_, _, apiBaseValue, tokenValue, _, _, err := resolveRuntimeAuth(opts.commonOptions, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := newClient(apiBaseValue, tokenValue)
	if opts.Format == models.BundleFormatArchive {
		archive, err := client.exportBundleArchive(ctx, filters)
		if err != nil {
			return err
		}
		if err := os.WriteFile(opts.OutputPath, archive, 0o644); err != nil {
			return err
		}
		fmt.Printf("saved archive to %s (%d bytes)\n", opts.OutputPath, len(archive))
		return nil
	}
	bundle, err := client.exportBundleJSON(ctx, filters)
	if err != nil {
		return err
	}
	if err := writePrettyJSON(opts.OutputPath, bundle); err != nil {
		return err
	}
	printBundleStats(*bundle)
	fmt.Printf("saved bundle to %s\n", opts.OutputPath)
	return nil
}

func runResume(args []string) error {
	fs := flag.NewFlagSet("sync resume", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts commonOptions
	addCommonFlags(fs, &opts)
	bundle := fs.String("bundle", "", "existing .ahubz bundle path")
	sessionFile := fs.String("session-file", "", "resume state path")
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}
	if strings.TrimSpace(*bundle) == "" {
		return errors.New("--bundle is required")
	}
	statePath := *sessionFile
	if strings.TrimSpace(statePath) == "" {
		statePath = defaultSessionFile(*bundle)
	}
	state, err := loadSessionFile(statePath)
	if err != nil {
		return err
	}
	archiveBytes, err := os.ReadFile(state.BundlePath)
	if err != nil {
		return err
	}
	_, _, apiBaseValue, tokenValue, _, _, err := resolveRuntimeAuth(opts, &state)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := newClient(apiBaseValue, tokenValue)
	session, err := client.resumeSession(ctx, state.SessionID, archiveBytes)
	if err != nil {
		return err
	}
	if session.Status != models.SyncSessionStatusReady {
		session, err = client.getSyncSession(ctx, state.SessionID)
		if err != nil {
			return err
		}
	}
	result, err := client.commitSession(ctx, state.SessionID, state.PreviewFingerprint)
	if err != nil {
		return err
	}
	_ = os.Remove(statePath)
	printJSON(map[string]any{
		"session": session,
		"result":  result,
	})
	return nil
}

func runHistory(args []string) error {
	opts, err := parseCommonOptions("sync history", args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_, _, apiBaseValue, tokenValue, _, _, err := resolveRuntimeAuth(opts, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	jobs, err := newClient(apiBaseValue, tokenValue).listSyncJobs(ctx)
	if err != nil {
		return err
	}
	printJSON(jobs)
	return nil
}

func runDiff(args []string) error {
	fs := flag.NewFlagSet("sync diff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	left := fs.String("left", "", "left bundle (.ahub or .ahubz)")
	right := fs.String("right", "", "right bundle (.ahub or .ahubz)")
	format := fs.String("format", "text", "text or json")
	filters, err := addFilterFlags(fs)
	if err != nil {
		return err
	}
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return &ExitError{Code: 2}
	}
	if strings.TrimSpace(*left) == "" || strings.TrimSpace(*right) == "" {
		return errors.New("--left and --right are required")
	}
	leftBundle, _, _, err := loadBundleFile(*left)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return &ExitError{Code: 2}
	}
	rightBundle, _, _, err := loadBundleFile(*right)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return &ExitError{Code: 2}
	}
	diff := compareBundles(*leftBundle, *rightBundle, filters.toModel())
	if *format == "json" {
		printJSON(diff)
	} else {
		fmt.Print(renderDiffText(diff, *left, *right))
	}
	if diff.Equal {
		return nil
	}
	return &ExitError{Code: 1}
}

type filterFlags struct {
	IncludeDomains stringListFlag
	IncludeSkills  stringListFlag
	ExcludeSkills  stringListFlag
}

func (f filterFlags) toModel() models.BundleFilters {
	return models.BundleFilters{
		IncludeDomains: append([]string{}, f.IncludeDomains...),
		IncludeSkills:  append([]string{}, f.IncludeSkills...),
		ExcludeSkills:  append([]string{}, f.ExcludeSkills...),
	}
}

type inputPayload struct {
	Kind         string
	Bundle       *models.Bundle
	ArchiveBytes []byte
	Manifest     *models.BundleArchiveManifest
	BundlePath   string
	SourceDir    string
	Mode         string
	SessionFile  string
	Transport    string
}

type pullOptions struct {
	commonOptions
	Format     string
	OutputPath string
}

func parseInputCommand(name string, args []string, includeTransport bool) (commonOptions, models.BundleFilters, inputPayload, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts commonOptions
	addCommonFlags(fs, &opts)
	source := fs.String("source", "", "directory containing skill subdirectories")
	bundle := fs.String("bundle", "", "existing .ahub or .ahubz bundle file")
	mode := fs.String("mode", "merge", "merge or mirror")
	sessionFile := fs.String("session-file", "", "session state file")
	transport := fs.String("transport", models.SyncTransportAuto, "auto, json, or archive")
	filters, err := addFilterFlags(fs)
	if err != nil {
		return commonOptions{}, models.BundleFilters{}, inputPayload{}, err
	}
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return commonOptions{}, models.BundleFilters{}, inputPayload{}, flag.ErrHelp
		}
		return commonOptions{}, models.BundleFilters{}, inputPayload{}, &ExitError{Code: 2}
	}
	if !validMode(*mode) {
		return commonOptions{}, models.BundleFilters{}, inputPayload{}, errors.New("mode must be merge or mirror")
	}
	if includeTransport && *source != "" && *bundle != "" {
		return commonOptions{}, models.BundleFilters{}, inputPayload{}, errors.New("use either --source or --bundle")
	}
	if *source == "" && *bundle == "" {
		return commonOptions{}, models.BundleFilters{}, inputPayload{}, errors.New("either --source or --bundle is required")
	}
	input := inputPayload{
		Mode:        *mode,
		SessionFile: *sessionFile,
		Transport:   *transport,
	}
	if strings.TrimSpace(*source) != "" {
		input.Kind = models.BundleFormatJSON
		input.SourceDir = *source
		bundleValue, err := buildBundle(*source, *mode)
		if err != nil {
			return commonOptions{}, models.BundleFilters{}, inputPayload{}, err
		}
		input.Bundle = bundleValue
		return opts, filters.toModel(), input, nil
	}
	bundleValue, manifest, archiveBytes, err := loadBundleFile(*bundle)
	if err != nil {
		return commonOptions{}, models.BundleFilters{}, inputPayload{}, err
	}
	input.Bundle = bundleValue
	input.Manifest = manifest
	input.ArchiveBytes = archiveBytes
	input.BundlePath = *bundle
	if strings.EqualFold(filepath.Ext(*bundle), ".ahubz") {
		input.Kind = models.BundleFormatArchive
	} else {
		input.Kind = models.BundleFormatJSON
	}
	return opts, filters.toModel(), input, nil
}

func parsePullFlags(args []string) (pullOptions, models.BundleFilters, error) {
	fs := flag.NewFlagSet("sync pull", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts pullOptions
	addCommonFlags(fs, &opts.commonOptions)
	format := fs.String("format", "json", "json or archive")
	output := fs.String("output", "backup.ahub", "output file")
	fs.StringVar(output, "o", "backup.ahub", "output file")
	filters, err := addFilterFlags(fs)
	if err != nil {
		return pullOptions{}, models.BundleFilters{}, err
	}
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return pullOptions{}, models.BundleFilters{}, flag.ErrHelp
		}
		return pullOptions{}, models.BundleFilters{}, &ExitError{Code: 2}
	}
	if *format != models.BundleFormatJSON && *format != models.BundleFormatArchive {
		return pullOptions{}, models.BundleFilters{}, errors.New("format must be json or archive")
	}
	opts.Format = *format
	opts.OutputPath = *output
	return opts, filters.toModel(), nil
}

func parseCommonOptions(name string, args []string) (commonOptions, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts commonOptions
	addCommonFlags(fs, &opts)
	if err := parseFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return commonOptions{}, flag.ErrHelp
		}
		return commonOptions{}, &ExitError{Code: 2}
	}
	return opts, nil
}

func addCommonFlags(fs *flag.FlagSet, opts *commonOptions) {
	fs.StringVar(&opts.Token, "token", "", "override sync token")
	fs.StringVar(&opts.APIBase, "api-base", "", "override Agent Hub base URL")
	fs.StringVar(&opts.Profile, "profile", "", "profile name")
	fs.StringVar(&opts.ConfigPath, "config", "", "override config path")
}

func addFilterFlags(fs *flag.FlagSet) (filterFlags, error) {
	var filters filterFlags
	fs.Var(&filters.IncludeDomains, "include-domain", "include sync domain (profile, memory, skills)")
	fs.Var(&filters.IncludeSkills, "include-skill", "include only these skills")
	fs.Var(&filters.ExcludeSkills, "exclude-skill", "exclude these skills")
	return filters, nil
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return flag.ErrHelp
		}
		return err
	}
	return nil
}

func validMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "merge", "mirror":
		return true
	default:
		return false
	}
}

func loadCLIConfig(configOverride string) (string, *runtimecfg.CLIConfig, error) {
	if strings.TrimSpace(configOverride) != "" {
		return runtimecfg.LoadConfig(configOverride)
	}
	if envOverride := strings.TrimSpace(os.Getenv(syncConfigEnv)); envOverride != "" {
		return runtimecfg.LoadConfig(envOverride)
	}
	return runtimecfg.LoadConfig("")
}

func resolveAPIBase(opts commonOptions, cfg *runtimecfg.CLIConfig, state *sessionState) string {
	if value := strings.TrimSpace(opts.APIBase); value != "" {
		return strings.TrimRight(value, "/")
	}
	for _, name := range apiBaseEnvs {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	if _, profile, ok := profileEntry(opts.Profile, cfg); ok && strings.TrimSpace(profile.APIBase) != "" {
		return strings.TrimRight(profile.APIBase, "/")
	}
	if state != nil && strings.TrimSpace(state.APIBase) != "" {
		return strings.TrimRight(state.APIBase, "/")
	}
	return strings.TrimRight(fallbackAPIBase, "/")
}

func resolveRuntimeAuth(opts commonOptions, state *sessionState) (string, *runtimecfg.CLIConfig, string, string, string, string, error) {
	configPath, cfg, err := loadCLIConfig(opts.ConfigPath)
	if err != nil {
		return "", nil, "", "", "", "", err
	}
	apiBaseValue := resolveAPIBase(opts, cfg, state)
	if value := strings.TrimSpace(opts.Token); value != "" {
		return configPath, cfg, apiBaseValue, value, strings.TrimSpace(opts.Profile), "flag", nil
	}
	for _, name := range tokenEnvs {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return configPath, cfg, apiBaseValue, value, strings.TrimSpace(opts.Profile), "env", nil
		}
	}
	profileName, profile, ok := profileEntry(opts.Profile, cfg)
	if !ok {
		return "", nil, "", "", "", "", errors.New("no sync token found; run `agenthub sync login` or pass --token")
	}
	if strings.TrimSpace(profile.Token) == "" {
		return "", nil, "", "", "", "", fmt.Errorf("profile %s has no saved token; run `agenthub sync login --profile %s`", profileName, profileName)
	}
	if profileExpired(profile) {
		return "", nil, "", "", "", "", fmt.Errorf("stored token for profile %s expired at %s; run `agenthub sync login --profile %s`", profileName, profile.ExpiresAt, profileName)
	}
	return configPath, cfg, apiBaseValue, profile.Token, profileName, "profile:" + profileName, nil
}

func selectedProfileName(requested string, cfg *runtimecfg.CLIConfig) string {
	if value := strings.TrimSpace(requested); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(syncProfileEnv)); value != "" {
		return value
	}
	if value := strings.TrimSpace(cfg.CurrentProfile); value != "" {
		return value
	}
	if len(cfg.Profiles) == 1 {
		for name := range cfg.Profiles {
			return name
		}
	}
	return ""
}

func profileEntry(requested string, cfg *runtimecfg.CLIConfig) (string, runtimecfg.SyncProfile, bool) {
	name := selectedProfileName(requested, cfg)
	if name == "" {
		return "", runtimecfg.SyncProfile{}, false
	}
	profile, ok := cfg.Profiles[name]
	return name, profile, ok
}

func profileExpired(profile runtimecfg.SyncProfile) bool {
	if strings.TrimSpace(profile.ExpiresAt) == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, profile.ExpiresAt)
	if err != nil {
		return false
	}
	return !expiresAt.After(time.Now().UTC())
}

func pickProfileName(cfg *runtimecfg.CLIConfig, requested, apiBaseValue string) string {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested)
	}
	if current := strings.TrimSpace(cfg.CurrentProfile); current != "" {
		if profile, ok := cfg.Profiles[current]; ok && strings.TrimRight(profile.APIBase, "/") == strings.TrimRight(apiBaseValue, "/") {
			return current
		}
	}
	for name, profile := range cfg.Profiles {
		if strings.TrimRight(profile.APIBase, "/") == strings.TrimRight(apiBaseValue, "/") {
			return name
		}
	}
	if len(cfg.Profiles) == 0 {
		return "default"
	}
	if _, ok := cfg.Profiles["default"]; !ok {
		return "default"
	}
	host := "profile"
	if parsed, err := url.Parse(apiBaseValue); err == nil && parsed.Hostname() != "" {
		host = strings.Split(parsed.Hostname(), ".")[0]
	}
	base := safeLabel(host, "profile")
	candidate := base
	counter := 2
	for {
		if _, ok := cfg.Profiles[candidate]; !ok {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, counter)
		counter++
	}
}

func renderProfiles(cfg *runtimecfg.CLIConfig) string {
	if len(cfg.Profiles) == 0 {
		return "No saved profiles. Run `agenthub sync login`."
	}
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		profile := cfg.Profiles[name]
		prefix := " "
		if name == cfg.CurrentProfile {
			prefix = "*"
		}
		authStatus := "logged-out"
		if strings.TrimSpace(profile.Token) != "" {
			if profileExpired(profile) {
				authStatus = "expired"
			} else {
				authStatus = "ready"
			}
		}
		scopes := strings.Join(profile.Scopes, ",")
		if scopes == "" {
			scopes = "-"
		}
		expiresAt := defaultString(profile.ExpiresAt, "-")
		apiBaseValue := defaultString(profile.APIBase, "-")
		lines = append(lines, fmt.Sprintf("%s %s  %s  %s  scopes=%s  expires=%s", prefix, name, apiBaseValue, authStatus, scopes, expiresAt))
	}
	return strings.Join(lines, "\n")
}

func printLoginSummary(profileName, apiBaseValue string, info *models.AgentAuthInfo) {
	fmt.Printf("Logged in to %s as profile %s\n", apiBaseValue, profileName)
	if strings.TrimSpace(info.UserSlug) != "" {
		fmt.Printf("User: %s\n", info.UserSlug)
	}
	if info.ExpiresAt != nil {
		fmt.Printf("Token expires at %s\n", info.ExpiresAt.UTC().Format(time.RFC3339))
	} else {
		fmt.Println("Token expires at -")
	}
	if len(info.Scopes) == 0 {
		fmt.Println("Scopes: -")
	} else {
		fmt.Printf("Scopes: %s\n", strings.Join(info.Scopes, ", "))
	}
	fmt.Printf("Current profile: %s\n", profileName)
}

func waitForBrowserLogin(apiBaseValue, profileName, access string, ttlMinutes int) (loginCallbackPayload, error) {
	if access == "" {
		access = "both"
	}
	if ttlMinutes <= 0 {
		ttlMinutes = 30
	}
	state, err := randomHex(18)
	if err != nil {
		return loginCallbackPayload{}, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return loginCallbackPayload{}, err
	}
	defer listener.Close()
	payloadCh := make(chan loginCallbackPayload, 1)
	errCh := make(chan error, 1)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if r.Method != http.MethodPost || r.URL.Path != "/callback" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			var payload loginCallbackPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid request body"}`))
				return
			}
			if payload.State != state {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid login state"}`))
				return
			}
			select {
			case payloadCh <- payload:
			default:
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}),
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)
	syncURL := fmt.Sprintf("%s/data/sync?%s", strings.TrimRight(apiBaseValue, "/"), url.Values{
		"cli_login":       []string{"1"},
		"cli_profile":     []string{profileName},
		"cli_access":      []string{access},
		"cli_ttl_minutes": []string{fmt.Sprintf("%d", ttlMinutes)},
		"cli_callback":    []string{callbackURL},
		"cli_state":       []string{state},
	}.Encode())
	fmt.Printf("Opening browser for Agent Hub sync login:\n%s\n", syncURL)
	_ = openBrowser(syncURL)
	timer := time.NewTimer(loginTimeout)
	defer timer.Stop()
	select {
	case payload := <-payloadCh:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return payload, nil
	case err := <-errCh:
		return loginCallbackPayload{}, err
	case <-timer.C:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return loginCallbackPayload{}, errors.New("timed out waiting for browser login callback")
	}
}

func openBrowser(target string) error {
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

func saveSessionFile(path string, state sessionState) error {
	return writePrettyJSON(path, state)
}

func loadSessionFile(path string) (sessionState, error) {
	var state sessionState
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func defaultSessionFile(bundlePath string) string {
	return bundlePath + ".session.json"
}

func writePrettyJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func randomHex(byteLen int) (string, error) {
	if byteLen <= 0 {
		byteLen = 16
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
