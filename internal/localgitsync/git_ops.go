package localgitsync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/agi-bar/neuDrive/internal/models"
	"github.com/google/uuid"
)

const remoteConflictPushBlockedMessage = "Remote has commits that are not in this neuDrive mirror. Review the remote changes, then confirm overwrite to push with force-with-lease."

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
		blocked, expectedRemoteHead := prepareRemotePush(ctx, mirror, pushToken)
		if blocked {
			return result, nil
		}
		if mirror.ForceRemoteOverwrite && expectedRemoteHead != "" {
			if err := gitPushForceWithLeaseWithToken(ctx, mirror.RootPath, mirror.RemoteName, mirror.RemoteBranch, expectedRemoteHead, pushToken); err != nil {
				mirror.ForceRemoteOverwrite = false
				mirror.RemoteConflict = true
				mirror.LastPushError = fmt.Sprintf("Force overwrite failed; the remote may have changed again. %s", err.Error())
				return result, nil
			}
		} else if err := gitPushWithToken(ctx, mirror.RootPath, mirror.RemoteName, mirror.RemoteBranch, pushToken); err != nil {
			mirror.ForceRemoteOverwrite = false
			mirror.LastPushError = err.Error()
			if isNonFastForwardPushError(err) {
				mirror.RemoteConflict = true
			}
			return result, nil
		}
	default:
		if err := ensureGitRemote(ctx, mirror.RootPath, mirror.RemoteName, pushRemoteURL); err != nil {
			return result, err
		}
		result.pushAttempted = true
		blocked, expectedRemoteHead := prepareRemotePush(ctx, mirror, "")
		if blocked {
			return result, nil
		}
		if mirror.ForceRemoteOverwrite && expectedRemoteHead != "" {
			if err := gitPushForceWithLease(ctx, mirror.RootPath, mirror.RemoteName, mirror.RemoteBranch, expectedRemoteHead); err != nil {
				mirror.ForceRemoteOverwrite = false
				mirror.RemoteConflict = true
				mirror.LastPushError = fmt.Sprintf("Force overwrite failed; the remote may have changed again. %s", err.Error())
				return result, nil
			}
		} else if err := gitPush(ctx, mirror.RootPath, mirror.RemoteName, mirror.RemoteBranch); err != nil {
			mirror.ForceRemoteOverwrite = false
			mirror.LastPushError = err.Error()
			if isNonFastForwardPushError(err) {
				mirror.RemoteConflict = true
			}
			return result, nil
		}
	}

	now := time.Now().UTC()
	mirror.LastPushAt = &now
	mirror.LastPushError = ""
	mirror.RemoteConflict = false
	mirror.ForceRemoteOverwrite = false
	result.pushSucceeded = true
	return result, nil
}

func prepareRemotePush(ctx context.Context, mirror *models.LocalGitMirror, token string) (bool, string) {
	if mirror == nil {
		return true, ""
	}
	remoteHead, remoteMissing, err := fetchAndResolveRemoteHead(ctx, mirror.RootPath, mirror.RemoteName, mirror.RemoteBranch, token)
	if err != nil {
		mirror.ForceRemoteOverwrite = false
		mirror.LastPushError = err.Error()
		return true, ""
	}
	if remoteMissing || strings.TrimSpace(remoteHead) == "" {
		mirror.RemoteConflict = false
		return false, ""
	}
	localHead, err := gitRevParse(ctx, mirror.RootPath, "HEAD")
	if err != nil {
		mirror.ForceRemoteOverwrite = false
		mirror.LastPushError = err.Error()
		return true, remoteHead
	}
	if strings.TrimSpace(localHead) == strings.TrimSpace(remoteHead) {
		mirror.RemoteConflict = false
		return false, remoteHead
	}
	remoteIsAncestor, err := gitIsAncestor(ctx, mirror.RootPath, remoteHead, localHead)
	if err != nil {
		mirror.ForceRemoteOverwrite = false
		mirror.LastPushError = err.Error()
		return true, remoteHead
	}
	if remoteIsAncestor {
		mirror.RemoteConflict = false
		return false, remoteHead
	}
	if mirror.ForceRemoteOverwrite {
		mirror.RemoteConflict = false
		return false, remoteHead
	}
	mirror.ForceRemoteOverwrite = false
	mirror.RemoteConflict = true
	mirror.LastPushError = remoteConflictPushBlockedMessage
	return true, remoteHead
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
	args := append(gitCredentialHelperArgs(), "push", remoteName, "HEAD:"+remoteBranch)
	_, err := runGitCommand(ctx, rootPath, map[string]string{
		"NEUDRIVE_GITHUB_TOKEN": token,
		"GIT_TERMINAL_PROMPT":   "0",
	}, args...)
	return err
}

