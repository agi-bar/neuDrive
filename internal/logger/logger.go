package logger

import (
	"context"
	"log/slog"
	"os"
)

// ctxKey is an unexported type for context keys defined in this package.
type ctxKey string

const requestIDKey ctxKey = "request_id"

var defaultLogger *slog.Logger

// Init initialises the default slog logger. level should be one of
// "debug", "info", "warn", "error". format should be "json" or "text".
func Init(level string, format string) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Default returns the package-level logger that was configured by Init.
func Default() *slog.Logger { return defaultLogger }

// WithRequestID returns a new context that carries the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext extracts the request ID from the context, if present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// FromContext returns a logger enriched with the request ID stored in ctx.
func FromContext(ctx context.Context) *slog.Logger {
	l := slog.Default()
	if id := RequestIDFromContext(ctx); id != "" {
		l = l.With("request_id", id)
	}
	return l
}
