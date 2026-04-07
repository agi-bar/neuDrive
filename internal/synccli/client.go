package synccli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/models"
)

type APIError struct {
	Status  int
	Code    string
	Message string
	Body    string
}

func (e *APIError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if strings.TrimSpace(e.Body) != "" {
		return e.Body
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(baseURL, token string) *client {
	return &client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *client) getAuthInfo(ctx context.Context) (*models.AgentAuthInfo, error) {
	var info models.AgentAuthInfo
	err := c.doJSON(ctx, http.MethodGet, "/agent/auth/whoami", nil, &info)
	if err == nil {
		return &info, nil
	}
	apiErr := &APIError{}
	if errorsAs(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		if fallbackErr := c.doJSON(ctx, http.MethodGet, "/agent/auth/info", nil, &info); fallbackErr == nil {
			return &info, nil
		}
	}
	return nil, err
}

func (c *client) previewBundle(ctx context.Context, bundle *models.Bundle, manifest *models.BundleArchiveManifest) (*models.BundlePreviewResult, error) {
	if bundle == nil && manifest == nil {
		return nil, fmt.Errorf("preview payload is required")
	}
	if manifest != nil {
		payload := models.BundlePreviewRequest{Manifest: manifest}
		var result models.BundlePreviewResult
		if err := c.doJSON(ctx, http.MethodPost, "/agent/import/preview", payload, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	var result models.BundlePreviewResult
	if err := c.doJSON(ctx, http.MethodPost, "/agent/import/preview", bundle, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *client) importBundle(ctx context.Context, bundle models.Bundle) (*models.BundleImportResult, error) {
	var result models.BundleImportResult
	if err := c.doJSON(ctx, http.MethodPost, "/agent/import/bundle", bundle, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *client) exportBundleJSON(ctx context.Context, filters models.BundleFilters) (*models.Bundle, error) {
	var result models.Bundle
	if err := c.doJSON(ctx, http.MethodGet, "/agent/export/bundle?format=json"+bundleFilterQuery(filters), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *client) exportBundleArchive(ctx context.Context, filters models.BundleFilters) ([]byte, error) {
	return c.doBytes(ctx, http.MethodGet, "/agent/export/bundle?format=archive"+bundleFilterQuery(filters))
}

func (c *client) startSyncSession(ctx context.Context, request models.SyncStartSessionRequest) (*models.SyncSessionResponse, error) {
	var result models.SyncSessionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/agent/import/session", request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *client) uploadPart(ctx context.Context, sessionID string, index int, data []byte) (*models.SyncSessionResponse, error) {
	var result models.SyncSessionResponse
	if err := c.doBytesJSON(ctx, http.MethodPut, fmt.Sprintf("/agent/import/session/%s/parts/%d", url.PathEscape(sessionID), index), data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *client) getSyncSession(ctx context.Context, sessionID string) (*models.SyncSessionResponse, error) {
	var result models.SyncSessionResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/agent/import/session/%s", url.PathEscape(sessionID)), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *client) commitSession(ctx context.Context, sessionID string, previewFingerprint string) (*models.BundleImportResult, error) {
	req := models.SyncCommitRequest{PreviewFingerprint: previewFingerprint}
	var result models.BundleImportResult
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/agent/import/session/%s/commit", url.PathEscape(sessionID)), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *client) abortSession(ctx context.Context, sessionID string) error {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/agent/import/session/%s", url.PathEscape(sessionID)), nil, nil)
}

func (c *client) listSyncJobs(ctx context.Context) ([]models.SyncJob, error) {
	var resp struct {
		Jobs []models.SyncJob `json:"jobs"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/agent/sync/jobs", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Jobs, nil
}

func (c *client) resumeSession(ctx context.Context, sessionID string, archive []byte) (*models.SyncSessionResponse, error) {
	state, err := c.getSyncSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	chunkSize := state.ChunkSizeBytes
	if chunkSize <= 0 {
		chunkSize = models.DefaultSyncChunkSize
	}
	for _, index := range state.MissingParts {
		start := int64(index) * chunkSize
		if start >= int64(len(archive)) {
			break
		}
		end := start + chunkSize
		if end > int64(len(archive)) {
			end = int64(len(archive))
		}
		state, err = c.uploadPart(ctx, sessionID, index, archive[start:end])
		if err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (c *client) doBytes(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp.StatusCode, body)
	}
	return body, nil
}

func (c *client) doBytesJSON(ctx context.Context, method, path string, payload []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, body)
	}
	return decodeSuccess(body, out)
}

func (c *client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var bodyReader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, body)
	}
	return decodeSuccess(body, out)
}

func decodeSuccess(body []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Data != nil {
		return json.Unmarshal(envelope.Data, out)
	}
	return json.Unmarshal(body, out)
}

func decodeAPIError(status int, body []byte) error {
	var apiErr struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil {
		if strings.TrimSpace(apiErr.Message) != "" || strings.TrimSpace(apiErr.Code) != "" {
			return &APIError{Status: status, Code: apiErr.Code, Message: apiErr.Message, Body: string(body)}
		}
		if apiErr.Error != nil {
			var nested struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(apiErr.Error, &nested); err == nil {
				return &APIError{Status: status, Message: nested.Message, Body: string(body)}
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = http.StatusText(status)
	}
	return &APIError{Status: status, Body: text}
}

func bundleFilterQuery(filters models.BundleFilters) string {
	values := url.Values{}
	for _, value := range filters.IncludeDomains {
		values.Add("include_domain", value)
	}
	for _, value := range filters.IncludeSkills {
		values.Add("include_skill", value)
	}
	for _, value := range filters.ExcludeSkills {
		values.Add("exclude_skill", value)
	}
	encoded := values.Encode()
	if encoded == "" {
		return ""
	}
	return "&" + encoded
}

func errorsAs(err error, target interface{}) bool {
	switch target := target.(type) {
	case **APIError:
		apiErr, ok := err.(*APIError)
		if ok {
			*target = apiErr
			return true
		}
	}
	return false
}
