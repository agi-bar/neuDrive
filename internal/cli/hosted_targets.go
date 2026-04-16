package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/profileauth"
	"github.com/agi-bar/neudrive/internal/runtimecfg"
)

const hostedLoginTimeout = 5 * time.Minute

type commandTargetOptions struct {
	Local   bool
	Profile string
	APIBase string
	Token   string
}

type resolvedCommandTarget struct {
	APIBase     string
	Token       string
	ProfileName string
	TokenSource string
	Target      string
	AuthMode    string
	ExpiresAt   string
}

type loginResult struct {
	Code  string
	State string
}

func addTargetFlags(fs *flag.FlagSet, opts *commandTargetOptions) {
	fs.BoolVar(&opts.Local, "local", false, "force the local neuDrive target")
	fs.StringVar(&opts.Profile, "profile", "", "remote profile name")
	fs.StringVar(&opts.APIBase, "api-base", "", "explicit neuDrive API base")
	fs.StringVar(&opts.Token, "token", "", "explicit bearer token")
}

func resolveCommandTarget(ctx context.Context, opts commandTargetOptions) (*resolvedCommandTarget, error) {
	if opts.Local {
		return resolveLocalCommandTarget(ctx)
	}
	if strings.TrimSpace(opts.Profile) != "" {
		return resolveProfileCommandTarget(ctx, strings.TrimSpace(opts.Profile))
	}
	if strings.TrimSpace(opts.APIBase) != "" || strings.TrimSpace(opts.Token) != "" {
		if strings.TrimSpace(opts.APIBase) == "" || strings.TrimSpace(opts.Token) == "" {
			return nil, errors.New("--api-base and --token must be provided together")
		}
		return &resolvedCommandTarget{
			APIBase:     strings.TrimRight(strings.TrimSpace(opts.APIBase), "/"),
			Token:       strings.TrimSpace(opts.Token),
			TokenSource: "flag",
			Target:      "explicit",
		}, nil
	}

	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		return nil, err
	}
	switch runtimecfg.SelectedTarget(cfg) {
	case runtimecfg.TargetLocal:
		return resolveLocalCommandTarget(ctx)
	default:
		profileName := runtimecfg.TargetProfileName(cfg.CurrentTarget)
		if profileName == "" {
			return resolveLocalCommandTarget(ctx)
		}
		return resolveProfileCommandTargetWithConfig(ctx, configPath, cfg, profileName)
	}
}

func resolveLocalCommandTarget(ctx context.Context) (*resolvedCommandTarget, error) {
	_, state, token, err := ensureLocalOwnerAccessForAPI(ctx)
	if err != nil {
		return nil, err
	}
	return &resolvedCommandTarget{
		APIBase:     state.APIBase,
		Token:       token,
		TokenSource: "local",
		Target:      runtimecfg.TargetLocal,
		AuthMode:    runtimecfg.AuthModeScopedToken,
	}, nil
}

func resolveProfileCommandTarget(ctx context.Context, profileName string) (*resolvedCommandTarget, error) {
	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		return nil, err
	}
	return resolveProfileCommandTargetWithConfig(ctx, configPath, cfg, profileName)
}

func resolveProfileCommandTargetWithConfig(ctx context.Context, configPath string, cfg *runtimecfg.CLIConfig, profileName string) (*resolvedCommandTarget, error) {
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q does not exist", profileName)
	}
	if strings.TrimSpace(profile.AuthMode) == runtimecfg.AuthModeOAuthSession {
		refreshed, err := profileauth.EnsureProfileToken(ctx, configPath, cfg, profileName)
		if err != nil {
			return nil, err
		}
		profile = refreshed
	}
	if strings.TrimSpace(profile.Token) == "" {
		return nil, fmt.Errorf("profile %q has no saved token; run `neu login --profile %s`", profileName, profileName)
	}
	if strings.TrimSpace(profile.AuthMode) != runtimecfg.AuthModeOAuthSession && profileauth.TokenExpired(profile.ExpiresAt) {
		return nil, fmt.Errorf("stored token for profile %s expired at %s; run `neu login --profile %s`", profileName, profile.ExpiresAt, profileName)
	}
	return &resolvedCommandTarget{
		APIBase:     strings.TrimRight(strings.TrimSpace(profile.APIBase), "/"),
		Token:       strings.TrimSpace(profile.Token),
		ProfileName: profileName,
		TokenSource: "profile:" + profileName,
		Target:      runtimecfg.ProfileTarget(profileName),
		AuthMode:    profileAuthMode(profile),
		ExpiresAt:   strings.TrimSpace(profile.ExpiresAt),
	}, nil
}

