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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
		return
	}

	if githubConfig == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "github oauth not configured"})
		return
	}

	tokenResp, err := auth.ExchangeGithubCode(githubConfig, req.Code)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to exchange code with github"})
		return
	}

	ghUser, err := auth.FetchGithubUser(tokenResp.AccessToken)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch github user"})
		return
	}

	// TODO: upsert user in database

	jwtToken, err := auth.GenerateJWT(githubConfig.JWTSecret, ghUser)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": jwtToken,
		"user": map[string]interface{}{
			"id":       ghUser.ID,
			"username": ghUser.Login,
			"email":    ghUser.Email,
		},
	})
}