func gitPushForceWithLease(ctx context.Context, rootPath, remoteName, remoteBranch, expectedRemoteHead string) error {
	_, err := runGitCommand(ctx, rootPath, map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
	}, "push", "--force-with-lease=refs/heads/"+remoteBranch+":"+expectedRemoteHead, remoteName, "HEAD:"+remoteBranch)
	return err
}

func gitPushForceWithLeaseWithToken(ctx context.Context, rootPath, remoteName, remoteBranch, expectedRemoteHead, token string) error {
	args := append(gitCredentialHelperArgs(), "push", "--force-with-lease=refs/heads/"+remoteBranch+":"+expectedRemoteHead, remoteName, "HEAD:"+remoteBranch)
	_, err := runGitCommand(ctx, rootPath, map[string]string{
		"NEUDRIVE_GITHUB_TOKEN": token,
		"GIT_TERMINAL_PROMPT":   "0",
	}, args...)
	return err
}

func fetchAndResolveRemoteHead(ctx context.Context, rootPath, remoteName, remoteBranch, token string) (string, bool, error) {
	if strings.TrimSpace(token) == "" {
		if err := gitFetchBranch(ctx, rootPath, remoteName, remoteBranch); err != nil {
			if isMissingRemoteBranchError(err) {
				return "", true, nil
			}
			return "", false, err
		}
	} else if err := gitFetchBranchWithToken(ctx, rootPath, remoteName, remoteBranch, token); err != nil {
		if isMissingRemoteBranchError(err) {
			return "", true, nil
		}
		return "", false, err
	}
	remoteHead, err := gitRevParse(ctx, rootPath, remoteTrackingRef(remoteName, remoteBranch))
	if err != nil {
		if isBadRevisionError(err) {
			return "", true, nil
		}
		return "", false, err
	}
	return remoteHead, false, nil
}

func gitFetchBranch(ctx context.Context, rootPath, remoteName, remoteBranch string) error {
	_, err := runGitCommand(ctx, rootPath, map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
	}, "fetch", "--prune", remoteName, remoteBranch)
	return err
}

func gitFetchBranchWithToken(ctx context.Context, rootPath, remoteName, remoteBranch, token string) error {
	args := append(gitCredentialHelperArgs(), "fetch", "--prune", remoteName, remoteBranch)
	_, err := runGitCommand(ctx, rootPath, map[string]string{
		"NEUDRIVE_GITHUB_TOKEN": token,
		"GIT_TERMINAL_PROMPT":   "0",
	}, args...)
	return err
}

func gitRevParse(ctx context.Context, rootPath, revision string) (string, error) {
	return runGitCommand(ctx, rootPath, nil, "rev-parse", "--verify", revision)
}

func gitIsAncestor(ctx context.Context, rootPath, ancestor, descendant string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", rootPath, "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Env = gitCommandEnv(nil)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		if strings.TrimSpace(string(output)) == "" {
			return false, nil
		}
		return false, fmt.Errorf("git merge-base --is-ancestor failed: %s", strings.TrimSpace(string(output)))
	}
	return false, err
}

func remoteTrackingRef(remoteName, remoteBranch string) string {
	return "refs/remotes/" + strings.TrimSpace(remoteName) + "/" + strings.TrimSpace(remoteBranch)
}

func gitCredentialHelperArgs() []string {
	return []string{
		"-c", "credential.helper=",
		"-c", "credential.helper=!f() { echo username=x-access-token; echo password=$NEUDRIVE_GITHUB_TOKEN; }; f",
	}
}

func isMissingRemoteBranchError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "couldn't find remote ref") ||
		strings.Contains(message, "could not find remote ref") ||
		strings.Contains(message, "remote ref does not exist")
}

func isBadRevisionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "needed a single revision") ||
		strings.Contains(message, "unknown revision") ||
		strings.Contains(message, "ambiguous argument")
}

func isNonFastForwardPushError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "non-fast-forward") ||
		strings.Contains(message, "fetch first") ||
		strings.Contains(message, "stale info") ||
		strings.Contains(message, "failed to push some refs")
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
