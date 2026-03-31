package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Slug        string    `json:"slug" db:"slug"`
	DisplayName string    `json:"display_name" db:"display_name"`
	Email       string    `json:"email,omitempty" db:"email"`
	AvatarURL   string    `json:"avatar_url,omitempty" db:"avatar_url"`
	Timezone    string    `json:"timezone" db:"timezone"`
	Language    string    `json:"language" db:"language"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type AuthBinding struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	UserID       uuid.UUID              `json:"user_id" db:"user_id"`
	Provider     string                 `json:"provider" db:"provider"`
	ProviderID   string                 `json:"provider_id" db:"provider_id"`
	ProviderData map[string]interface{} `json:"provider_data" db:"provider_data"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

type Credentials struct {
	ID                uuid.UUID  `json:"id"`
	UserID            uuid.UUID  `json:"user_id"`
	Email             string     `json:"email"`
	PasswordHash      string     `json:"-"` // never expose
	EmailVerified     bool       `json:"email_verified"`
	VerificationToken string     `json:"-"`
	ResetToken        string     `json:"-"`
	ResetTokenExpires *time.Time `json:"-"`
	LastLoginAt       *time.Time `json:"last_login_at"`
	LoginCount        int        `json:"login_count"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Session struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	RefreshTokenHash string    `json:"-"`
	UserAgent        string    `json:"user_agent"`
	IPAddress        string    `json:"ip_address"`
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// Auth API request/response types

type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	User         User   `json:"user"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
