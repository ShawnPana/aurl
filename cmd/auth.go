package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/shawnpana/aurl/internal/config"
	"github.com/shawnpana/aurl/internal/openapi"
	"github.com/spf13/cobra"
)

var authHeaders []string
var authOAuth2 bool
var authClientID string
var authClientSecret string
var authRefreshToken string
var authTokenURL string

var authCmd = &cobra.Command{
	Use:   "auth [name]",
	Short: "Configure auth for a registered API",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuth,
}

func init() {
	authCmd.Flags().StringArrayVar(&authHeaders, "header", nil, "Auth header (e.g., \"Authorization: Bearer token\")")
	authCmd.Flags().BoolVar(&authOAuth2, "oauth2", false, "Configure OAuth2 refresh token auth")
	authCmd.Flags().StringVar(&authClientID, "client-id", "", "OAuth2 client ID")
	authCmd.Flags().StringVar(&authClientSecret, "client-secret", "", "OAuth2 client secret")
	authCmd.Flags().StringVar(&authRefreshToken, "refresh-token", "", "OAuth2 refresh token")
	authCmd.Flags().StringVar(&authTokenURL, "token-url", "", "OAuth2 token endpoint URL")
}

func runAuth(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Check if this is a registered API or GraphQL endpoint
	data, err := config.LoadSpec(name)
	isGraphQL := false
	if err != nil {
		// Try GraphQL
		_, err = config.LoadGraphQL(name)
		if err != nil {
			return fmt.Errorf("%q not registered. Run 'aurl list' to see registered commands.", name)
		}
		isGraphQL = true
	}

	// Load existing auth to preserve base_url_override
	existingAuth, _ := config.LoadAuth(name)
	auth := &config.AuthConfig{
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}
	if existingAuth != nil {
		auth.BaseURLOverride = existingAuth.BaseURLOverride
	}

	// Conflict check: --oauth2 and Authorization header
	if authOAuth2 {
		for _, h := range authHeaders {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "Authorization") {
				return fmt.Errorf("cannot use --oauth2 with an Authorization header")
			}
		}
		if authClientID == "" || authClientSecret == "" || authRefreshToken == "" || authTokenURL == "" {
			return fmt.Errorf("--oauth2 requires --client-id, --client-secret, --refresh-token, and --token-url")
		}
		auth.OAuth2 = &config.OAuth2Config{
			ClientID:     authClientID,
			ClientSecret: authClientSecret,
			RefreshToken: authRefreshToken,
			TokenURL:     authTokenURL,
		}
	}

	// Add manual headers from flags
	for _, h := range authHeaders {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			auth.Headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// If no manual headers and no --oauth2, run interactive detection (OpenAPI only)
	if len(authHeaders) == 0 && !authOAuth2 {
		if isGraphQL {
			fmt.Println("Use --header to set auth for GraphQL APIs.")
			fmt.Printf("  Example: aurl auth %s --header \"Authorization: Bearer your-token\"\n", name)
			return nil
		}

		spec, err := openapi.Parse(data)
		if err != nil {
			return fmt.Errorf("failed to parse spec: %w", err)
		}

		schemes := openapi.DetectAuth(spec)
		if len(schemes) > 0 {
			fmt.Println("Auth detected:")
			for i, scheme := range schemes {
				fmt.Printf("  [%d] %s (%s)\n", i+1, scheme.Name, scheme.Description)
			}
			fmt.Println()

			reader := bufio.NewReader(os.Stdin)
			for _, scheme := range schemes {
				promptAndStoreAuth(reader, scheme, auth)
			}
		} else {
			fmt.Println("No securitySchemes found in spec. Use --header to set auth manually.")
			fmt.Printf("  Example: aurl auth %s --header \"Authorization: Bearer your-token\"\n", name)
			return nil
		}
	}

	if err := config.SaveAuth(name, auth); err != nil {
		return fmt.Errorf("failed to save auth: %w", err)
	}

	fmt.Println("Auth updated.")
	return nil
}
