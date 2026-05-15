package feedbacklaunch

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	defaultLaunchURL = "/feedback/start"
	defaultTTL       = 90 * time.Second
	maxTTL           = 300 * time.Second
)

type Config struct {
	Issuer    string
	Audience  string
	ProjectID string
	LaunchURL string
	TTL       time.Duration
	Secret    []byte
	Clock     func() time.Time
	NewJTI    func() string
}

type Reporter struct {
	Subject   string
	Name      string
	Email     string
	AvatarURL string
	Locale    string
}

type Launch struct {
	URL       string
	Token     string
	ExpiresAt time.Time
}

type Client struct {
	issuer    string
	audience  string
	projectID string
	launchURL string
	ttl       time.Duration
	secret    []byte
	clock     func() time.Time
	newJTI    func() string
}

func New(config Config) (*Client, error) {
	issuer := strings.TrimRight(strings.TrimSpace(config.Issuer), "/")
	audience := strings.TrimSpace(config.Audience)
	projectID := strings.TrimSpace(config.ProjectID)
	launchURL := strings.TrimSpace(config.LaunchURL)
	if launchURL == "" {
		launchURL = defaultLaunchURL
	}
	ttl := config.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}

	if issuer == "" {
		return nil, errors.New("feedbacklaunch: issuer is required")
	}
	if audience == "" {
		return nil, errors.New("feedbacklaunch: audience is required")
	}
	if projectID == "" {
		return nil, errors.New("feedbacklaunch: project id is required")
	}
	if ttl < time.Second || ttl > maxTTL {
		return nil, errors.New("feedbacklaunch: ttl must be between 1s and 300s")
	}
	if len(config.Secret) == 0 {
		return nil, errors.New("feedbacklaunch: hs256 secret is required")
	}
	if _, err := url.Parse(launchURL); err != nil {
		return nil, fmt.Errorf("feedbacklaunch: launch url is invalid: %w", err)
	}

	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	newJTI := config.NewJTI
	if newJTI == nil {
		newJTI = randomJTI
	}

	return &Client{
		issuer:    issuer,
		audience:  audience,
		projectID: projectID,
		launchURL: launchURL,
		ttl:       ttl,
		secret:    append([]byte(nil), config.Secret...),
		clock:     clock,
		newJTI:    newJTI,
	}, nil
}

func (c *Client) CreateLaunch(reporter Reporter) (Launch, error) {
	subject := strings.TrimSpace(reporter.Subject)
	if subject == "" {
		return Launch{}, errors.New("feedbacklaunch: reporter subject is required")
	}

	now := c.clock().UTC()
	expiresAt := now.Add(c.ttl)
	claims := map[string]any{
		"iss":        c.issuer,
		"aud":        c.audience,
		"sub":        subject,
		"project_id": c.projectID,
		"iat":        now.Unix(),
		"exp":        expiresAt.Unix(),
		"jti":        strings.TrimSpace(c.newJTI()),
		"name":       strings.TrimSpace(reporter.Name),
		"email":      strings.TrimSpace(reporter.Email),
		"avatar_url": strings.TrimSpace(reporter.AvatarURL),
		"locale":     strings.TrimSpace(reporter.Locale),
	}
	if claims["jti"] == "" {
		return Launch{}, errors.New("feedbacklaunch: jti is required")
	}

	token, err := c.signJWT(claims)
	if err != nil {
		return Launch{}, err
	}
	launchURL, err := appendLaunchToken(c.launchURL, token)
	if err != nil {
		return Launch{}, err
	}
	return Launch{
		URL:       launchURL,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (c *Client) signJWT(claims map[string]any) (string, error) {
	encodedHeader, err := encodeJWTPart(map[string]string{"typ": "JWT", "alg": "HS256"})
	if err != nil {
		return "", err
	}
	encodedClaims, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func encodeJWTPart(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func appendLaunchToken(base string, token string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("feedbacklaunch: launch url is invalid: %w", err)
	}
	query := parsed.Query()
	query.Set("launch_token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func randomJTI() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("fl_%d", time.Now().UnixNano())
	}
	return "fl_" + hex.EncodeToString(data[:])
}
