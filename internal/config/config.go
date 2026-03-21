package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type OAuth2Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	TokenURL     string `json:"token_url"`
	AccessToken  string `json:"access_token,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}

type AuthConfig struct {
	BaseURLOverride string            `json:"base_url_override,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	QueryParams     map[string]string `json:"query_params,omitempty"`
	OAuth2          *OAuth2Config     `json:"oauth2,omitempty"`
}

type APIEntry struct {
	Name    string
	Title   string
	Version string
	BaseURL string
}

func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aurl")
}

func APIsDir() string {
	return filepath.Join(ConfigDir(), "apis")
}

func AuthDir() string {
	return filepath.Join(ConfigDir(), "auth")
}

func GraphQLDir() string {
	return filepath.Join(ConfigDir(), "graphql")
}

type GraphQLEntry struct {
	Name     string
	Endpoint string
}

func SaveGraphQL(name string, data []byte) error {
	dir := GraphQLDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), data, 0644)
}

func LoadGraphQL(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(GraphQLDir(), name+".json"))
}

func DeleteGraphQL(name string) error {
	authPath := filepath.Join(AuthDir(), name+".json")
	os.Remove(authPath)
	return os.Remove(filepath.Join(GraphQLDir(), name+".json"))
}

func ListGraphQL() ([]GraphQLEntry, error) {
	dir := GraphQLDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var gqls []GraphQLEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		gql := GraphQLEntry{Name: name}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err == nil {
			var raw map[string]any
			if json.Unmarshal(data, &raw) == nil {
				if ep, ok := raw["endpoint"].(string); ok {
					gql.Endpoint = ep
				}
			}
		}

		gqls = append(gqls, gql)
	}
	return gqls, nil
}

func SaveSpec(name string, data []byte) error {
	dir := APIsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), data, 0644)
}

func LoadSpec(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(APIsDir(), name+".json"))
}

func DeleteAPI(name string) error {
	specPath := filepath.Join(APIsDir(), name+".json")
	authPath := filepath.Join(AuthDir(), name+".json")
	os.Remove(authPath) // ignore error, may not exist
	return os.Remove(specPath)
}

func ListAPIs() ([]APIEntry, error) {
	dir := APIsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var apis []APIEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		api := APIEntry{Name: name}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err == nil {
			var raw map[string]any
			if json.Unmarshal(data, &raw) == nil {
				if info, ok := raw["info"].(map[string]any); ok {
					if t, ok := info["title"].(string); ok {
						api.Title = t
					}
					if v, ok := info["version"].(string); ok {
						api.Version = v
					}
				}
				if servers, ok := raw["servers"].([]any); ok && len(servers) > 0 {
					if s, ok := servers[0].(map[string]any); ok {
						if u, ok := s["url"].(string); ok {
							api.BaseURL = u
						}
					}
				}
			}
		}

		apis = append(apis, api)
	}
	return apis, nil
}

func SaveAuth(name string, auth *AuthConfig) error {
	dir := AuthDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), data, 0600)
}

func LoadAuth(name string) (*AuthConfig, error) {
	data, err := os.ReadFile(filepath.Join(AuthDir(), name+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &AuthConfig{}, nil
		}
		return nil, err
	}
	var auth AuthConfig
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("invalid auth config: %w", err)
	}
	return &auth, nil
}
