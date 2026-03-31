package api

import (
	"encoding/json"
	"net/http"

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

func HandleTreeRead(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		path = "/"
	}

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: query database for file tree entries belonging to user
	node := &FileNode{
		Path:  path,
		Name:  path,
		IsDir: true,
		Children: []*FileNode{
			{Path: path + "/README.md", Name: "README.md", IsDir: false, MimeType: "text/markdown", Size: 0},
		},
	}

	writeJSON(w, http.StatusOK, node)
}

func HandleTreeWrite(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req WriteFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// TODO: insert or update file tree entry in database
	node := &FileNode{
		Path:     path,
		Name:     path,
		IsDir:    req.IsDir,
		Content:  req.Content,
		MimeType: req.MimeType,
		Size:     int64(len(req.Content)),
	}

	writeJSON(w, http.StatusOK, node)
}

func HandleTreeDelete(w http.ResponseWriter, r *http.Request) {
	path := chi.URLParam(r, "*")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: delete file tree entry from database
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "path": path})
}

func HandleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
		return
	}

	user := GetUser(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	// TODO: full-text search in database
	results := []FileNode{}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   query,
		"results": results,
	})
}
