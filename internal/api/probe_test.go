package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agi-bar/neudrive/internal/config"
)

func TestTestPostEndpointRespondsOK(t *testing.T) {
	s := NewServerWithDeps(ServerDeps{
		Config: &config.Config{
			CORSOrigins: []string{"http://localhost:3000"},
			RateLimit:   100,
			MaxBodySize: 10 << 20,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/test/post", nil)
	rec := httptest.NewRecorder()

	s.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var body APISuccess
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok response, got %+v", body)
	}
}