func profileAuthMode(profile runtimecfg.SyncProfile) string {
	if strings.TrimSpace(profile.AuthMode) != "" {
		return strings.TrimSpace(profile.AuthMode)
	}
	return runtimecfg.AuthModeScopedToken
}

func runLogin(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("login")
		return 0
	}

	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profileName := fs.String("profile", "official", "profile name to save")
	apiBase := fs.String("api-base", runtimecfg.DefaultRemoteOfficial, "neuDrive hosted base URL")
	urlAlias := fs.String("url", "", "alias for --api-base")
	tokenValue := fs.String("token", "", "manually save an existing bearer token instead of opening the browser")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if strings.TrimSpace(*urlAlias) != "" {
		*apiBase = *urlAlias
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "login: load config: %v\n", err)
		return 1
	}

	apiBaseValue := strings.TrimRight(strings.TrimSpace(*apiBase), "/")
	if apiBaseValue == "" {
		apiBaseValue = runtimecfg.DefaultRemoteOfficial
	}
	profileValue := strings.TrimSpace(*profileName)
	if profileValue == "" {
		profileValue = "official"
	}

	var entry runtimecfg.SyncProfile
	source := "manual"
	if strings.TrimSpace(*tokenValue) != "" {
		entry = runtimecfg.SyncProfile{
			APIBase:   apiBaseValue,
			Token:     strings.TrimSpace(*tokenValue),
			AuthMode:  runtimecfg.AuthModeScopedToken,
			Source:    source,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		}
	} else {
		source = "oauth"
		session, userSlug, scopes, err := loginWithOAuth(ctx, apiBaseValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "login: %v\n", err)
			return 1
		}
		entry = runtimecfg.SyncProfile{
			APIBase:      apiBaseValue,
			Token:        session.AccessToken,
			RefreshToken: session.RefreshToken,
			ExpiresAt:    session.ExpiresAt.UTC().Format(time.RFC3339),
			Scopes:       append([]string{}, scopes...),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			Source:       source,
			AuthMode:     runtimecfg.AuthModeOAuthSession,
		}
		if userSlug != "" {
			entry.Source = source + ":" + userSlug
		}
	}

	info, err := fetchAgentAuthInfo(ctx, apiBaseValue, entry.Token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login: validate token: %v\n", err)
		return 1
	}
	if len(info.Scopes) > 0 {
		entry.Scopes = append([]string{}, info.Scopes...)
	}
	if info.ExpiresAt != nil {
		entry.ExpiresAt = info.ExpiresAt.UTC().Format(time.RFC3339)
	}
	cfg.Profiles[profileValue] = entry
	cfg.CurrentTarget = runtimecfg.ProfileTarget(profileValue)
	if err := runtimecfg.SaveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "login: save config: %v\n", err)
		return 1
	}

	fmt.Printf("Logged in to %s as profile %s\n", apiBaseValue, profileValue)
	if strings.TrimSpace(info.UserSlug) != "" {
		fmt.Printf("User: %s\n", info.UserSlug)
	}
	fmt.Printf("Auth mode: %s\n", profileAuthMode(entry))
	if strings.TrimSpace(entry.ExpiresAt) != "" {
		fmt.Printf("Token expires at: %s\n", entry.ExpiresAt)
	} else {
		fmt.Println("Token expires at: -")
	}
	if len(entry.Scopes) == 0 {
		fmt.Println("Scopes: -")
	} else {
		fmt.Printf("Scopes: %s\n", strings.Join(entry.Scopes, ", "))
	}
	fmt.Printf("Current target: %s\n", cfg.CurrentTarget)
	return 0
}

