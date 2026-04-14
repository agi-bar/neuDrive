package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/agi-bar/neudrive/internal/models"
	"github.com/agi-bar/neudrive/internal/services"
)

func explicitRequestSource(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, key := range []string{"source_platform", "source"} {
		if value := services.NormalizeSource(r.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	for _, key := range []string{"X-NeuDrive-Platform", "X-NeuDrive-Source"} {
		if value := services.NormalizeSource(r.Header.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func inferAuthContextSource(ctx context.Context) string {
	if source := services.SourceFromContext(ctx); source != "" {
		return source
	}
	if conn := connectionFromCtx(ctx); conn != nil {
		if source := services.NormalizeSource(conn.Platform); source != "" {
			return source
		}
	}
	if token := scopedTokenFromCtx(ctx); token != nil {
		if source := services.InferSourceFromTokenName(token.Name); source != "" {
			return source
		}
	}
	return ""
}

func (s *Server) withAuthenticatedSource(ctx context.Context, conn *models.Connection, token *models.ScopedToken) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if source := inferAuthContextSource(ctx); source != "" {
		return services.ContextWithSource(ctx, source)
	}
	if conn != nil {
		if source := services.NormalizeSource(conn.Platform); source != "" {
			return services.ContextWithSource(ctx, source)
		}
	}
	if token != nil {
		if source := services.InferSourceFromTokenName(token.Name); source != "" {
			return services.ContextWithSource(ctx, source)
		}
	}
	return ctx
}

func (s *Server) inferredRequestSource(r *http.Request, body []byte, fallback string) string {
	if source := explicitRequestSource(r); source != "" {
		return source
	}
	if r != nil {
		if source := inferAuthContextSource(r.Context()); source != "" {
			return source
		}
		if sessionID := strings.TrimSpace(r.Header.Get("Mcp-Session-Id")); sessionID != "" {
			if source, ok := s.mcpSessionSource(sessionID); ok {
				return source
			}
		}
	}
	if r != nil {
		if inferred := services.NormalizeSource(inferCaptureSource(r, body)); inferred != "" && inferred != "unknown" {
			return inferred
		}
	}
	return services.NormalizeSource(fallback)
}

func (s *Server) requestSourceContext(r *http.Request, fallback string) context.Context {
	if r == nil {
		return services.ContextWithSource(context.Background(), fallback)
	}
	return services.ContextWithSource(r.Context(), s.inferredRequestSource(r, nil, fallback))
}

func applyExplicitSourceHints(ctx context.Context, metadata map[string]interface{}, source, sourcePlatform string) (context.Context, map[string]interface{}) {
	merged := services.WithSourceMetadata(metadata, source)
	merged = services.WithSourcePlatformMetadata(merged, sourcePlatform)

	if platform := services.NormalizeSource(sourcePlatform); platform != "" {
		return services.ContextWithSource(ctx, platform), merged
	}
	if explicit := services.NormalizeSource(source); explicit != "" {
		return services.ContextWithSource(ctx, explicit), merged
	}
	return ctx, merged
}

func (s *Server) rememberMCPSessionSource(sessionID, source string) {
	if s == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	source = services.NormalizeSource(source)
	if sessionID == "" || source == "" {
		return
	}
	s.mcpSessionSources.Store(sessionID, source)
}

func (s *Server) mcpSessionSource(sessionID string) (string, bool) {
	if s == nil {
		return "", false
	}
	value, ok := s.mcpSessionSources.Load(strings.TrimSpace(sessionID))
	if !ok {
		return "", false
	}
	source, _ := value.(string)
	source = services.NormalizeSource(source)
	return source, source != ""
}
