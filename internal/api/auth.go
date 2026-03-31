package api

import (
	"encoding/json"
	"net/http"

	"github.com/agi-bar/agenthub/internal/auth"
)

var githubConfig *auth.GithubOAuthConfig

func SetGithubConfig(cfg *auth.GithubOAuthConfig) {
	githubConfig = cfg
}

type GithubCallbackRequest struct {
	Code string `json:"code"`
}

func HandleGithubCallback(w http.ResponseWriter, r *http.Request) {
	var req GithubCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, ErrCodeBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		respondValidationError(w, "code", "code is required")
		return
	}

	if githubConfig == nil {
		respondError(w, http.StatusInternalServerError, ErrCodeInternal, "github oauth not configured")
		return
	}

	tokenResp, err := auth.ExchangeGithubCode(githubConfig, req.Code)
	if err != nil {
		respondError(w, http.StatusBadGateway, ErrCodeInternal, "failed to exchange code with github")
		return
	}

	ghUser, err := auth.FetchGithubUser(tokenResp.AccessToken)
	if err != nil {
		respondError(w, http.StatusBadGateway, ErrCodeInternal, "failed to fetch github user")
		return
	}

	// TODO: upsert user in database

	jwtToken, err := auth.GenerateJWT(githubConfig.JWTSecret, ghUser)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondOK(w, map[string]interface{}{
		"token": jwtToken,
		"user": map[string]interface{}{
			"id":       ghUser.ID,
			"username": ghUser.Login,
			"email":    ghUser.Email,
		},
	})
}
