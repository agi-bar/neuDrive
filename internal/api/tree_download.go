package api

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/agi-bar/neudrive/internal/hubpath"
	"github.com/agi-bar/neudrive/internal/models"
	"github.com/google/uuid"
)

func (s *Server) handleTreeDownloadZip(w http.ResponseWriter, r *http.Request) {
	if s.FileTreeService == nil {
		respondNotConfigured(w, "file tree service")
		return
	}
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}

	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" {
		respondValidationError(w, "path", "query parameter 'path' is required")
		return
	}
	if isHiddenPublicFeaturePath(rawPath) {
		respondNotFound(w, "file")
		return
	}

	trustLevel := trustLevelFromCtx(r.Context())
	publicPath, isDir, err := s.resolveTreeDownloadPath(r.Context(), userID, trustLevel, rawPath)
	if err != nil {
		respondNotFound(w, "file")
		return
	}

	filename := treeDownloadArchiveName(publicPath)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	zw := zip.NewWriter(w)
	if err := s.writeTreeDownloadArchive(r.Context(), zw, userID, trustLevel, publicPath, isDir); err != nil {
		http.Error(w, "download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := zw.Close(); err != nil {
		http.Error(w, "download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) resolveTreeDownloadPath(ctx context.Context, userID uuid.UUID, trustLevel int, rawPath string) (string, bool, error) {
	publicPath := hubpath.NormalizePublic(rawPath)
	storagePath := hubpath.NormalizeStorage(rawPath)
	if publicPath == "/" {
		return publicPath, true, nil
	}

	entry, err := s.FileTreeService.Read(ctx, userID, storagePath, trustLevel)
	if err == nil {
		return hubpath.StorageToPublic(entry.Path), entry.IsDirectory, nil
	}

	if children, listErr := s.FileTreeService.List(ctx, userID, storagePath, trustLevel); listErr == nil && (storagePath == "/" || len(children) > 0) {
		return hubpath.StorageToPublic(storagePath), true, nil
	}

	return "", false, err
}

func (s *Server) writeTreeDownloadArchive(ctx context.Context, zw *zip.Writer, userID uuid.UUID, trustLevel int, publicPath string, isDir bool) error {
	zipRoot := treeDownloadZipRoot(publicPath)
	storagePath := hubpath.NormalizeStorage(publicPath)
	if !isDir {
		entry, err := s.FileTreeService.Read(ctx, userID, storagePath, trustLevel)
		if err != nil {
			return err
		}
		return s.writeTreeDownloadFile(ctx, zw, userID, trustLevel, entry, zipRoot)
	}

	return s.writeTreeDownloadDirectory(ctx, zw, userID, trustLevel, storagePath, zipRoot)
}

func (s *Server) writeTreeDownloadDirectory(ctx context.Context, zw *zip.Writer, userID uuid.UUID, trustLevel int, storagePath string, zipPath string) error {
	if zipPath != "" {
		header := &zip.FileHeader{
			Name: strings.TrimSuffix(zipPath, "/") + "/",
		}
		header.SetMode(0o755 | os.ModeDir)
		if _, err := zw.CreateHeader(header); err != nil {
			return err
		}
	}

	children, err := s.FileTreeService.List(ctx, userID, storagePath, trustLevel)
	if err != nil {
		return err
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Path < children[j].Path
	})

	for _, child := range children {
		childPublicPath := hubpath.StorageToPublic(child.Path)
		childZipPath := pathpkg.Join(zipPath, hubpath.BaseName(childPublicPath))
		if child.IsDirectory {
			if err := s.writeTreeDownloadDirectory(ctx, zw, userID, trustLevel, child.Path, childZipPath); err != nil {
				return err
			}
			continue
		}
		if err := s.writeTreeDownloadFile(ctx, zw, userID, trustLevel, &child, childZipPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) writeTreeDownloadFile(ctx context.Context, zw *zip.Writer, userID uuid.UUID, trustLevel int, entry *models.FileTreeEntry, zipPath string) error {
	if entry == nil {
		return fmt.Errorf("missing file entry")
	}

	data := []byte(entry.Content)
	if isBinaryMetadata(entry.Metadata) {
		binary, _, err := s.FileTreeService.ReadBinary(ctx, userID, entry.Path, trustLevel)
		if err != nil {
			return err
		}
		data = binary
	}

	header := &zip.FileHeader{
		Name:   zipPath,
		Method: zip.Deflate,
	}
	header.Modified = entry.UpdatedAt
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, bytes.NewReader(data))
	return err
}

func treeDownloadZipRoot(publicPath string) string {
	if publicPath == "/" {
		return "root"
	}
	return hubpath.BaseName(publicPath)
}

func treeDownloadArchiveName(publicPath string) string {
	return treeDownloadZipRoot(publicPath) + ".zip"
}
