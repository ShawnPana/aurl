package oauth2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shawnpana/aurl/internal/config"
)

// EnsureValidToken checks if the OAuth2 access token is still valid and refreshes it if needed.
// Returns the access token, or ("", nil) if auth has no OAuth2 config.
func EnsureValidToken(name string, auth *config.AuthConfig) (string, error) {
	if auth.OAuth2 == nil {
		return "", nil
	}

	cfg := auth.OAuth2

	// Return existing token if not expired (with 30s buffer)
	if cfg.AccessToken != "" && cfg.ExpiresAt > 0 {
		if time.Now().Unix()+30 < cfg.ExpiresAt {
			return cfg.AccessToken, nil
		}
	}

	// Refresh the token
	token, expiresAt, err := refreshToken(cfg)
	if err != nil {
		return "", err
	}

	// Update and persist
	cfg.AccessToken = token
	cfg.ExpiresAt = expiresAt
	if err := config.SaveAuth(name, auth); err != nil {
		return "", fmt.Errorf("failed to save refreshed token: %w", err)
	}

	return token, nil
}

func refreshToken(cfg *config.OAuth2Config) (string, int64, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"refresh_token": {cfg.RefreshToken},
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(cfg.TokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read token response: %w", err)
	}

	var result struct {
		AccessToken      string `json:"access_token"`
		ExpiresIn        int64  `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", 0, fmt.Errorf("failed to parse token response: %w", err)
	}

	if result.Error != "" {
		msg := result.Error
		if result.ErrorDescription != "" {
			msg += ": " + result.ErrorDescription
		}
		return "", 0, fmt.Errorf("token refresh error: %s", msg)
	}

	if result.AccessToken == "" {
		return "", 0, fmt.Errorf("token response missing access_token")
	}

	expiresIn := result.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}
	expiresAt := time.Now().Unix() + expiresIn

	return result.AccessToken, expiresAt, nil
}
