package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type FileNode struct {
	Path          string         `json:"path"`
	Name          string         `json:"name"`
	IsDir         bool           `json:"is_dir"`
	Source        string         `json:"source,omitempty"`
	Kind          string         `json:"kind,omitempty"`
	Content       string         `json:"content,omitempty"`
	MimeType      string         `json:"mime_type,omitempty"`
	Size          int64          `json:"size,omitempty"`
	Version       int64          `json:"version,omitempty"`
	Checksum      string         `json:"checksum,omitempty"`
	Metadata      interface{}    `json:"metadata,omitempty"`
	BundleContext *BundleContext `json:"bundle_context,omitempty"`
	Children      []*FileNode    `json:"children,omitempty"`
	CreatedAt     string         `json:"created_at,omitempty"`
	UpdatedAt     string         `json:"updated_at,omitempty"`
	DeletedAt     string         `json:"deleted_at,omitempty"`
}

type BundleContext struct {
	Kind          string                 `json:"kind"`
	Name          string                 `json:"name"`
	Path          string                 `json:"path"`
	Source        string                 `json:"source,omitempty"`
	ReadOnly      bool                   `json:"read_only,omitempty"`
	Description   string                 `json:"description,omitempty"`
	WhenToUse     string                 `json:"when_to_use,omitempty"`
	Status        string                 `json:"status,omitempty"`
	PrimaryPath   string                 `json:"primary_path,omitempty"`
	LogPath       string                 `json:"log_path,omitempty"`
	Capabilities  []string               `json:"capabilities,omitempty"`
	AllowedTools  []string               `json:"allowed_tools,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Arguments     map[string]interface{} `json:"arguments,omitempty"`
	Activation    map[string]interface{} `json:"activation,omitempty"`
	MinTrustLevel int                    `json:"min_trust_level,omitempty"`
	RelativePath  string                 `json:"relative_path"`
}

type WriteFileRequest struct {
	Content          string                 `json:"content"`
	MimeType         string                 `json:"mime_type,omitempty"`
	IsDir            bool                   `json:"is_dir"`
	Source           string                 `json:"source,omitempty"`
	SourcePlatform   string                 `json:"source_platform,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	MinTrustLevel    int                    `json:"min_trust_level,omitempty"`
	ExpectedVersion  *int64                 `json:"expected_version,omitempty"`
	ExpectedChecksum string                 `json:"expected_checksum,omitempty"`
}

type SearchRequest struct {
	Query string `json:"q"`
	Path  string `json:"path,omitempty"`
}

type SnapshotResponse struct {
	Path         string      `json:"path"`
	Cursor       int64       `json:"cursor"`
	RootChecksum string      `json:"root_checksum"`
	Entries      []*FileNode `json:"entries"`
}

type ChangesResponse struct {
	Path       string                   `json:"path"`
	FromCursor int64                    `json:"from_cursor"`
	NextCursor int64                    `json:"next_cursor"`
	Changes    []map[string]interface{} `json:"changes"`
}

func (s *Server) handleTreeRead(w http.ResponseWriter, r *http.Request) {
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())
	path := chi.URLParam(r, "*")
	if isHiddenPublicFeaturePath(path) {
		respondNotFound(w, "file")
		return
	}
	node, err := s.readOrListTreePath(r.Context(), userID, trustLevel, path)
	if err != nil {
		respondNotFound(w, "file")
		return
	}

	respondOK(w, node)
}

