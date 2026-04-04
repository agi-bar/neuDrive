package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const authTestSecret = "auth-test-secret"

func TestValidateTokenRejectsRefreshTokens(t *testing.T) {
	claims := Claims{
		UserID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Slug:     "refresh-user",
		TokenUse: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString([]byte(authTestSecret))
	if err != nil {
		t.Fatalf("sign refresh token: %v", err)
	}

	if _, err := ValidateToken(raw, authTestSecret); err == nil {
		t.Fatal("expected refresh token to be rejected as bearer auth")
	}
}
