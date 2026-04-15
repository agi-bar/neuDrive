package localgitsync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const gitHubAppStatePurpose = "git_mirror_github_app_user"

type gitHubAppTokenResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
	ErrorURI              string `json:"error_uri"`
}

type gitHubAppDeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type gitHubAppStateClaims struct {
	UserID   string `json:"user_id"`
	Purpose  string `json:"purpose"`
	ReturnTo string `json:"return_to"`
	jwt.RegisteredClaims
}

type gitHubOwner struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type gitHubRepoPermissions struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

type gitHubRepoItem struct {
	Name          string                `json:"name"`
	FullName      string                `json:"full_name"`
	DefaultBranch string                `json:"default_branch"`
	CloneURL      string                `json:"clone_url"`
	Permissions   gitHubRepoPermissions `json:"permissions"`
	Owner         gitHubOwner           `json:"owner"`
}

type gitHubMembership struct {
	Organization gitHubOwner `json:"organization"`
	Role         string      `json:"role"`
	State        string      `json:"state"`
}

type gitHubCreateRepoRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Private     bool   `json:"private"`
	AutoInit    bool   `json:"auto_init"`
}

func defaultAuthModeForExecution(executionMode string) string {
	if executionMode == ExecutionModeHosted {
		return AuthModeGitHubToken
	}
	return AuthModeLocalCredentials
}

func (s *Service) gitHubAppConfigured() bool {
	return s != nil &&
		strings.TrimSpace(s.gitHubAppClientID) != "" &&
		strings.TrimSpace(s.gitHubAppClientSecret) != ""
}

