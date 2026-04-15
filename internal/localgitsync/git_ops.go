package localgitsync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

func (s *Service) finalizeMirrorRepo(ctx context.Context, userID uuid.UUID, mirror *models.LocalGitMirror) (repoSyncResult, error) {
	if mirror == nil {
		return repoSyncResult{}, fmt.Errorf("missing mirror configuration")
	}
	result := repoSyncResult{}
	dirty, err := gitWorkTreeDirty(ctx, mirror.RootPath)
	if err != nil {
		return result, err
	}
	if dirty && mirror.AutoCommitEnabled {
		if err := gitAddAll(ctx, mirror.RootPath); err != nil {
			return result, err
		}
		commitHash, err := gitCommitAll(ctx, mirror.RootPath, time.Now().UTC())
		if err != nil {
			return result, err
		}
		now := time.Now().UTC()
		mirror.LastCommitAt = &now
		mirror.LastCommitHash = commitHash
		result.commitCreated = true
	}
	if !mirror.AutoPushEnabled {
		return result, nil
	}
	if strings.TrimSpace(mirror.RemoteURL) == "" {
		return result, fmt.Errorf("auto push is enabled but no remote URL is configured")
	}

	pushRemoteURL := strings.TrimSpace(mirror.RemoteURL)
	pushToken := ""
	switch mirror.AuthMode {
	case AuthModeGitHubToken, AuthModeGitHubAppUser:
		normalizedURL, _, _, err := normalizeGitHubRemoteURL(pushRemoteURL)
		if err != nil {
			return result, err
		}
		pushRemoteURL = normalizedURL
		mirror.RemoteURL = normalizedURL
		switch mirror.AuthMode {
		case AuthModeGitHubToken:
			token, configured, err := s.readStoredGitHubToken(ctx, userID)
			if err != nil {
				return result, err
			}
			if !configured || strings.TrimSpace(token) == "" {
				return result, fmt.Errorf("auto push is enabled but no GitHub token is configured")
			}
			pushToken = token
		case AuthModeGitHubAppUser:
			token, _, err := s.refreshGitHubAppUserAccessToken(ctx, userID)
			if err != nil {
				return result, err
			}
			if strings.TrimSpace(token) == "" {
				return result, fmt.Errorf("connect the GitHub App account before enabling auto push")
			}
			pushToken = token
		}
		if err := ensureGitRemote(ctx, mirror.RootPath, mirror.RemoteName, pushRemoteURL); err != nil {
			return result, err
		}
		result.pushAttempted = true
		if err := gitPushWithToken(ctx, mirror.RootPath, mirror.RemoteName, mirror.RemoteBranch, pushToken); err != nil {
			mirror.LastPushError = err.Error()
			return result, nil
		}
	default:
		if err := ensureGitRemote(ctx, mirror.RootPath, mirror.RemoteName, pushRemoteURL); err != nil {
			return result, err
		}
		result.pushAttempted = true
		if err := gitPush(ctx, mirror.RootPath, mirror.RemoteName, mirror.RemoteBranch); err != nil {
			mirror.LastPushError = err.Error()
			return result, nil
		}
	}

	now := time.Now().UTC()
	mirror.LastPushAt = &now
	mirror.LastPushError = ""
	result.pushSucceeded = true
	return result, nil
}

func gitWorkTreeDirty(ctx context.Context, rootPath string) (bool, error) {
	out, err := runGitCommand(ctx, rootPath, nil, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func gitAddAll(ctx context.Context, rootPath string) error {
	_, err := runGitCommand(ctx, rootPath, nil, "add", "-A")
	return err
}

func gitCommitAll(ctx context.Context, rootPath string, now time.Time) (string, error) {
	env := map[string]string{
		"GIT_AUTHOR_NAME":     commitAuthorName,
		"GIT_AUTHOR_EMAIL":    commitAuthorEmail,
		"GIT_COMMITTER_NAME":  commitAuthorName,
		"GIT_COMMITTER_EMAIL": commitAuthorEmail,
	}
	if _, err := runGitCommand(ctx, rootPath, env, "commit", "-m", fmt.Sprintf("neudrive mirror sync: %s", now.Format(time.RFC3339))); err != nil {
		return "", err
	}
	return runGitCommand(ctx, rootPath, nil, "rev-parse", "HEAD")
}

func ensureGitRemote(ctx context.Context, rootPath, remoteName, remoteURL string) error {
	currentURL, err := runGitCommand(ctx, rootPath, nil, "remote", "get-url", remoteName)
	if err != nil {
		if strings.Contains(err.Error(), "No such remote") || strings.Contains(err.Error(), "No such remote '"+remoteName+"'") {
			_, addErr := runGitCommand(ctx, rootPath, nil, "remote", "add", remoteName, remoteURL)
			return addErr
		}
		_, addErr := runGitCommand(ctx, rootPath, nil, "remote", "add", remoteName, remoteURL)
		return addErr
	}
	if strings.TrimSpace(currentURL) == strings.TrimSpace(remoteURL) {
		return nil
	}
	_, err = runGitCommand(ctx, rootPath, nil, "remote", "set-url", remoteName, remoteURL)
	return err
}

func gitPush(ctx context.Context, rootPath, remoteName, remoteBranch string) error {
	_, err := runGitCommand(ctx, rootPath, map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
	}, "push", remoteName, "HEAD:"+remoteBranch)
	return err
}

func gitPushWithToken(ctx context.Context, rootPath, remoteName, remoteBranch, token string) error {
	_, err := runGitCommand(ctx, rootPath, map[string]string{
		"NEUDRIVE_GITHUB_TOKEN": token,
		"GIT_TERMINAL_PROMPT":   "0",
	}, "-c", "credential.helper=", "-c", "credential.helper=!f() { echo username=x-access-token; echo password=$NEUDRIVE_GITHUB_TOKEN; }; f", "push", remoteName, "HEAD:"+remoteBranch)
	return err
}

func runGitCommand(ctx context.Context, rootPath string, extraEnv map[string]string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", rootPath}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = gitCommandEnv(extraEnv)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			trimmed = err.Error()
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), trimmed)
	}
	return trimmed, nil
}

func gitCommandEnv(extraEnv map[string]string) []string {
	env := scrubGitEnv(os.Environ())
	for key, value := range extraEnv {
		env = append(env, key+"="+value)
	}
	return env
}
