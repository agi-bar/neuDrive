package api

import (
	"net/http"
	"strings"
	"time"

	feedbacklaunch "github.com/agi-bar/triage/sdk/go/feedbacklaunch"
)

type feedbackLaunchResponse struct {
	LaunchURL string `json:"launch_url"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Server) handleFeedbackLaunch(w http.ResponseWriter, r *http.Request) {
	if s.Config == nil || !s.Config.FeedbackEnabled {
		respondError(w, http.StatusNotFound, ErrCodeNotFound, "feedback is not configured")
		return
	}
	if strings.TrimSpace(s.Config.FeedbackLaunchSecret) == "" {
		respondError(w, http.StatusConflict, ErrCodeConflict, "feedback launch secret is not configured")
		return
	}
	if s.AuthService == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "auth service not configured")
		return
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		respondUnauthorized(w)
		return
	}
	user, err := s.AuthService.GetProfile(r.Context(), userID)
	if err != nil {
		respondNotFound(w, "user")
		return
	}

	client, err := s.feedbackLaunchClient(r)
	if err != nil {
		respondError(w, http.StatusConflict, ErrCodeConflict, "feedback launch configuration is invalid")
		return
	}

	launch, err := client.CreateLaunch(feedbacklaunch.Reporter{
		Subject:   user.ID.String(),
		Name:      nonEmpty(user.DisplayName, user.Slug),
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
		Locale:    user.Language,
	})
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondOK(w, feedbackLaunchResponse{
		LaunchURL: launch.URL,
		ExpiresAt: launch.ExpiresAt.Format(time.RFC3339),
	})
}

func (s *Server) feedbackLaunchClient(r *http.Request) (*feedbacklaunch.Client, error) {
	secret := strings.TrimSpace(s.Config.FeedbackLaunchSecret)
	signer, err := feedbacklaunch.NewHS256Signer([]byte(secret))
	if err != nil {
		return nil, err
	}
	issuer := strings.TrimSpace(s.Config.FeedbackLaunchIssuer)
	if issuer == "" {
		issuer = requestOrigin(r)
	}
	return feedbacklaunch.New(feedbacklaunch.Config{
		Issuer:    issuer,
		Audience:  nonEmpty(s.Config.FeedbackLaunchAudience, "triage.feedback"),
		ProjectID: nonEmpty(s.Config.FeedbackLaunchProjectID, "neudrive"),
		LaunchURL: s.Config.FeedbackLaunchURL,
		TTL:       time.Duration(s.Config.FeedbackLaunchTTLSeconds) * time.Second,
		Signer:    signer,
	})
}

func requestOrigin(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if r.TLS != nil {
			proto = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
