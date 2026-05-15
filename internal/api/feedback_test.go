package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agi-bar/neudrive/internal/config"
)

func TestHandleFeedbackLaunchCreatesTriageLaunchURL(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:                testJWTSecret,
		VaultMasterKey:           strings.Repeat("0", 64),
		CORSOrigins:              []string{"http://localhost:3000"},
		RateLimit:                100,
		MaxBodySize:              10 * 1024 * 1024,
		PublicBaseURL:            "https://www.neudrive.ai",
		FeedbackEnabled:          true,
		FeedbackLaunchURL:        "https://triage.example.com/feedback/start?project=neudrive",
		FeedbackLaunchSecret:     "feedback-secret",
		FeedbackLaunchIssuer:     "https://www.neudrive.ai",
		FeedbackLaunchAudience:   "triage.feedback",
		FeedbackLaunchProjectID:  "neudrive",
		FeedbackLaunchTTLSeconds: 90,
	}
	ts, store, adminToken, _, _ := newTestHTTPServerWithConfig(t, cfg)
	user, err := store.EnsureOwner(t.Context())
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/feedback/launch", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("feedback launch request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var envelope testEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("response ok = false: %s", string(envelope.Data))
	}
	var launch feedbackLaunchResponse
	if err := json.Unmarshal(envelope.Data, &launch); err != nil {
		t.Fatalf("decode launch: %v", err)
	}
	launchURL, err := url.Parse(launch.LaunchURL)
	if err != nil {
		t.Fatalf("parse launch url: %v", err)
	}
	if launchURL.Scheme != "https" || launchURL.Host != "triage.example.com" || launchURL.Path != "/feedback/start" {
		t.Fatalf("launch URL = %q", launch.LaunchURL)
	}
	if launchURL.Query().Get("project") != "neudrive" {
		t.Fatalf("project query = %q", launchURL.Query().Get("project"))
	}
	token := launchURL.Query().Get("launch_token")
	if token == "" {
		t.Fatalf("launch token missing from URL: %q", launch.LaunchURL)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}

	var header map[string]string
	if err := decodeFeedbackJWTPart(parts[0], &header); err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if header["typ"] != "JWT" || header["alg"] != "HS256" {
		t.Fatalf("header = %+v, want JWT HS256", header)
	}

	var decodedClaims map[string]any
	if err := decodeFeedbackJWTPart(parts[1], &decodedClaims); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if decodedClaims["iss"] != "https://www.neudrive.ai" || decodedClaims["aud"] != "triage.feedback" || decodedClaims["project_id"] != "neudrive" {
		t.Fatalf("claims = %+v", decodedClaims)
	}
	if decodedClaims["sub"] != user.ID.String() || decodedClaims["name"] != "Local Owner" || decodedClaims["locale"] != "en" {
		t.Fatalf("identity claims = %+v", decodedClaims)
	}
}

func TestRequestOriginUsesForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://internal.local/api/feedback/launch", nil)
	req.Host = "internal.local"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "www.neudrive.ai")
	if got := requestOrigin(req); got != "https://www.neudrive.ai" {
		t.Fatalf("requestOrigin = %q, want forwarded public origin", got)
	}
}

func decodeFeedbackJWTPart(part string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