func runLogout(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("logout")
		return 0
	}

	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profileName := fs.String("profile", "", "profile name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if strings.TrimSpace(*profileName) == "" && fs.NArg() > 0 {
		*profileName = fs.Arg(0)
	}

	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "logout: load config: %v\n", err)
		return 1
	}
	targetProfile := strings.TrimSpace(*profileName)
	if targetProfile == "" {
		targetProfile = runtimecfg.TargetProfileName(cfg.CurrentTarget)
	}
	if targetProfile == "" {
		fmt.Fprintln(os.Stderr, "logout: no hosted profile selected; pass --profile")
		return 1
	}
	entry, ok := cfg.Profiles[targetProfile]
	if !ok {
		fmt.Fprintf(os.Stderr, "logout: profile %s does not exist\n", targetProfile)
		return 1
	}
	entry.Token = ""
	entry.RefreshToken = ""
	entry.ExpiresAt = ""
	entry.Scopes = nil
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cfg.Profiles[targetProfile] = entry
	if runtimecfg.TargetProfileName(cfg.CurrentTarget) == targetProfile {
		cfg.CurrentTarget = runtimecfg.TargetLocal
	}
	if err := runtimecfg.SaveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "logout: save config: %v\n", err)
		return 1
	}
	fmt.Printf("Logged out profile %s\n", targetProfile)
	fmt.Printf("Current target: %s\n", runtimecfg.SelectedTarget(cfg))
	return 0
}

func runUse(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("use")
		return 0
	}

	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profileName := fs.String("profile", "", "profile name or local")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if strings.TrimSpace(*profileName) == "" && fs.NArg() > 0 {
		*profileName = fs.Arg(0)
	}
	selection := strings.TrimSpace(*profileName)
	if selection == "" {
		fmt.Fprintln(os.Stderr, usageLine("use <local|profile>"))
		return 2
	}

	configPath, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "use: load config: %v\n", err)
		return 1
	}
	if selection == runtimecfg.TargetLocal {
		cfg.CurrentTarget = runtimecfg.TargetLocal
	} else {
		if _, ok := cfg.Profiles[selection]; !ok {
			fmt.Fprintf(os.Stderr, "use: profile %s does not exist\n", selection)
			return 1
		}
		cfg.CurrentTarget = runtimecfg.ProfileTarget(selection)
	}
	if err := runtimecfg.SaveConfig(configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "use: save config: %v\n", err)
		return 1
	}
	fmt.Printf("Current target: %s\n", cfg.CurrentTarget)
	return 0
}

func runWhoAmICommand(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("whoami")
		return 0
	}

	fs := flag.NewFlagSet("whoami", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var opts commandTargetOptions
	addTargetFlags(fs, &opts)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := resolveCommandTarget(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "whoami: %v\n", err)
		return 1
	}
	info, err := fetchAgentAuthInfo(ctx, target.APIBase, target.Token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "whoami: %v\n", err)
		return 1
	}

	currentProfile := target.ProfileName
	if currentProfile == "" {
		currentProfile = "-"
	}
	fmt.Printf("Current target: %s\n", target.Target)
	fmt.Printf("Current profile: %s\n", currentProfile)
	fmt.Printf("API base: %s\n", strings.TrimRight(info.APIBase, "/"))
	if strings.TrimSpace(info.UserSlug) != "" {
		fmt.Printf("User: %s\n", info.UserSlug)
	}
	fmt.Printf("Auth mode: %s\n", defaultText(info.AuthMode, target.AuthMode, runtimecfg.AuthModeScopedToken))
	fmt.Printf("Trust level: %d\n", info.TrustLevel)
	fmt.Printf("Token source: %s\n", target.TokenSource)
	if info.ExpiresAt != nil {
		fmt.Printf("Token expires at: %s\n", info.ExpiresAt.UTC().Format(time.RFC3339))
	} else if strings.TrimSpace(target.ExpiresAt) != "" {
		fmt.Printf("Token expires at: %s\n", target.ExpiresAt)
	} else {
		fmt.Println("Token expires at: -")
	}
	if len(info.Scopes) == 0 {
		fmt.Println("Scopes: -")
	} else {
		fmt.Printf("Scopes: %s\n", strings.Join(info.Scopes, ", "))
	}
	return 0
}

