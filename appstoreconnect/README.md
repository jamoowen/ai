# App Store Connect MCP

`appstoreconnect-mcp` is a local STDIO [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server for Apple's App Store Connect API. It accepts an OpenAPI operation ID, parameters, and JSON body; it resolves and validates the request against the checked-in specification, pins the Apple destination, and creates the bearer JWT internally. Models never receive a general HTTP client or your Apple credentials.

Choose the integration that fits your application:

- **Codex or Claude Code:** configure either MCP host to launch the local binary.
- **A custom agent using MCP:** launch the binary from your agent and adapt its discovered MCP tools to your model provider's tool format.
- **A Go agent loop:** import the public `appstoreconnect` package and call its catalog, token, and client primitives directly.

The MCP server exposes five tools: `asc_search_operations`, `asc_describe_operation`, `asc_read`, `asc_write`, and `asc_delete`.

## Create Apple credentials

Before generating a key, the **Account Holder** must request App Store Connect API access in **Users and Access → Integrations → App Store Connect API → Request Access**. Apple reviews the request. Once access is enabled, choose the least-privileged key type and role that fits the agent's job. Apple's [App Store Connect API setup guide](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api) explains the access process.

### Team key

An Account Holder or Admin creates a team key in **Users and Access → Integrations → Team Keys**. Choose its role when generating it. A team key applies across all apps in the account according to that role; it cannot be restricted to selected apps.

Record the key's **Key ID** and the account's **Issuer ID** from the Integrations page. Use both values with this server.

### Individual key

An eligible user creates an individual key in **profile → Edit Profile → Individual API Key → Generate Key**. It inherits that user's App Store Connect permissions and app access, and each user can have one active individual key. It is useful when access should follow a particular user's scope.

Use its **Key ID**, but leave `ASC_ISSUER_ID` unset. Apple requires individual-key JWTs to use `sub=user` rather than a team issuer; this server selects that form when the issuer setting is empty. See Apple's [key-creation](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api) and [token-generation](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests) documentation.

### Protect the private key

Apple offers the downloaded `.p8` private key only once and does not keep a replacement copy. Keep it outside this repository, do not share it or put it in client-side code, and revoke it if it is lost or compromised. This repository ignores `*.p8` as a guardrail, not as a safe storage strategy.

For example, store it in a private directory and limit its file permissions:

```sh
chmod 600 /absolute/private/path/AuthKey_ABC123DEFG.p8
```

## Configure the environment

Export the values before starting an MCP host or custom agent. Use absolute paths: an MCP host may start the binary from a different working directory.

```sh
export ASC_KEY_ID="your-key-id"
export ASC_ISSUER_ID="your-team-issuer-id" # team keys only
export ASC_PRIVATE_KEY_PATH="/absolute/private/path/AuthKey_your-key-id.p8"
export ASC_OPENAPI_SOURCE="/absolute/path/to/ai/api/apple/app-store-connect.openapi.json"
```

For an individual key:

```sh
unset ASC_ISSUER_ID
```

| Variable | Purpose |
| --- | --- |
| `ASC_KEY_ID` | Apple API key ID. Required. |
| `ASC_ISSUER_ID` | Apple team issuer ID. Required for a team key; unset for an individual key. |
| `ASC_PRIVATE_KEY_PATH` | Absolute path to the private `.p8` key. Required. |
| `ASC_OPENAPI_SOURCE` | OpenAPI document path or HTTPS URL. Prefer an absolute path to the checked-in [`../api/apple/app-store-connect.openapi.json`](../api/apple/app-store-connect.openapi.json). The public raw URL is `https://raw.githubusercontent.com/jamoowen/ai/refs/heads/main/api/apple/app-store-connect.openapi.json`. |

Reads are available by default. Enable mutations only when the host is authorized to perform them:

```sh
export ASC_ALLOW_WRITES=true
export ASC_ALLOW_DELETES=true
export ASC_MAX_RESPONSE_BYTES=1048576 # optional; default is 1 MiB
```

`ASC_ALLOW_WRITES` permits POST and PATCH through `asc_write`; `ASC_ALLOW_DELETES` separately permits DELETE through `asc_delete`.

## Build and run

Build the local server from the repository root:

```sh
go build -o bin/appstoreconnect-mcp ./cmd/appstoreconnect-mcp
```

Running `bin/appstoreconnect-mcp` directly starts a STDIO protocol server and waits for MCP messages. It is not an interactive shell command; normally Codex, Claude Code, or your custom agent launches it and owns its standard input and output.

## Connect Codex

Codex supports local STDIO MCP servers through `~/.codex/config.toml` or a trusted project's `.codex/config.toml`. Configure an absolute binary path and forward only locally exported secrets. See the official [Codex MCP documentation](https://developers.openai.com/codex/mcp).

For a team key:

```toml
[mcp_servers.app_store_connect]
command = "/absolute/path/to/ai/bin/appstoreconnect-mcp"
env_vars = ["ASC_KEY_ID", "ASC_ISSUER_ID", "ASC_PRIVATE_KEY_PATH"]

[mcp_servers.app_store_connect.env]
ASC_OPENAPI_SOURCE = "/absolute/path/to/ai/api/apple/app-store-connect.openapi.json"
```

For an individual key, omit `ASC_ISSUER_ID` from `env_vars`. Add the mutation variables only when you intentionally want them available to this server.

Verify the registration with `codex mcp list`; inside Codex, use `/mcp` to inspect its connection and tools.

## Connect Claude Code

Claude Code can register a local STDIO server with `claude mcp add [options] <name> -- <command> [args...]`. The `--` separates Claude Code options from the subprocess command. This project-local team-key configuration uses explicit STDIO transport and forwards exported variables:

```sh
claude mcp add \
  --scope local \
  --env ASC_KEY_ID="$ASC_KEY_ID" \
        ASC_ISSUER_ID="$ASC_ISSUER_ID" \
        ASC_PRIVATE_KEY_PATH="$ASC_PRIVATE_KEY_PATH" \
        ASC_OPENAPI_SOURCE="$ASC_OPENAPI_SOURCE" \
  --transport stdio \
  app-store-connect -- \
  /absolute/path/to/ai/bin/appstoreconnect-mcp
```

For an individual key, omit the `ASC_ISSUER_ID=...` assignment. `--scope local` keeps the registration private to this project; use another scope only if that broader availability is deliberate. Verify it with `claude mcp get app-store-connect` or `claude mcp list`, and use `/mcp` in Claude Code for connection status.

For a shareable project configuration, a repository-root `.mcp.json` can reference each developer's environment without committing resolved secrets:

```json
{
  "mcpServers": {
    "app-store-connect": {
      "type": "stdio",
      "command": "${ASC_MCP_BINARY}",
      "env": {
        "ASC_KEY_ID": "${ASC_KEY_ID}",
        "ASC_ISSUER_ID": "${ASC_ISSUER_ID:-}",
        "ASC_PRIVATE_KEY_PATH": "${ASC_PRIVATE_KEY_PATH}",
        "ASC_OPENAPI_SOURCE": "${ASC_OPENAPI_SOURCE}"
      }
    }
  }
}
```

Project-scoped servers require user approval before use. See the official [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp) for scopes and configuration details.

## Use the MCP tools

Use the tools in this order:

1. Call `asc_search_operations` with a task-oriented query and optional HTTP method.
2. Call `asc_describe_operation` for the returned `operationId`; inspect its parameters, request body, and referenced schemas.
3. Invoke that exact operation through `asc_read`, `asc_write`, or `asc_delete`.

Never invent an operation ID. Inspect schemas before mutations, and request each page explicitly: the server does not perform automatic pagination or high-level workflows.

For example, search first:

```json
{"query":"list apps","method":"GET","limit":10}
```

Then pass the selected identifier to `asc_describe_operation`:

```json
{"operationId":"apps_getCollection"}
```

An invocation has this shape; include only the parameters required by the described operation:

```json
{
  "operationId": "apps_getCollection",
  "query": {"limit": 5}
}
```

`asc_read` accepts GET operations only, `asc_write` accepts POST and PATCH operations only, and `asc_delete` accepts DELETE operations only. The server validates paths, query parameters, request bodies, and the destination before sending a request to Apple.

## Use the MCP from a custom agent

For an agent framework that already supports MCP, point it at the built binary and pass the same environment variables. For a framework with its own tool abstraction, use this adapter pattern:

1. Keep one MCP session alive for the agent loop.
2. Discover tools when the session starts.
3. Map each tool's name, description, and `InputSchema` into the model provider's tool definition.
4. Forward the model's chosen tool name and JSON arguments to MCP unchanged.
5. Check both the transport/protocol error and the tool result's error flag, then append the returned content to the next model turn.

Preserve MCP tool annotations in your model-facing policy. In particular, require an explicit authorization or confirmation step before `asc_write` and `asc_delete`.

The public integration boundary is the executable. An external module cannot import this repository's `internal/appstoreconnectmcp` package.

### Go MCP SDK example

This example uses `github.com/modelcontextprotocol/go-sdk v1.8.0` to start a local server and call a discovered tool. Keep the session for the lifetime of the agent loop, and close it when the loop ends.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()
	cmd := exec.Command("/absolute/path/to/ai/bin/appstoreconnect-mcp")
	cmd.Env = os.Environ() // Includes the exported ASC_* variables.

	client := mcp.NewClient(&mcp.Implementation{Name: "my-agent", Version: "v1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer session.Close()

	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", tool.Name, tool.Description)
		// Register tool.Name, tool.Description, and tool.InputSchema with the model.
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "asc_search_operations",
		Arguments: map[string]any{
			"query": "apps", "method": "GET", "limit": 10,
		},
	})
	if err != nil {
		log.Fatal(err) // Transport or MCP protocol error.
	}
	if result.IsError {
		log.Fatalf("tool failed: %#v", result.Content)
	}

	response, err := json.MarshalIndent(result.StructuredContent, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(response))
}
```

`session.Tools` handles tool-list pagination. If your agent needs finer control, use `session.ListTools`; for every invocation, check the Go `error` and `result.IsError`. The [Go MCP SDK v1.8.0 documentation](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0) covers the protocol and transports.

## Use the Go primitives directly

For Go-only agent loops, import `github.com/jamoowen/ai/appstoreconnect`. The public package gives you a catalog, a token source, and a constrained client without requiring MCP.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jamoowen/ai/appstoreconnect"
)

func main() {
	catalog, err := appstoreconnect.LoadCatalogSource(os.Getenv("ASC_OPENAPI_SOURCE"))
	if err != nil {
		log.Fatal(err)
	}
	tokens, err := appstoreconnect.NewES256TokenSource(
		os.Getenv("ASC_KEY_ID"),
		os.Getenv("ASC_ISSUER_ID"),
		os.Getenv("ASC_PRIVATE_KEY_PATH"),
		0,
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}
	client := &appstoreconnect.Client{Catalog: catalog, Tokens: tokens}

	matches := catalog.Search("list apps", "GET", 10)
	if len(matches) == 0 {
		log.Fatal("no matching operation")
	}
	description, err := catalog.Describe(matches[0].OperationID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("operation: %#v\n", description["operation"])

	response, err := client.Invoke(context.Background(), appstoreconnect.ReadOperation, appstoreconnect.Invocation{
		OperationID: "apps_getCollection",
		Query:       map[string]any{"limit": 5},
	})
	if response != nil {
		fmt.Printf("status: %d\n", response.Status)
	}
	if err != nil {
		log.Fatal(err)
	}
}
```

