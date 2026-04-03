package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agi-bar/agenthub/internal/hubpath"
	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type FileNode struct {
	Path      string      `json:"path"`
	Name      string      `json:"name"`
	IsDir     bool        `json:"is_dir"`
	Content   string      `json:"content,omitempty"`
	MimeType  string      `json:"mime_type,omitempty"`
	Size      int64       `json:"size,omitempty"`
	Children  []*FileNode `json:"children,omitempty"`
	CreatedAt string      `json:"created_at,omitempty"`
	UpdatedAt string      `json:"updated_at,omitempty"`
}

type WriteFileRequest struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type,omitempty"`
	IsDir    bool   `json:"is_dir"`
}

type SearchRequest struct {
	Query string `json:"q"`
	Path  string `json:"path,omitempty"`
}

func (s *Server) handleTreeRead(w http.ResponseWriter, r *http.Request) {
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
		if err := s.FileTreeService.EnsureDirectory(r.Context(), userID, path); err != nil {
			respondInternalError(w, err)
			return
		}
		publicPath := hubpath.NormalizePublic(path)
		if !strings.HasSuffix(publicPath, "/") && publicPath != "/" {
			publicPath += "/"
		}
		respondOK(w, &FileNode{Path: publicPath, Name: hubpath.BaseName(publicPath), IsDir: true})
		return
	}

	mimeType := req.MimeType
	if mimeType == "" {
		mimeType = "text/plain"
	}

	entry, err := s.FileTreeService.Write(r.Context(), userID, path, req.Content, mimeType, models.TrustLevelFull)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, fileTreeEntryToNode(entry))
}

func (s *Server) handleTreeDelete(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		respondValidationError(w, "path", "path is required")
		return
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	if err := s.FileTreeService.Delete(r.Context(), userID, path); err != nil {
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

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())

	entries, err := s.FileTreeService.Search(r.Context(), userID, query, trustLevel, "")
	if err != nil {
		respondInternalError(w, err)
		return
	}

	results := make([]*FileNode, 0, len(entries))
	for _, e := range entries {
		results = append(results, fileTreeEntryToNode(&e))
	}

	respondOK(w, map[string]interface{}{
		"query":   query,
		"results": results,
	})
}

func fileTreeEntryToNode(e *models.FileTreeEntry) *FileNode {
	publicPath := hubpath.StorageToPublic(e.Path)
	return &FileNode{
		Path:      publicPath,
		Name:      hubpath.BaseName(publicPath),
		IsDir:     e.IsDirectory,
		Content:   e.Content,
		MimeType:  e.ContentType,
		Size:      int64(len(e.Content)),
		CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: e.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
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
		return fileTreeEntryToNode(entry), nil
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
		children = append(children, fileTreeEntryToNode(&e))
	}

	return &FileNode{
		Path:     publicPath,
		Name:     hubpath.BaseName(publicPath),
		IsDir:    true,
		Children: children,
	}, nil
}