func runProfilesCommand(args []string) int {
	if isExplicitHelpRequest(args) {
		printHelpTopic("profiles")
		return 0
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, usageLine("profiles"))
		return 2
	}

	_, cfg, err := runtimecfg.LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "profiles: load config: %v\n", err)
		return 1
	}

	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	currentTarget := runtimecfg.SelectedTarget(cfg)
	fmt.Printf("Current target: %s\n", currentTarget)
	localMarker := " "
	if currentTarget == runtimecfg.TargetLocal {
		localMarker = "*"
	}
	fmt.Printf("%s local\n", localMarker)
	if len(names) == 0 {
		fmt.Println("No saved hosted profiles.")
		return 0
	}
	for _, name := range names {
		profile := cfg.Profiles[name]
		marker := " "
		if runtimecfg.TargetProfileName(currentTarget) == name {
			marker = "*"
		}
		status := "logged-out"
		if strings.TrimSpace(profile.Token) != "" {
			if strings.TrimSpace(profile.AuthMode) == runtimecfg.AuthModeOAuthSession {
				if profileauth.TokenExpired(profile.ExpiresAt) {
					status = "refresh-needed"
				} else {
					status = "ready"
				}
			} else if profileauth.TokenExpired(profile.ExpiresAt) {
				status = "expired"
			} else {
				status = "ready"
			}
		}
		scopes := strings.Join(profile.Scopes, ",")
		if scopes == "" {
			scopes = "-"
		}
		expiresAt := strings.TrimSpace(profile.ExpiresAt)
		if expiresAt == "" {
			expiresAt = "-"
		}
		fmt.Printf("%s %s  %s  %s  auth=%s  scopes=%s  expires=%s\n",
			marker,
			name,
			defaultText(profile.APIBase, "-"),
			status,
			profileAuthMode(profile),
			scopes,
			expiresAt,
		)
	}
	return 0
}

func loginWithOAuth(ctx context.Context, apiBase string) (*profileauth.Session, string, []string, error) {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if !strings.HasPrefix(strings.ToLower(apiBase), "https://") {
		return nil, "", nil, fmt.Errorf("hosted OAuth login requires an HTTPS api base")
	}

	state, err := randomHex(18)
	if err != nil {
		return nil, "", nil, err
	}
	codeVerifier, codeChallenge, err := pkcePair()
	if err != nil {
		return nil, "", nil, err
	}
	loginResult, redirectURI, err := waitForOAuthCallback(apiBase, state, codeChallenge)
	if err != nil {
		return nil, "", nil, err
	}
	session, err := profileauth.ExchangeCode(ctx, apiBase, loginResult.Code, redirectURI, codeVerifier)
	if err != nil {
		return nil, "", nil, err
	}
	info, err := fetchAgentAuthInfo(ctx, apiBase, session.AccessToken)
	if err != nil {
		return nil, "", nil, err
	}
	scopes := session.Scopes
	if len(info.Scopes) > 0 {
		scopes = append([]string{}, info.Scopes...)
	}
	return session, info.UserSlug, scopes, nil
}

func waitForOAuthCallback(apiBase, state, codeChallenge string) (loginResult, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return loginResult{}, "", err
	}
	defer listener.Close()

	callbackCh := make(chan loginResult, 1)
	errCh := make(chan error, 1)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
				return
			}
			result := loginResult{
				Code:  strings.TrimSpace(r.URL.Query().Get("code")),
				State: strings.TrimSpace(r.URL.Query().Get("state")),
			}
			if result.State != state || result.Code == "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("neuDrive CLI login failed. You can close this page and try again."))
				return
			}
			select {
			case callbackCh <- result:
			default:
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!doctype html><html><body><h1>neuDrive CLI login complete</h1><p>You can close this page and return to the terminal.</p></body></html>"))
		}),
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	callbackURI := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)
	authorizeURL := fmt.Sprintf("%s/oauth/authorize?%s", strings.TrimRight(apiBase, "/"), url.Values{
		"response_type":         []string{"code"},
		"client_id":             []string{profileauth.OAuthClientID(apiBase)},
		"redirect_uri":          []string{callbackURI},
		"scope":                 []string{"admin offline_access"},
		"state":                 []string{state},
		"code_challenge":        []string{codeChallenge},
		"code_challenge_method": []string{"S256"},
	}.Encode())
	fmt.Printf("Opening browser for neuDrive hosted login:\n%s\n", authorizeURL)
	_ = openBrowser(authorizeURL)

	timer := time.NewTimer(hostedLoginTimeout)
	defer timer.Stop()
	select {
	case result := <-callbackCh:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return result, callbackURI, nil
	case err := <-errCh:
		return loginResult{}, "", err
	case <-timer.C:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return loginResult{}, "", errors.New("timed out waiting for browser login callback")
	}
}

func pkcePair() (string, string, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomHex(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func fetchAgentAuthInfo(ctx context.Context, apiBase, token string) (*models.AgentAuthInfo, error) {
	var info models.AgentAuthInfo
	if err := localAPIGet(ctx, apiBase, token, "/agent/auth/whoami", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func defaultText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