func (s *Server) handleTreeWrite(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		respondValidationError(w, "path", "path is required")
		return
	}
	if isHiddenPublicFeaturePath(path) {
		respondNotFound(w, "file")
		return
	}
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	var req WriteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}
	ctx := s.requestSourceContext(r, "manual")
	ctx, req.Metadata = applyExplicitSourceHints(ctx, req.Metadata, req.Source, req.SourcePlatform)

	if req.IsDir {
		entry, err := s.FileTreeService.EnsureDirectoryWithMetadata(ctx, userID, path, req.Metadata, req.MinTrustLevel)
		if err != nil {
			if errors.Is(err, services.ErrReadOnlyPath) {
				respondForbidden(w, err.Error())
				return
			}
			respondInternalError(w, err)
			return
		}
		respondOKWithLocalGitSync(w, fileTreeEntryToNode(s.renderSystemSkillEntry(r.Context(), userID, trustLevelFromCtx(r.Context()), entry)), s.syncLocalGitMirror(r.Context(), userID))
		return
	}

	mimeType := req.MimeType
	if mimeType == "" {
		mimeType = "text/plain"
	}

	minTrustLevel := req.MinTrustLevel
	if minTrustLevel <= 0 {
		minTrustLevel = models.TrustLevelFull
	}
	writeMetadata := req.Metadata
	if services.EntrySourceFromMetadata(writeMetadata) == "" {
		if _, readErr := s.FileTreeService.Read(r.Context(), userID, path, trustLevelFromCtx(r.Context())); errors.Is(readErr, services.ErrEntryNotFound) {
			writeMetadata = services.WithSourceMetadata(writeMetadata, services.SourceOrDefault(ctx, "manual"))
		}
	}
	entry, err := s.FileTreeService.WriteEntry(ctx, userID, path, req.Content, mimeType, models.FileTreeWriteOptions{
		Metadata:         writeMetadata,
		MinTrustLevel:    minTrustLevel,
		ExpectedVersion:  req.ExpectedVersion,
		ExpectedChecksum: req.ExpectedChecksum,
	})
	if err != nil {
		if errors.Is(err, services.ErrOptimisticLockConflict) {
			respondError(w, http.StatusConflict, ErrCodeConflict, err.Error())
			return
		}
		if errors.Is(err, services.ErrReadOnlyPath) {
			respondForbidden(w, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	respondOKWithLocalGitSync(w, fileTreeEntryToNode(s.renderSystemSkillEntry(r.Context(), userID, trustLevelFromCtx(r.Context()), entry)), s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleTreeDelete(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		respondValidationError(w, "path", "path is required")
		return
	}
	if isHiddenPublicFeaturePath(path) {
		respondNotFound(w, "file")
		return
	}
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if err := s.FileTreeService.Delete(r.Context(), userID, path); err != nil {
		if errors.Is(err, services.ErrReadOnlyPath) {
			respondForbidden(w, err.Error())
			return
		}
		respondNotFound(w, "file")
		return
	}

	respondOKWithLocalGitSync(w, map[string]string{"status": "deleted", "path": hubpath.NormalizePublic(path)}, s.syncLocalGitMirror(r.Context(), userID))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondValidationError(w, "q", "query parameter 'q' is required")
		return
	}
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())

	results, err := s.searchHub(r.Context(), userID, trustLevel, query, "")
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"query":   query,
		"results": results,
	})
}

func fileTreeEntryToNode(e *models.FileTreeEntry) *FileNode {
	publicPath := hubpath.StorageToPublic(e.Path)
	deletedAt := ""
	if e.DeletedAt != nil {
		deletedAt = e.DeletedAt.Format("2006-01-02T15:04:05Z")
	}
	size := int64(len(e.Content))
	if raw, ok := e.Metadata["size_bytes"]; ok {
		switch typed := raw.(type) {
		case int:
			size = int64(typed)
		case int64:
			size = typed
		case float64:
			size = int64(typed)
		}
	}
	node := &FileNode{
		Path:      publicPath,
		Name:      hubpath.BaseName(publicPath),
		IsDir:     e.IsDirectory,
		Source:    services.EntrySource(e),
		Kind:      e.Kind,
		Content:   e.Content,
		MimeType:  e.ContentType,
		Size:      size,
		Version:   e.Version,
		Checksum:  e.Checksum,
		Metadata:  e.Metadata,
		CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: e.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		DeletedAt: deletedAt,
	}
	if e.IsDirectory {
		if summary := services.BundleSummaryFromMetadata(e.Path, e.Metadata, e.MinTrustLevel); summary != nil {
			node.BundleContext = apiBundleContext(services.BundleContextFromSummary(*summary, publicPath))
		}
	}
	return node
}

func (s *Server) handleTreeSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	if isHiddenPublicFeaturePath(path) {
		respondNotFound(w, "file")
		return
	}
	trustLevel := trustLevelFromCtx(r.Context())
	snapshot, err := s.FileTreeService.Snapshot(r.Context(), userID, path, trustLevel)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	nodes := make([]*FileNode, 0, len(snapshot.Entries))
	for _, entry := range filterVisibleEntries(snapshot.Entries) {
		nodes = append(nodes, fileTreeEntryToNode(&entry))
	}
	respondOK(w, SnapshotResponse{
		Path:         snapshot.Path,
		Cursor:       snapshot.Cursor,
		RootChecksum: snapshot.RootChecksum,
		Entries:      nodes,
	})
}