`Client.Invoke` can return a non-nil response alongside an error, such as for a non-2xx Apple response, so inspect both. The direct package constrains operations by `ReadOperation`, `WriteOperation`, and `DeleteOperation`, but it does not enforce `ASC_ALLOW_WRITES` or `ASC_ALLOW_DELETES`. Direct users own authorization and confirmation controls around `WriteOperation` and `DeleteOperation`.

## Troubleshooting and limits

- **The schema cannot be found:** set `ASC_OPENAPI_SOURCE` to an absolute checked-in path or an HTTPS URL. The default is relative to the binary's working directory.
- **Apple rejects authentication:** use `ASC_ISSUER_ID` only for a team key. Leave it unset for an individual key, and ensure the `.p8` file corresponds to `ASC_KEY_ID`.
- **A mutation says it is disabled:** set the appropriate exact opt-in, `ASC_ALLOW_WRITES=true` or `ASC_ALLOW_DELETES=true`, in the process that launches the server.
- **Output looks like JSON-RPC:** standard output is MCP protocol traffic. Do not add ordinary logs to it; use standard error for diagnostics.
- **A response body is rejected:** the server returns JSON values and text only; binary response bodies are unsupported.

The current server intentionally has no automatic pagination, embeddings or semantic search, high-level workflow tools, binary upload/download support, remote or streamable MCP hosting, or live Apple integration tests.

## References

- [Apple: App Store Connect API setup](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api)
- [Apple: Creating API keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api)
- [Apple: Generating tokens for API requests](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests)
- [OpenAI: Codex MCP](https://developers.openai.com/codex/mcp)
- [Anthropic: Claude Code MCP](https://code.claude.com/docs/en/mcp)
- [Model Context Protocol Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)
