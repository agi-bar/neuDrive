package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/cors"
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

func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-API-Key"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		log.Printf("%s %s %d %s %s",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			time.Since(start).Round(time.Millisecond),
			r.RemoteAddr,
		)
	})
}

func TrustLevelMiddleware(requiredLevel int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trustLevel := trustLevelFromCtx(r.Context())
			if trustLevel < requiredLevel {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "insufficient trust level",
				})
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
