package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/agi-bar/agenthub/internal/localgitsync"
)

type localAPIEnvelope struct {
	OK           bool                   `json:"ok"`
	Data         json.RawMessage        `json:"data"`
	LocalGitSync *localgitsync.SyncInfo `json:"local_git_sync,omitempty"`
	Error        struct {
		Message string `json:"message"`
	} `json:"error"`
}

func localPlatformAPIPostJSON(ctx context.Context, apiBase, token, apiPath string, requestBody any, out any) (*localgitsync.SyncInfo, error) {
	return localPlatformAPIJSON(ctx, http.MethodPost, apiBase, token, apiPath, requestBody, out)
}

func localPlatformAPIDelete(ctx context.Context, apiBase, token, apiPath string, out any) (*localgitsync.SyncInfo, error) {
	return localPlatformAPIJSON(ctx, http.MethodDelete, apiBase, token, apiPath, nil, out)
}

func localPlatformAPIJSON(ctx context.Context, method, apiBase, token, apiPath string, requestBody any, out any) (*localgitsync.SyncInfo, error) {
	if strings.TrimSpace(apiBase) == "" {
		return nil, errors.New("missing local daemon API base")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing local owner token")
	}

	fullURL, err := joinLocalPlatformAPIURL(apiBase, apiPath)
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
	req.Header.Set("Authorization", "Bearer "+token)
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("unexpected API response (%s): %s", resp.Status, snippet)
	}
	if !envelope.OK {
		if envelope.Error.Message != "" {
			return envelope.LocalGitSync, errors.New(envelope.Error.Message)
		}
		return envelope.LocalGitSync, fmt.Errorf("unexpected API error (%s)", resp.Status)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return envelope.LocalGitSync, err
		}
	}
	return envelope.LocalGitSync, nil
}

func joinLocalPlatformAPIURL(apiBase, apiPath string) (string, error) {
	base, err := url.Parse(strings.TrimRight(apiBase, "/"))
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + apiPath
	return base.String(), nil
}

func resolveLocalPath(pathValue string) (string, error) {
	if strings.TrimSpace(pathValue) == "" {
		return "", errors.New("path is required")
	}
	expanded := expandUser(strings.TrimSpace(pathValue))
	return filepath.Abs(expanded)
}

func defaultExportRoot(platform string) (string, error) {
	return filepath.Abs(filepath.Join(".", "agenthub-export", platform))
}
