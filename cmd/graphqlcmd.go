package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/shawnpana/aurl/internal/client"
	"github.com/shawnpana/aurl/internal/config"
	"github.com/shawnpana/aurl/internal/graphql"
	"github.com/shawnpana/aurl/internal/oauth2"
	"github.com/spf13/cobra"
)

// NewGraphQLCommand creates a dynamic cobra command for a registered GraphQL API.
func NewGraphQLCommand(name, configPath, authPath string) *cobra.Command {
	return &cobra.Command{
		Use:                name + " [query] [variables]",
		Short:              "Query the " + name + " GraphQL API",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraphQL(name, configPath, authPath, args)
		},
	}
}

func runGraphQL(name, configPath, authPath string, args []string) error {
	// Load config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("GraphQL API %q not found. Run 'aurl list' to see registered APIs.", name)
	}

	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("invalid config for %q: %w", name, err)
	}

	endpoint, _ := stored["endpoint"].(string)
	schemaData, _ := stored["schema"].(map[string]any)

	schema, err := graphql.ParseSchema(schemaData, endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse schema for %q: %w", name, err)
	}

	// Help
	if len(args) == 0 || isHelpArg(args[0]) {
		fmt.Print(graphql.FormatHelp(schema))
		return nil
	}

	// Describe
	if args[0] == "describe" {
		if len(args) < 2 {
			return fmt.Errorf("Usage: aurl %s describe [query|mutation|type]", name)
		}
		fmt.Print(graphql.FormatDescribe(schema, args[1]))
		return nil
	}

	// Execute query
	query := args[0]

	// Build the GraphQL request body
	gqlBody := map[string]any{
		"query": query,
	}

	// Optional variables as second arg
	if len(args) >= 2 {
		var vars map[string]any
		if err := json.Unmarshal([]byte(args[1]), &vars); err != nil {
			return fmt.Errorf("invalid variables JSON: %w", err)
		}
		gqlBody["variables"] = vars
	}

	bodyBytes, err := json.Marshal(gqlBody)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// Load auth
	auth, _ := config.LoadAuth(name)

	// OAuth2 token refresh
	if auth != nil && auth.OAuth2 != nil {
		token, err := oauth2.EnsureValidToken(name, auth)
		if err != nil {
			return fmt.Errorf("OAuth2 token refresh failed: %w", err)
		}
		if auth.Headers == nil {
			auth.Headers = make(map[string]string)
		}
		auth.Headers["Authorization"] = "Bearer " + token
	}

	// Build request
	req := &client.Request{
		Method:  "POST",
		URL:     endpoint,
		Body:    bodyBytes,
		Headers: make(map[string]string),
	}
	req.Headers["Content-Type"] = "application/json"

	if auth != nil {
		for k, v := range auth.Headers {
			req.Headers[k] = v
		}
		if len(auth.QueryParams) > 0 {
			req.QueryParams = auth.QueryParams
		}
	}

	// Execute
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	// Pretty-print response
	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "HTTP %d\n", resp.StatusCode)
	}

	// Try to extract just the data/errors from the response
	var result map[string]any
	if json.Unmarshal(resp.Body, &result) == nil {
		// Check for GraphQL errors
		if errors, ok := result["errors"].([]any); ok && len(errors) > 0 {
			fmt.Fprintln(os.Stderr, "GraphQL errors:")
			for _, e := range errors {
				if errMap, ok := e.(map[string]any); ok {
					if msg, ok := errMap["message"].(string); ok {
						fmt.Fprintf(os.Stderr, "  - %s\n", msg)
					}
				}
			}
			if result["data"] == nil {
				os.Exit(2)
			}
		}

		// Print data
		if data, ok := result["data"]; ok && data != nil {
			pretty, _ := json.MarshalIndent(data, "", "  ")
			fmt.Println(string(pretty))
			return nil
		}
	}

	// Fallback: print raw
	client.PrintResponse(resp)

	if resp.StatusCode >= 400 {
		os.Exit(2)
	}

	return nil
}

// isGraphQLQuery checks if a string looks like a GraphQL query.
func isGraphQLQuery(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{") ||
		strings.HasPrefix(s, "query") ||
		strings.HasPrefix(s, "mutation") ||
		strings.HasPrefix(s, "subscription")
}
