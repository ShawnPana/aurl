# aurl

A command line tool for turning any API into a CLI command, supporting OpenAPI 3.0, OpenAPI 3.1, Swagger 2.0, and GraphQL. Built for AI agents — aurl makes APIs as easy to use as tool calls.

Register any API by name, and aurl parses the spec to auto-detect auth, validate requests, generate documentation, and provide example bodies. Agents like Claude Code, Codex, and Cursor can discover endpoints via `--help`, understand parameters and types via `describe`, and make validated requests — all without reading raw API docs.

## Install

**Homebrew:**

```bash
brew install shawnpana/tap/aurl
```

**Go:**

```bash
go install github.com/shawnpana/aurl@latest
```

**From source:**

```bash
git clone https://github.com/ShawnPana/aurl.git
cd aurl
make install
```

## Agent Skill

Install the aurl skill so your coding agent knows how to use it:

```bash
npx skills add ShawnPana/aurl
```

This works with Claude Code, Cursor, Codex, Copilot, and [40+ other agents](https://skills.sh). The skill teaches your agent how to register APIs, explore endpoints, and make requests with aurl.

## Quick Start

Register an API, then use it:

```bash
# Register a REST API
aurl add petstore https://petstore3.swagger.io/api/v3/openapi.json

# Register a GraphQL API
aurl add --graphql linear https://api.linear.app/graphql

# See what's available
aurl petstore --help
aurl linear --help

# Make requests
aurl petstore GET /pet/1
aurl linear '{ viewer { name email } }'
```

## Usage

### Register an API

```bash
# From a URL
aurl add petstore https://petstore3.swagger.io/api/v3/openapi.json

# From a local file
aurl add myapi ./openapi.json

# With a base URL override (if the spec doesn't include one)
aurl add myapi ./openapi.json --base-url https://api.example.com

# With auth headers
aurl add myapi https://api.example.com/openapi.json --header "Authorization: Bearer token"

# GraphQL endpoint
aurl add --graphql linear https://api.linear.app/graphql
```

When registering, aurl parses the spec's `securitySchemes` and prompts you for credentials:

```
Registered "petstore" (Swagger Petstore v1.0.27)
Base URL: https://petstore3.swagger.io/api/v3

Auth detected:
  [1] api_key (API Key in header "api_key")
  [2] petstore_auth (OAuth2 (provide token manually))

Enter value for api_key (or press Enter to skip): sk-xxxxx
Enter value for petstore_auth (or press Enter to skip):
```

### Explore an API

```bash
# See all endpoints grouped by tag
aurl petstore --help
```

```
Swagger Petstore - OpenAPI 3.0 v1.0.27
Base URL: https://petstore3.swagger.io/api/v3

A sample Pet Store Server based on the OpenAPI 3.0 specification.
Docs: https://swagger.io

[pet] Everything about your Pets
  POST   /pet                           # Add a new pet to the store.
    body: '{"id":0,"name":"...","photoUrls":["..."],"status":"available"}'
    200: Pet  400: Invalid input
  GET    /pet/findByStatus              # Finds Pets by status.
    params: status* (available|pending|sold) - Status values to filter by
    200: Pet[]  400: Invalid status value
  GET    /pet/{petId}                   # Find pet by ID.
    params: petId* - ID of pet to return
    200: Pet  400: Invalid ID supplied  404: Pet not found

(*) = required
```

```bash
# Detailed docs for a single endpoint
aurl petstore describe GET /pet/{petId}
```

```
GET /pet/{petId}
Find pet by ID.

Returns a single pet.

Parameters:
  petId* (path, integer)
    ID of pet to return

Responses:
  200 - successful operation
    Schema: Pet
  400 - Invalid ID supplied
  404 - Pet not found
```

```bash
# Open external documentation in browser
aurl petstore docs
```

### Make Requests

**REST:**

```bash
# GET
aurl petstore GET /pet/1

# GET with query params
aurl petstore GET '/pet/findByStatus?status=available'

# POST with JSON body
aurl petstore POST /pet '{"name":"doggie","photoUrls":["http://example.com"]}'

# PUT
aurl petstore PUT /pet '{"id":1,"name":"updated","photoUrls":["http://example.com"]}'

# DELETE
aurl petstore DELETE /pet/1
```

**GraphQL:**

```bash
# Query
aurl linear '{ viewer { name email } }'

# Query with variables
aurl linear '{ issue(id: $id) { title state { name } } }' '{"id":"ABC-123"}'

# Mutation
aurl linear 'mutation { issueCreate(input: {title: "Bug", teamId: "xxx"}) { issue { id } } }'
```

### Validation

aurl validates your requests against the spec before sending:

**Enum violations** are caught immediately:

```
$ aurl petstore GET '/pet/findByStatus?status=invalid'
Error: status: invalid value "invalid"
  allowed: available, pending, sold
```

**Missing required fields** trigger a warning:

```
$ aurl petstore POST /pet '{"tag":"dog"}'
Warning: missing required fields: name, photoUrls
  expected: '{"name":"...","photoUrls":["..."]}'
Send anyway? [y/N]:
```

**4xx errors** suggest the expected body from the spec:

```
$ aurl petstore POST /pet
HTTP 415
Expected body for POST /pet:
  '{"name":"...","photoUrls":["..."],"status":"available"}'
```

### Manage APIs

```bash
# List all registered APIs
aurl list

NAME      TYPE     TITLE                      VERSION  ENDPOINT
petstore  api      Swagger Petstore           1.0.27   https://petstore3.swagger.io/api/v3
linear    graphql                                       https://api.linear.app/graphql

# Reconfigure auth
aurl auth petstore --header "Authorization: Bearer new-token"

# Remove an API
aurl remove petstore
```

## Auth

aurl supports all standard OpenAPI security schemes:

| Scheme | What aurl does |
|--------|--------------|
| `apiKey` | Detects header/query param name from spec, prompts for value |
| `http` + `bearer` | Prompts for token, sets `Authorization: Bearer <token>` |
| `http` + `basic` | Prompts for username + password, encodes as Basic auth |
| `oauth2` / `openIdConnect` | Prompts for a token manually |

Auth is stored in `~/.aurl/auth/` with `0600` permissions.

For GraphQL APIs or APIs without `securitySchemes`, use `--header`:

```bash
aurl add myapi https://api.example.com/openapi.json --header "X-Api-Key: secret"
aurl auth myapi --header "Authorization: Bearer new-token"
```

## How It Works

aurl stores API specs and auth config locally:

```
~/.aurl/
├── apis/          # OpenAPI spec files
├── graphql/       # GraphQL introspection results
└── auth/          # Auth headers per API (0600 permissions)
```

When you run `aurl [name]`, it:

1. Loads the spec from `~/.aurl/apis/` or `~/.aurl/graphql/`
2. Parses it to understand endpoints, parameters, types, and auth
3. For `--help`: generates documentation from the spec
4. For requests: validates against the spec, attaches auth headers, executes, and pretty-prints the response

Specs are parsed lazily — aurl only reads the spec for the command you invoke. Adding 50 APIs doesn't slow down startup.

## Shell Completions

```bash
# Zsh
aurl completion zsh > "${fpath[1]}/_aurl"

# Bash
aurl completion bash > /etc/bash_completion.d/aurl

# Fish
aurl completion fish > ~/.config/fish/completions/aurl.fish
```

## License

MIT
