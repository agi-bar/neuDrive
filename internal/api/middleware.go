package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/agi-bar/agenthub/internal/logger"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
)

const (
	maxSkillsArchiveRequestBytes = 50 << 20
	maxMCPArchiveRequestBytes    = 80 << 20
	maxSyncPartRequestBytes      = 8 << 20
)

// GetUser returns an AuthenticatedUser from the context for backward
// compatibility with existing package-level handlers (filetree, vault, etc.).
// It reads from the context keys set by Server.authMiddleware.
func GetUser(ctx interface{ Value(any) any }) *AuthenticatedUser {
	userID, ok := ctx.Value(ctxKeyUserID).(interface{ String() string })
	if !ok {
		return nil
	}
	slug, _ := ctx.Value(ctxKeyUserSlug).(string)
	return &AuthenticatedUser{
		UserID:   userID.String(),
		Username: slug,
	}
}

// AuthenticatedUser is a lightweight struct used by the existing package-level
// handlers to read the authenticated user identity.
type AuthenticatedUser struct {
	UserID   string
	Username string
	Email    string
}

// CORSMiddleware configures CORS with the given allowed origins. Credentials
// are allowed, and rate-limit headers are exposed to the browser.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-API-Key"},
		ExposedHeaders:   []string{"Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

// SecurityHeadersMiddleware adds standard security headers to every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// CSP for API-only paths: deny everything.
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/agent/") {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

// PanicRecoveryMiddleware catches panics, logs a stack trace, and returns a
// 500 Internal Server Error so the server stays up.
func PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := string(debug.Stack())
				slog.Error("panic recovered",
					"error", fmt.Sprintf("%v", rec),
					"method", r.Method,
					"path", r.URL.Path,
					"stack", stack,
				)
				respondError(w, http.StatusInternalServerError, ErrCodeInternal, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// MaxBodySizeMiddleware limits the size of request bodies to prevent abuse.
func MaxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, bodySizeLimitForPath(r.URL.Path, maxBytes))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bodySizeLimitForPath(path string, fallback int64) int64 {
	limit := fallback
	switch {
	case path == "/mcp":
		if limit < maxMCPArchiveRequestBytes {
			limit = maxMCPArchiveRequestBytes
		}
	case path == "/api/import/skills", path == "/agent/import/skills", path == "/agent/import/preview", path == "/agent/import/bundle":
		if limit < maxSkillsArchiveRequestBytes {
			limit = maxSkillsArchiveRequestBytes
		}
	case strings.HasPrefix(path, "/agent/import/session/") && strings.Contains(path, "/parts/"):
		if limit < maxSyncPartRequestBytes {
			limit = maxSyncPartRequestBytes
		}
	}
	return limit
}

// RequestIDMiddleware generates a UUID for each incoming request, stores it in
// the context (accessible via logger.RequestIDFromContext), and sets the
// X-Request-ID response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		ctx := logger.WithRequestID(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(ww, r)

		logger.FromContext(r.Context()).Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}

func TrustLevelMiddleware(requiredLevel int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trustLevel := trustLevelFromCtx(r.Context())
			if trustLevel < requiredLevel {
				respondForbidden(w, "insufficient trust level")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
