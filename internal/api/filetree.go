package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agi-bar/agenthub/internal/models"
	"github.com/go-chi/chi/v5"
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
	path := chi.URLParam(r, "*")
	if path == "" {
		path = "/"
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())

	// If path looks like a directory (ends with / or is root), list children
	if strings.HasSuffix(path, "/") || path == "/" {
		entries, err := s.FileTreeService.List(r.Context(), userID, path, trustLevel)
		if err != nil {
			respondInternalError(w, err)
			return
		}

		children := make([]*FileNode, 0, len(entries))
		for _, e := range entries {
			children = append(children, fileTreeEntryToNode(&e))
		}

		node := &FileNode{
			Path:     path,
			Name:     path,
			IsDir:    true,
			Children: children,
		}
		respondOK(w, node)
		return
	}

	// Otherwise, read a specific file
	entry, err := s.FileTreeService.Read(r.Context(), userID, path, trustLevel)
	if err != nil {
		respondNotFound(w, "file")
		return
	}

	respondOK(w, fileTreeEntryToNode(entry))
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
		respondOK(w, &FileNode{Path: path, Name: path, IsDir: true})
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

	respondOK(w, map[string]string{"status": "deleted", "path": path})
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

	entries, err := s.FileTreeService.Search(r.Context(), userID, query, trustLevel)
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
	return &FileNode{
		Path:      e.Path,
		Name:      e.Path,
		IsDir:     e.IsDirectory,
		Content:   e.Content,
		MimeType:  e.ContentType,
		Size:      int64(len(e.Content)),
		CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: e.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
