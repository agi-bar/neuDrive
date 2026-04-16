package synccli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/profileauth"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
)

const (
	fallbackAPIBase = "http://localhost:8080"
	syncConfigEnv   = "NEUDRIVE_SYNC_CONFIG"
	syncProfileEnv  = "NEUDRIVE_SYNC_PROFILE"
	syncAPIBaseEnv  = "NEUDRIVE_SYNC_API_BASE"
	syncTokenEnv    = "NEUDRIVE_SYNC_TOKEN"
)

var (
	apiBaseEnvs = []string{syncAPIBaseEnv, "NEUDRIVE_API_BASE"}
	tokenEnvs   = []string{syncTokenEnv, "NEUDRIVE_TOKEN"}
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
	Local      bool
}

type sessionState struct {
	APIBase            string `json:"api_base"`
	BundlePath         string `json:"bundle_path"`
	SessionID          string `json:"session_id"`
	PreviewFingerprint string `json:"preview_fingerprint"`
	Profile            string `json:"profile"`
	CreatedAt          string `json:"created_at"`
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
	fmt.Println("Usage: neudrive sync export|preview|push|pull|resume|history|diff")
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("sync export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	source := fs.String("source", "", "directory containing skill subdirectories")
	mode := fs.String("mode", "merge", "merge or mirror")
	format := fs.String("format", "json", "json or archive")
	output := fs.String("output", "backup.ndrv", "output bundle path")
	fs.StringVar(output, "o", "backup.ndrv", "output bundle path")
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
	if !strings.HasSuffix(strings.ToLower(bundlePath), ".ndrvz") {
		stem := "bundle"
		if input.SourceDir != "" {
			stem = filepath.Base(filepath.Clean(input.SourceDir))
		} else if bundlePath != "" {
			stem = strings.TrimSuffix(filepath.Base(bundlePath), filepath.Ext(bundlePath))
		}
		bundlePath = filepath.Join(".", stem+".ndrvz")
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
	bundle := fs.String("bundle", "", "existing .ndrvz bundle path")
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
	left := fs.String("left", "", "left bundle (.ndrv or .ndrvz)")
	right := fs.String("right", "", "right bundle (.ndrv or .ndrvz)")
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
	bundle := fs.String("bundle", "", "existing .ndrv or .ndrvz bundle file")
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
	if strings.EqualFold(filepath.Ext(*bundle), ".ndrvz") {
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
	output := fs.String("output", "backup.ndrv", "output file")
	fs.StringVar(output, "o", "backup.ndrv", "output file")
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
	fs.StringVar(&opts.APIBase, "api-base", "", "override neuDrive base URL")
	fs.StringVar(&opts.Profile, "profile", "", "profile name")
	fs.StringVar(&opts.ConfigPath, "config", "", "override config path")
	fs.BoolVar(&opts.Local, "local", false, "force the local neuDrive target")
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
		return "", nil, "", "", "", "", errors.New("no sync token found; run `neudrive login` or pass --token")
	}
	if strings.TrimSpace(profile.AuthMode) == runtimecfg.AuthModeOAuthSession {
		refreshed, err := profileauth.EnsureProfileToken(context.Background(), configPath, cfg, profileName)
		if err != nil {
			return "", nil, "", "", "", "", fmt.Errorf("profile %s needs a fresh hosted login; run `neudrive login --profile %s`", profileName, profileName)
		}
		profile = refreshed
	}
	if strings.TrimSpace(profile.Token) == "" {
		return "", nil, "", "", "", "", fmt.Errorf("profile %s has no saved token; run `neudrive login --profile %s`", profileName, profileName)
	}
	if strings.TrimSpace(profile.AuthMode) != runtimecfg.AuthModeOAuthSession && profileExpired(profile) {
		return "", nil, "", "", "", "", fmt.Errorf("stored token for profile %s expired at %s; run `neudrive login --profile %s`", profileName, profile.ExpiresAt, profileName)
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
	if value := runtimecfg.TargetProfileName(cfg.CurrentTarget); value != "" {
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
