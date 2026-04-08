package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/agi-bar/agenthub/internal/services"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type FileNode struct {
	Path      string      `json:"path"`
	Name      string      `json:"name"`
	IsDir     bool        `json:"is_dir"`
	Kind      string      `json:"kind,omitempty"`
	Content   string      `json:"content,omitempty"`
	MimeType  string      `json:"mime_type,omitempty"`
	Size      int64       `json:"size,omitempty"`
	Version   int64       `json:"version,omitempty"`
	Checksum  string      `json:"checksum,omitempty"`
	Metadata  interface{} `json:"metadata,omitempty"`
	Children  []*FileNode `json:"children,omitempty"`
	CreatedAt string      `json:"created_at,omitempty"`
	UpdatedAt string      `json:"updated_at,omitempty"`
	DeletedAt string      `json:"deleted_at,omitempty"`
}

type WriteFileRequest struct {
	Content          string                 `json:"content"`
	MimeType         string                 `json:"mime_type,omitempty"`
	IsDir            bool                   `json:"is_dir"`
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

	if req.IsDir {
		entry, err := s.FileTreeService.EnsureDirectoryWithMetadata(r.Context(), userID, path, req.Metadata, req.MinTrustLevel)
		if err != nil {
			if errors.Is(err, services.ErrReadOnlyPath) {
				respondForbidden(w, err.Error())
				return
			}
			respondInternalError(w, err)
			return
		}
		respondOK(w, fileTreeEntryToNode(s.renderSystemSkillEntry(r.Context(), userID, trustLevelFromCtx(r.Context()), entry)))
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
	entry, err := s.FileTreeService.WriteEntry(r.Context(), userID, path, req.Content, mimeType, models.FileTreeWriteOptions{
		Metadata:         req.Metadata,
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

	respondOK(w, fileTreeEntryToNode(s.renderSystemSkillEntry(r.Context(), userID, trustLevelFromCtx(r.Context()), entry)))
}

func (s *Server) handleTreeDelete(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		respondValidationError(w, "path", "path is required")
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

	respondOK(w, map[string]string{"status": "deleted", "path": hubpath.NormalizePublic(path)})
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
	return &FileNode{
		Path:      publicPath,
		Name:      hubpath.BaseName(publicPath),
		IsDir:     e.IsDirectory,
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
	trustLevel := trustLevelFromCtx(r.Context())
	snapshot, err := s.FileTreeService.Snapshot(r.Context(), userID, path, trustLevel)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	nodes := make([]*FileNode, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
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

	changes, nextCursor, err := s.FileTreeService.Changes(r.Context(), userID, cursor, path, trustLevelFromCtx(r.Context()))
	if err != nil {
		respondInternalError(w, err)
		return
	}

	payload := make([]map[string]interface{}, 0, len(changes))
	for _, change := range changes {
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
		Path:     publicPath,
		Name:     hubpath.BaseName(publicPath),
		IsDir:    true,
		Children: children,
	}, nil
}
