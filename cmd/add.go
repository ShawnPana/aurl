package cmd

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/shawnpana/aurl/internal/config"
	"github.com/shawnpana/aurl/internal/graphql"
	"github.com/shawnpana/aurl/internal/openapi"
	"github.com/shawnpana/aurl/internal/urlutil"
	"github.com/spf13/cobra"
)

var addHeaders []string
var addBaseURL string
var addGraphQL bool

var addCmd = &cobra.Command{
	Use:   "add [name] [spec or endpoint]",
	Short: "Register an API as a CLI command",
	Long:  "Register an OpenAPI spec or GraphQL endpoint as a named CLI command.\n\nExamples:\n  aurl add petstore https://petstore.swagger.io/openapi.json\n  aurl add --graphql countries https://countries.trevorblades.com/graphql",
	Args:  cobra.ExactArgs(2),
	RunE:  runAdd,
}

func init() {
	addCmd.Flags().StringArrayVar(&addHeaders, "header", nil, "Auth header (e.g., \"Authorization: Bearer token\")")
	addCmd.Flags().StringVar(&addBaseURL, "base-url", "", "Override the base URL from the spec")
	addCmd.Flags().BoolVar(&addGraphQL, "graphql", false, "Register a GraphQL endpoint instead of an OpenAPI spec")
}

func runAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	source := args[1]

	if isReservedName(name) {
		return fmt.Errorf("%q is a reserved command name", name)
	}

	if addGraphQL {
		return runAddGraphQL(name, source)
	}

	// Fetch or read the spec
	var data []byte
	var err error

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if err := urlutil.ValidatePublicURL(source); err != nil {
			return fmt.Errorf("refusing to fetch spec: %w", err)
		}
		data, err = fetchSpec(source)
	} else {
		data, err = os.ReadFile(source)
	}
	if err != nil {
		return fmt.Errorf("failed to read spec: %w", err)
	}

	// Validate it's valid JSON
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate it looks like an OpenAPI spec
	if _, ok := raw["openapi"]; !ok {
		if _, ok := raw["swagger"]; !ok {
			return fmt.Errorf("file does not appear to be an OpenAPI spec (missing 'openapi' or 'swagger' field)")
		}
	}

	// Save the spec
	if err := config.SaveSpec(name, data); err != nil {
		return fmt.Errorf("failed to save spec: %w", err)
	}

	// Parse for display and auth detection
	spec, err := openapi.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: spec saved but could not parse: %v\n", err)
		return nil
	}

	fmt.Printf("Registered %q", name)
	if spec.Title != "" {
		fmt.Printf(" (%s", spec.Title)
		if spec.Version != "" {
			fmt.Printf(" v%s", spec.Version)
		}
		fmt.Print(")")
	}
	fmt.Println()
	if spec.BaseURL != "" {
		fmt.Printf("Base URL: %s\n", spec.BaseURL)
	}

	// Build auth config
	auth := &config.AuthConfig{
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}

	if addBaseURL != "" {
		auth.BaseURLOverride = addBaseURL
	}

	// Add manual headers from flags
	for _, h := range addHeaders {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			auth.Headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Auto-detect auth from securitySchemes
	schemes := openapi.DetectAuth(spec)
	if len(schemes) > 0 {
		fmt.Println("\nAuth detected:")
		for i, scheme := range schemes {
			fmt.Printf("  [%d] %s (%s)\n", i+1, scheme.Name, scheme.Description)
		}
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		for _, scheme := range schemes {
			promptAndStoreAuth(reader, scheme, auth)
		}
	}

	// Save auth if anything was configured
	if len(auth.Headers) > 0 || len(auth.QueryParams) > 0 || auth.BaseURLOverride != "" {
		if err := config.SaveAuth(name, auth); err != nil {
			return fmt.Errorf("failed to save auth config: %w", err)
		}
	}

	fmt.Printf("\nDone. Run `aurl %s --help` to see available endpoints.\n", name)
	return nil
}

func promptAndStoreAuth(reader *bufio.Reader, scheme openapi.AuthScheme, auth *config.AuthConfig) {
	switch scheme.Type {
	case "apiKey":
		fmt.Printf("Enter value for %s (or press Enter to skip): ", scheme.Name)
		value := readLine(reader)
		if value != "" {
			if scheme.In == "header" {
				auth.Headers[scheme.HeaderName] = value
			} else if scheme.In == "query" {
				auth.QueryParams[scheme.HeaderName] = value
			}
		}

	case "http":
		switch scheme.Scheme {
		case "bearer":
			fmt.Printf("Enter bearer token for %s (or press Enter to skip): ", scheme.Name)
			value := readLine(reader)
			if value != "" {
				auth.Headers["Authorization"] = "Bearer " + value
			}
		case "basic":
			fmt.Printf("Enter username for %s (or press Enter to skip): ", scheme.Name)
			username := readLine(reader)
			if username != "" {
				fmt.Print("Enter password: ")
				password := readLine(reader)
				encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
				auth.Headers["Authorization"] = "Basic " + encoded
			}
		default:
			fmt.Printf("Enter value for %s (or press Enter to skip): ", scheme.Name)
			value := readLine(reader)
			if value != "" {
				auth.Headers["Authorization"] = value
			}
		}

	case "oauth2", "openIdConnect":
		fmt.Printf("Enter token for %s (or press Enter to skip): ", scheme.Name)
		value := readLine(reader)
		if value != "" {
			auth.Headers["Authorization"] = "Bearer " + value
		}
	}
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func runAddGraphQL(name, endpoint string) error {
	// Build auth headers from flags
	headers := make(map[string]string)
	for _, h := range addHeaders {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	fmt.Printf("Introspecting %s...\n", endpoint)

	// Run introspection
	result, err := graphql.Introspect(endpoint, headers)
	if err != nil {
		return fmt.Errorf("introspection failed: %w", err)
	}

	// Parse to count queries/mutations
	schema, err := graphql.ParseSchema(result, endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Store as { "endpoint": "...", "schema": { ... } }
	stored := map[string]any{
		"endpoint": endpoint,
		"schema":   result,
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	if err := config.SaveGraphQL(name, data); err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}

	// Count queries and mutations
	queryCount := 0
	if qt, ok := schema.Types[schema.QueryType]; ok {
		queryCount = len(qt.Fields)
	}
	mutationCount := 0
	if schema.MutationType != "" {
		if mt, ok := schema.Types[schema.MutationType]; ok {
			mutationCount = len(mt.Fields)
		}
	}

	fmt.Printf("Registered %q (GraphQL: %d queries, %d mutations)\n", name, queryCount, mutationCount)
	fmt.Printf("Endpoint: %s\n", endpoint)

	// Save auth if headers were provided
	if len(headers) > 0 {
		auth := &config.AuthConfig{Headers: headers}
		if err := config.SaveAuth(name, auth); err != nil {
			return fmt.Errorf("failed to save auth: %w", err)
		}
	}

	fmt.Printf("\nDone. Run `aurl %s --help` to see available queries and mutations.\n", name)
	return nil
}

func fetchSpec(url string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}