func (s *Service) hasStoredGitHubAppRefreshToken(ctx context.Context, userID uuid.UUID) (bool, error) {
	if s == nil || s.vault == nil {
		return false, nil
	}
	scopes, err := s.vault.ListScopes(ctx, userID, models.TrustLevelFull)
	if err != nil {
		return false, err
	}
	for _, scope := range scopes {
		if scope.Scope == gitMirrorGitHubAppRefreshTokenScope {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) readStoredGitHubAppRefreshToken(ctx context.Context, userID uuid.UUID) (string, bool, error) {
	configured, err := s.hasStoredGitHubAppRefreshToken(ctx, userID)
	if err != nil || !configured {
		return "", configured, err
	}
	if s == nil || s.vault == nil {
		return "", false, nil
	}
	token, err := s.vault.Read(ctx, userID, gitMirrorGitHubAppRefreshTokenScope, models.TrustLevelFull)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(token), true, nil
}

func (s *Service) writeStoredGitHubAppRefreshToken(ctx context.Context, userID uuid.UUID, token string) error {
	if s == nil || s.vault == nil {
		return fmt.Errorf("vault service not configured")
	}
	return s.vault.Write(
		ctx,
		userID,
		gitMirrorGitHubAppRefreshTokenScope,
		strings.TrimSpace(token),
		"GitHub App user refresh token for Git mirror",
		models.TrustLevelFull,
	)
}

func (s *Service) deleteStoredGitHubAppRefreshToken(ctx context.Context, userID uuid.UUID) error {
	if s == nil || s.vault == nil {
		return nil
	}
	configured, err := s.hasStoredGitHubAppRefreshToken(ctx, userID)
	if err != nil || !configured {
		return err
	}
	return s.vault.Delete(ctx, userID, gitMirrorGitHubAppRefreshTokenScope)
}

func (s *Service) StartGitHubAppBrowserFlow(_ context.Context, userID uuid.UUID, returnTo string) (*GitHubAppBrowserStartResult, error) {
	if !s.gitHubAppConfigured() {
		return nil, fmt.Errorf("GitHub App auth is not configured")
	}
	state, err := s.signGitHubAppState(userID, normalizeReturnTo(returnTo))
	if err != nil {
		return nil, err
	}
	callbackURL, err := s.gitHubAppCallbackURL()
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("client_id", s.gitHubAppClientID)
	values.Set("redirect_uri", callbackURL)
	values.Set("state", state)
	values.Set("allow_signup", "false")
	return &GitHubAppBrowserStartResult{
		AuthorizationURL: strings.TrimRight(s.githubBaseURL, "/") + "/login/oauth/authorize?" + values.Encode(),
	}, nil
}

func (s *Service) CompleteGitHubAppBrowserFlow(ctx context.Context, code, state string) (string, error) {
	claims, err := s.parseGitHubAppState(state)
	if err != nil {
		return normalizeReturnTo(""), err
	}
	tokenResp, err := s.exchangeGitHubAppCode(ctx, strings.TrimSpace(code))
	if err != nil {
		return claims.ReturnTo, err
	}
	if err := s.storeGitHubAppTokenResponse(ctx, claims.userID(), tokenResp); err != nil {
		return claims.ReturnTo, err
	}
	return appendMirrorCallbackStatus(claims.ReturnTo, "connected", ""), nil
}

func (s *Service) StartGitHubAppDeviceFlow(ctx context.Context, _ uuid.UUID) (*GitHubAppDeviceStartResult, error) {
	if !s.gitHubAppConfigured() {
		return nil, fmt.Errorf("GitHub App auth is not configured")
	}
	resp := gitHubAppDeviceCodeResponse{}
	if err := s.githubOAuthJSON(ctx, "/login/device/code", url.Values{
		"client_id": {s.gitHubAppClientID},
	}, &resp); err != nil {
		return nil, err
	}
	expiresAt := ""
	if resp.ExpiresIn > 0 {
		expiresAt = time.Now().UTC().Add(time.Duration(resp.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return &GitHubAppDeviceStartResult{
		DeviceCode:      resp.DeviceCode,
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		ExpiresAt:       expiresAt,
		Interval:        resp.Interval,
	}, nil
}

func (s *Service) PollGitHubAppDeviceFlow(ctx context.Context, userID uuid.UUID, deviceCode string) (*GitHubAppDevicePollResult, error) {
	if !s.gitHubAppConfigured() {
		return nil, fmt.Errorf("GitHub App auth is not configured")
	}
	tokenResp, err := s.exchangeGitHubAppDeviceCode(ctx, strings.TrimSpace(deviceCode))
	if err != nil {
		return nil, err
	}
	switch tokenResp.Error {
	case "authorization_pending", "slow_down":
		return &GitHubAppDevicePollResult{
			Pending: true,
			Message: tokenResp.Error,
		}, nil
	case "expired_token":
		return nil, fmt.Errorf("the device code expired; start the device flow again")
	case "access_denied":
		return nil, fmt.Errorf("GitHub authorization was denied")
	}
	if err := s.storeGitHubAppTokenResponse(ctx, userID, tokenResp); err != nil {
		return nil, err
	}
	active, _ := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	login := ""
	authorizedAt := ""
	if active != nil {
		login = strings.TrimSpace(active.GitHubAppUserLogin)
		authorizedAt = formatOptionalTime(active.GitHubAppAuthorizedAt)
	}
	return &GitHubAppDevicePollResult{
		Connected:             true,
		Message:               "connected",
		GitHubAppUserLogin:    login,
		GitHubAppAuthorizedAt: authorizedAt,
	}, nil
}

func (s *Service) DisconnectGitHubAppUser(ctx context.Context, userID uuid.UUID) error {
	if err := s.deleteStoredGitHubAppRefreshToken(ctx, userID); err != nil {
		return err
	}
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil || active == nil {
		return err
	}
	mirror := normalizeMirror(active)
	mirror.GitHubAppUserLogin = ""
	mirror.GitHubAppAuthorizedAt = nil
	mirror.GitHubAppRefreshExpiresAt = nil
	mirror.UpdatedAt = time.Now().UTC()
	if mirror.AuthMode == AuthModeGitHubAppUser {
		mirror.AutoPushEnabled = false
		clearGitHubTokenVerification(&mirror)
		clearGitHubRepoPermission(&mirror)
	}
	return s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror)
}

func (s *Service) ListGitHubAppRepos(ctx context.Context, userID uuid.UUID) ([]GitHubMirrorRepo, error) {
	token, _, err := s.refreshGitHubAppUserAccessToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	var repos []gitHubRepoItem
	if err := s.githubRequestWithTokenJSON(ctx, token, http.MethodGet, "/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member", nil, &repos); err != nil {
		return nil, err
	}
	result := make([]GitHubMirrorRepo, 0, len(repos))
	for _, repo := range repos {
		result = append(result, mapGitHubRepo(repo))
	}
	return result, nil
}

func (s *Service) CreateGitHubAppRepo(ctx context.Context, userID uuid.UUID, req GitHubMirrorRepoCreateRequest) (*GitHubMirrorRepo, error) {
	token, _, err := s.refreshGitHubAppUserAccessToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	ownerLogin := strings.TrimSpace(req.OwnerLogin)
	repoName := strings.TrimSpace(req.RepoName)
	if ownerLogin == "" || repoName == "" {
		return nil, fmt.Errorf("owner_login and repo_name are required")
	}
	viewer, err := s.fetchGitHubAppViewer(ctx, token)
	if err != nil {
		return nil, err
	}
	body := gitHubCreateRepoRequest{
		Name:        repoName,
		Description: strings.TrimSpace(req.Description),
		Private:     req.Private,
		AutoInit:    false,
	}
	path := "/user/repos"
	if !strings.EqualFold(ownerLogin, viewer.Login) {
		path = "/orgs/" + url.PathEscape(ownerLogin) + "/repos"
	}
	created := gitHubRepoItem{}
	if err := s.githubRequestWithTokenJSON(ctx, token, http.MethodPost, path, body, &created); err != nil {
		return nil, err
	}

	mirror, active, err := s.ensureMirror(ctx, userID)
	if err != nil {
		return nil, err
	}
	mirror.AuthMode = AuthModeGitHubAppUser
	mirror.RemoteName = strings.TrimSpace(req.RemoteName)
	if mirror.RemoteName == "" {
		if active != nil && strings.TrimSpace(active.RemoteName) != "" {
			mirror.RemoteName = strings.TrimSpace(active.RemoteName)
		} else {
			mirror.RemoteName = DefaultRemoteName
		}
	}
	mirror.RemoteBranch = strings.TrimSpace(req.RemoteBranch)
	if mirror.RemoteBranch == "" {
		if branch := strings.TrimSpace(created.DefaultBranch); branch != "" {
			mirror.RemoteBranch = branch
		} else {
			mirror.RemoteBranch = DefaultRemoteBranch
		}
	}
	mirror.RemoteURL = strings.TrimSpace(created.CloneURL)
	mirror.GitHubRepoPermission = mapGitHubRepo(created).ViewerPermission
	now := time.Now().UTC()
	mirror.CreatedAt = mirrorCreatedAt(active, now)
	mirror.UpdatedAt = now
	if err := s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror); err != nil {
		return nil, err
	}
	repo := mapGitHubRepo(created)
	return &repo, nil
}

func (s *Service) testGitHubAppRepoAccess(ctx context.Context, userID uuid.UUID, remoteURL string) (*gitHubRepoAccessResult, error) {
	normalizedRemote, owner, repo, err := normalizeGitHubRemoteURL(remoteURL)
	if err != nil {
		return nil, err
	}
	token, _, err := s.refreshGitHubAppUserAccessToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	viewer, err := s.fetchGitHubAppViewer(ctx, token)
	if err != nil {
		return &gitHubRepoAccessResult{
			OK:                  false,
			Repo:                owner + "/" + repo,
			NormalizedRemoteURL: normalizedRemote,
			Message:             "GitHub App user validation failed. Reconnect GitHub and try again.",
		}, nil
	}

	repoResp := gitHubRepoItem{}
	if err := s.githubRequestWithTokenJSON(ctx, token, http.MethodGet, "/repos/"+owner+"/"+repo, nil, &repoResp); err != nil {
		return &gitHubRepoAccessResult{
			OK:                  false,
			Login:               viewer.Login,
			Repo:                owner + "/" + repo,
			NormalizedRemoteURL: normalizedRemote,
			Message:             "GitHub App user cannot access this repository.",
		}, nil
	}

	permission := githubPermissionFromFlags(repoResp.Permissions.Admin, repoResp.Permissions.Push, repoResp.Permissions.Pull)
	result := &gitHubRepoAccessResult{
		OK:                  canPushPermission(permission),
		Login:               viewer.Login,
		Repo:                strings.TrimSpace(repoResp.FullName),
		NormalizedRemoteURL: normalizedRemote,
		Permission:          permission,
	}
	if result.OK {
		result.Message = "GitHub App user is valid and has push access to this repository."
		return result, nil
	}
	result.Message = "GitHub App user is connected, but it does not have push access to this repository."
	return result, nil
}

func (s *Service) refreshGitHubAppUserAccessToken(ctx context.Context, userID uuid.UUID) (string, *time.Time, error) {
	refreshToken, configured, err := s.readStoredGitHubAppRefreshToken(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	if !configured || strings.TrimSpace(refreshToken) == "" {
		return "", nil, fmt.Errorf("GitHub App user is not connected")
	}
	values := url.Values{
		"client_id":     {s.gitHubAppClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	if strings.TrimSpace(s.gitHubAppClientSecret) != "" {
		values.Set("client_secret", s.gitHubAppClientSecret)
	}
	tokenResp := gitHubAppTokenResponse{}
	if err := s.githubOAuthJSON(ctx, "/login/oauth/access_token", values, &tokenResp); err != nil {
		return "", nil, err
	}
	if tokenResp.Error != "" {
		return "", nil, fmt.Errorf("GitHub refresh token exchange failed: %s", tokenResp.Error)
	}
	if err := s.storeGitHubAppTokenResponse(ctx, userID, tokenResp); err != nil {
		return "", nil, err
	}
	expiresAt := optionalExpiresAt(tokenResp.ExpiresIn)
	return strings.TrimSpace(tokenResp.AccessToken), expiresAt, nil
}

func (s *Service) exchangeGitHubAppCode(ctx context.Context, code string) (gitHubAppTokenResponse, error) {
	values := url.Values{
		"client_id":     {s.gitHubAppClientID},
		"client_secret": {s.gitHubAppClientSecret},
		"code":          {code},
	}
	resp := gitHubAppTokenResponse{}
	err := s.githubOAuthJSON(ctx, "/login/oauth/access_token", values, &resp)
	return resp, err
}

func (s *Service) exchangeGitHubAppDeviceCode(ctx context.Context, deviceCode string) (gitHubAppTokenResponse, error) {
	values := url.Values{
		"client_id":   {s.gitHubAppClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	resp := gitHubAppTokenResponse{}
	err := s.githubOAuthJSON(ctx, "/login/oauth/access_token", values, &resp)
	return resp, err
}

func (s *Service) storeGitHubAppTokenResponse(ctx context.Context, userID uuid.UUID, tokenResp gitHubAppTokenResponse) error {
	if strings.TrimSpace(tokenResp.RefreshToken) == "" {
		return fmt.Errorf("GitHub App refresh token is missing")
	}
	if err := s.writeStoredGitHubAppRefreshToken(ctx, userID, tokenResp.RefreshToken); err != nil {
		return err
	}
	viewer, err := s.fetchGitHubAppViewer(ctx, strings.TrimSpace(tokenResp.AccessToken))
	if err != nil {
		return err
	}
	active, err := s.mirrors.GetActiveLocalGitMirror(ctx, userID)
	if err != nil {
		return err
	}
	if active == nil && !s.isHostedExecution() {
		return nil
	}
	mirror, active, err := s.ensureMirror(ctx, userID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	mirror.GitHubAppUserLogin = strings.TrimSpace(viewer.Login)
	mirror.GitHubAppAuthorizedAt = &now
	mirror.GitHubAppRefreshExpiresAt = optionalExpiresAt(tokenResp.RefreshTokenExpiresIn)
	mirror.CreatedAt = mirrorCreatedAt(active, now)
	mirror.UpdatedAt = now
	return s.mirrors.UpsertActiveLocalGitMirror(ctx, mirror)
}

func (s *Service) fetchGitHubAppViewer(ctx context.Context, accessToken string) (*gitHubUserResponse, error) {
	viewer := gitHubUserResponse{}
	if err := s.githubRequestWithTokenJSON(ctx, accessToken, http.MethodGet, "/user", nil, &viewer); err != nil {
		return nil, err
	}
	return &viewer, nil
}

func (s *Service) githubRequestWithTokenJSON(ctx context.Context, token, method, apiPath string, body any, out any) error {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	fullURL := strings.TrimRight(s.githubAPIBaseURL, "/") + apiPath
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("github api returned %s: %s", resp.Status, msg)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Service) githubOAuthJSON(ctx context.Context, path string, values url.Values, out any) error {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.githubBaseURL, "/")+path, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("github oauth returned %s: %s", resp.Status, msg)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Service) gitHubAppCallbackURL() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("PUBLIC_BASE_URL is required for GitHub App browser auth")
	}
	return base + "/api/git-mirror/github-app/callback", nil
}

func (s *Service) signGitHubAppState(userID uuid.UUID, returnTo string) (string, error) {
	if strings.TrimSpace(s.stateSigningSecret) == "" {
		return "", fmt.Errorf("JWT secret is required for GitHub App state signing")
	}
	claims := gitHubAppStateClaims{
		UserID:   userID.String(),
		Purpose:  gitHubAppStatePurpose,
		ReturnTo: returnTo,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.stateSigningSecret))
}

func (s *Service) parseGitHubAppState(raw string) (*gitHubAppStateClaims, error) {
	token, err := jwt.ParseWithClaims(raw, &gitHubAppStateClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.stateSigningSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*gitHubAppStateClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid state token")
	}
	if claims.Purpose != gitHubAppStatePurpose {
		return nil, fmt.Errorf("invalid state purpose")
	}
	return claims, nil
}

func (c *gitHubAppStateClaims) userID() uuid.UUID {
	id, _ := uuid.Parse(strings.TrimSpace(c.UserID))
	return id
}

func normalizeReturnTo(returnTo string) string {
	trimmed := strings.TrimSpace(returnTo)
	if trimmed == "" {
		return "/git-mirror"
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			trimmed = parsed.RequestURI()
		}
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/git-mirror"
	}
	return trimmed
}

func appendMirrorCallbackStatus(returnTo, status, errMessage string) string {
	target := normalizeReturnTo(returnTo)
	parsed, err := url.Parse(target)
	if err != nil {
		return "/git-mirror"
	}
	query := parsed.Query()
	if status != "" {
		query.Set("github_app_status", status)
	}
	if errMessage != "" {
		query.Set("github_app_error", errMessage)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func optionalExpiresAt(seconds int) *time.Time {
	if seconds <= 0 {
		return nil
	}
	value := time.Now().UTC().Add(time.Duration(seconds) * time.Second)
	return &value
}

func mapGitHubRepo(repo gitHubRepoItem) GitHubMirrorRepo {
	return GitHubMirrorRepo{
		OwnerLogin:       strings.TrimSpace(repo.Owner.Login),
		OwnerType:        strings.ToLower(strings.TrimSpace(repo.Owner.Type)),
		RepoName:         strings.TrimSpace(repo.Name),
		FullName:         strings.TrimSpace(repo.FullName),
		DefaultBranch:    strings.TrimSpace(repo.DefaultBranch),
		CloneURL:         strings.TrimSpace(repo.CloneURL),
		ViewerPermission: githubPermissionFromFlags(repo.Permissions.Admin, repo.Permissions.Push, repo.Permissions.Pull),
	}
}
