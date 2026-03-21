package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/shawnpana/aurl/internal/client"
	"github.com/shawnpana/aurl/internal/config"
	"github.com/shawnpana/aurl/internal/oauth2"
	"github.com/shawnpana/aurl/internal/openapi"
	"github.com/spf13/cobra"
)

// NewAPICommand creates a dynamic cobra command for a registered API.
func NewAPICommand(name, specPath, authPath string) *cobra.Command {
	return &cobra.Command{
		Use:                name + " [METHOD] [path] [body]",
		Short:              "Interact with the " + name + " API",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAPI(name, specPath, authPath, args)
		},
	}
}

func runAPI(name, specPath, authPath string, args []string) error {
	// Load and parse spec
	data, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("API %q not found. Run 'aurl list' to see registered APIs.", name)
	}

	spec, err := openapi.Parse(data)
	if err != nil {
		return fmt.Errorf("failed to parse spec for %q: %w", name, err)
	}

	// Show help if no args or help flag
	if len(args) == 0 || isHelpArg(args[0]) {
		fmt.Print(openapi.FormatHelp(spec))
		return nil
	}

	// Handle "describe METHOD /path"
	if args[0] == "describe" {
		if len(args) < 3 {
			return fmt.Errorf("Usage: aurl %s describe METHOD /path", name)
		}
		fmt.Print(openapi.FormatDescribe(spec, args[1], args[2]))
		return nil
	}

	// Handle "docs" — open externalDocs URL
	if args[0] == "docs" {
		if spec.ExternalDocs == nil || spec.ExternalDocs.URL == "" {
			fmt.Println("No external documentation URL found in spec.")
			return nil
		}
		url := spec.ExternalDocs.URL
		fmt.Println(url)
		return openBrowser(url)
	}

	// Expect: METHOD path [body]
	if len(args) < 2 {
		fmt.Print(openapi.FormatHelp(spec))
		return fmt.Errorf("\nUsage: aurl %s [METHOD] [path] [body]", name)
	}

	method := strings.ToUpper(args[0])
	path := args[1]

	// Normalize path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	var body []byte
	if len(args) >= 3 {
		body = []byte(args[2])
	}

	// Validate against spec
	enumErrors, missingFields := openapi.ValidateRequest(spec, method, path, body)

	// Enum violations are hard errors
	if len(enumErrors) > 0 {
		for _, e := range enumErrors {
			fmt.Fprintf(os.Stderr, "Error: %s\n", e.Error())
		}
		return fmt.Errorf("validation failed")
	}

	// Missing required fields are warnings with prompt
	if len(missingFields) > 0 {
		var fieldNames []string
		for _, e := range missingFields {
			fieldNames = append(fieldNames, e.Field)
		}
		fmt.Fprintf(os.Stderr, "Warning: missing required fields: %s\n", strings.Join(fieldNames, ", "))

		example := openapi.FindBodyExample(spec, method, path)
		if example != "" {
			fmt.Fprintf(os.Stderr, "  expected: '%s'\n", example)
		}

		fmt.Fprint(os.Stderr, "Send anyway? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted")
		}
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

	// Build base URL
	baseURL := spec.BaseURL
	if auth != nil && auth.BaseURLOverride != "" {
		baseURL = auth.BaseURLOverride
	}
	if baseURL == "" {
		return fmt.Errorf("no base URL found in spec. Use 'aurl auth %s --base-url URL' or re-add with --base-url", name)
	}

	// Strip trailing slash from base URL
	baseURL = strings.TrimRight(baseURL, "/")

	// Build request
	req := &client.Request{
		Method:  method,
		URL:     baseURL + path,
		Body:    body,
		Headers: make(map[string]string),
	}

	// Apply auth headers
	if auth != nil {
		for k, v := range auth.Headers {
			req.Headers[k] = v
		}
		// Auth query params get appended to the URL
		if len(auth.QueryParams) > 0 {
			req.QueryParams = auth.QueryParams
		}
	}

	// Execute
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	// Print response
	client.PrintResponseWithStatus(resp)

	// On 4xx, suggest expected body
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		example := openapi.FindBodyExample(spec, method, path)
		if example != "" && example != "{}" {
			fmt.Fprintf(os.Stderr, "\nExpected body for %s %s:\n  '%s'\n", method, path, example)
		}
	}

	// Exit code based on status
	if resp.StatusCode >= 400 {
		os.Exit(2)
	}

	return nil
}

func isHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return nil
	}
}
