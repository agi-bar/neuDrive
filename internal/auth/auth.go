package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("auth: invalid or expired token")
	ErrNoToken      = errors.New("auth: no token provided")
)

type Claims struct {
	UserID   uuid.UUID `json:"user_id"`
	Slug     string    `json:"slug"`
	TokenUse string    `json:"token_use,omitempty"`
	jwt.RegisteredClaims
}

type GitHubUser struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GenerateToken creates a JWT token for a user
func GenerateToken(userID uuid.UUID, slug string, secret string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Slug:     slug,
		TokenUse: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)), // 30 days
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken validates a JWT token and returns claims
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.TokenUse != "" && claims.TokenUse != "access" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ExchangeGitHubCode exchanges a GitHub OAuth code for user info
func ExchangeGitHubCode(ctx context.Context, clientID, clientSecret, code string) (*GitHubUser, error) {
	// Step 1: Exchange code for access token
	reqBody := fmt.Sprintf(`{"client_id":"%s","client_secret":"%s","code":"%s"}`, clientID, clientSecret, code)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("auth: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("auth: failed to decode token response: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("auth: github error: %s", tokenResp.Error)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("auth: no access token in response")
	}

	// Step 2: Get user info
	req, err = http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("auth: failed to create user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: github API returned status %d", resp.StatusCode)
	}

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("auth: failed to decode user info: %w", err)
	}
	return &user, nil
}

// ExtractTokenFromHeader extracts bearer token from Authorization header
func ExtractTokenFromHeader(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", ErrNoToken
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", ErrNoToken
	}
	return parts[1], nil
}

// ExtractAPIKey extracts API key from X-API-Key header (for Agent connections)
func ExtractAPIKey(r *http.Request) string {
	return r.Header.Get("X-API-Key")
}