func (s *Server) handleTreeChanges(w http.ResponseWriter, r *http.Request) {
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	cursorText := r.URL.Query().Get("cursor")
	if cursorText == "" {
		cursorText = "0"
	}
	cursor, err := strconv.ParseInt(cursorText, 10, 64)
	if err != nil {
		respondValidationError(w, "cursor", "cursor must be an integer")
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	if isHiddenPublicFeaturePath(path) {
		respondNotFound(w, "file")
		return
	}

	changes, nextCursor, err := s.FileTreeService.Changes(r.Context(), userID, cursor, path, trustLevelFromCtx(r.Context()))
	if err != nil {
		respondInternalError(w, err)
		return
	}

	payload := make([]map[string]interface{}, 0, len(changes))
	for _, change := range changes {
		if isHiddenPublicFeaturePath(change.Entry.Path) {
			continue
		}
		node := fileTreeEntryToNode(&change.Entry)
		payload = append(payload, map[string]interface{}{
			"cursor":      change.Cursor,
			"change_type": change.ChangeType,
			"entry":       node,
		})
	}
	respondOK(w, ChangesResponse{
		Path:       hubpath.NormalizePublic(path),
		FromCursor: cursor,
		NextCursor: nextCursor,
		Changes:    payload,
	})
}

func (s *Server) readOrListTreePath(ctx context.Context, userID uuid.UUID, trustLevel int, rawPath string) (*FileNode, error) {
	if rawPath == "" {
		rawPath = "/"
	}

	storagePath := hubpath.NormalizeStorage(rawPath)
	if storagePath == "/" || strings.HasSuffix(rawPath, "/") || strings.HasSuffix(storagePath, "/") {
		return s.listTreeNode(ctx, userID, trustLevel, storagePath)
	}

	entry, err := s.FileTreeService.Read(ctx, userID, storagePath, trustLevel)
	if err == nil {
		if entry.IsDirectory {
			return s.listTreeNode(ctx, userID, trustLevel, storagePath)
		}
		return fileTreeEntryToNode(s.renderSystemSkillEntry(ctx, userID, trustLevel, entry)), nil
	}

	// Only fall through to directory listing if the read error indicates "not found".
	// For other errors (database, permission, etc.), propagate the real error.
	node, listErr := s.listTreeNode(ctx, userID, trustLevel, storagePath)
	if listErr != nil {
		// If listing also fails, return the original read error for better diagnostics.
		return nil, err
	}
	return node, nil
}

func (s *Server) listTreeNode(ctx context.Context, userID uuid.UUID, trustLevel int, storagePath string) (*FileNode, error) {
	entries, err := s.FileTreeService.List(ctx, userID, storagePath, trustLevel)
	if err != nil {
		return nil, err
	}
	entries = filterVisibleEntries(entries)

	publicPath := hubpath.StorageToPublic(storagePath)
	if publicPath != "/" && !strings.HasSuffix(publicPath, "/") {
		publicPath += "/"
	}

	children := make([]*FileNode, 0, len(entries))
	for _, e := range entries {
		rendered := s.renderSystemSkillEntry(ctx, userID, trustLevel, &e)
		children = append(children, fileTreeEntryToNode(rendered))
	}

	return &FileNode{
		Path:          publicPath,
		Name:          hubpath.BaseName(publicPath),
		IsDir:         true,
		BundleContext: s.bundleContextForDirectory(ctx, userID, trustLevel, publicPath),
		Children:      children,
	}, nil
}

func (s *Server) bundleContextForDirectory(ctx context.Context, userID uuid.UUID, trustLevel int, currentPath string) *BundleContext {
	if s.FileTreeService == nil {
		return nil
	}
	context := services.BundleContextForPath(
		currentPath,
		func(path string) (*models.FileTreeEntry, error) {
			entry, err := s.FileTreeService.Read(ctx, userID, path, trustLevel)
			if err != nil {
				return nil, err
			}
			return s.renderSystemSkillEntry(ctx, userID, trustLevel, entry), nil
		},
		func(path string) ([]models.FileTreeEntry, error) {
			entries, err := s.FileTreeService.List(ctx, userID, path, trustLevel)
			if err != nil {
				return nil, err
			}
			rendered := make([]models.FileTreeEntry, 0, len(entries))
			for idx := range entries {
				renderedEntry := s.renderSystemSkillEntry(ctx, userID, trustLevel, &entries[idx])
				if renderedEntry == nil {
					continue
				}
				rendered = append(rendered, *renderedEntry)
			}
			return rendered, nil
		},
	)
	return apiBundleContext(context)
}

func apiBundleContext(context *models.BundleContext) *BundleContext {
	if context == nil {
		return nil
	}
	return &BundleContext{
		Kind:          context.Kind,
		Name:          context.Name,
		Path:          context.Path,
		Source:        context.Source,
		ReadOnly:      context.ReadOnly,
		Description:   context.Description,
		WhenToUse:     context.WhenToUse,
		Status:        context.Status,
		PrimaryPath:   context.PrimaryPath,
		LogPath:       context.LogPath,
		Capabilities:  append([]string{}, context.Capabilities...),
		AllowedTools:  append([]string{}, context.AllowedTools...),
		Tags:          append([]string{}, context.Tags...),
		Arguments:     cloneStringMap(context.Arguments),
		Activation:    cloneStringMap(context.Activation),
		MinTrustLevel: context.MinTrustLevel,
		RelativePath:  context.RelativePath,
	}
}

func cloneStringMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
