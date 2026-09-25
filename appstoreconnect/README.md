# App Store Connect MCP

Give an AI agent access to Apple's App Store Connect API through a local MCP server, or use the underlying Go primitives in your own agent.

Requests are validated against Apple's OpenAPI specification and signed locally using your API key. You provide the path to the private key, not its contents.

Maintainers can find the server's security and loading invariants in [DESIGN.md](DESIGN.md).

Choose your integration:

- **Existing agent:** use the setup prompt below with Codex, Claude Code, or another local MCP client.
- **Custom agent:** import the Go package directly, or connect through your framework's MCP client.

## 1. Set up with an existing AI agent

### Create your Apple API key

For personal use, create an **Individual API Key** in App Store Connect under **your username → Edit Profile → Individual API Key**. Download the `.p8` file and record its **Key ID**. An individual key does not use an Issuer ID.

For shared automation, create a **Team Key** under **Users and Access → Integrations → Team Keys** and also record the **Issuer ID**. Team keys apply across all apps in the account.

Use the minimum permissions your agent needs. App Manager access supports submitting and releasing apps. See [Apple's API setup guide](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api) for access requirements and key creation.

Keep the `.p8` file outside this repository. Apple only lets you download it once. Never paste its contents into a prompt, configuration file, or commit.

```sh
mkdir -p ~/.secrets/appstoreconnect

mv ~/Downloads/AuthKey_XXXXXXXXXX.p8 \
  ~/.secrets/appstoreconnect/

chmod 600 \
  ~/.secrets/appstoreconnect/AuthKey_XXXXXXXXXX.p8
```

`chmod 600` means only your user account can read or modify the file.

### Give your agent this prompt

With Go installed, open this repository in your coding agent. Fill in the values below and send the prompt. The agent handles the build and persistent MCP configuration.

```text
Set up this repository's App Store Connect MCP server for me.

MCP client: <Codex / Claude Code / another local MCP client>
Key ID: <your key ID>
Private key path: <absolute path to your .p8 file>
Issuer ID: <team keys only; leave blank for an individual key>

Inspect the repository and my client's existing MCP configuration.
Use the client's supported configuration format; check its current
help or official documentation rather than guessing.

Build from the repository root:
go build -o bin/appstoreconnect-mcp ./cmd/appstoreconnect-mcp

Register the binary as a local stdio MCP server. Use absolute paths.
Update an existing registration rather than adding a duplicate, and
preserve unrelated settings.

Persist these environment values in the client's local MCP configuration:
- ASC_KEY_ID: the Key ID above.
- ASC_PRIVATE_KEY_PATH: the private key path above.
- ASC_ISSUER_ID: empty for an individual key, or the supplied team Issuer ID.

Do not require me to export variables before each session. Keep writes
and deletes disabled, including any existing or inherited opt-ins.

Check that the private key is outside the repository. On macOS/Linux,
restrict the key file to chmod 600. Do not open or print its contents,
copy it into configuration, or include it in logs or commits.

Verify MCP initialization and tool discovery. Then search for the
list-apps GET operation, inspect its schema, and make one read-only
request for up to five apps. Do not modify App Store Connect data.

Show the relevant configuration changes and test results without exposing
unrelated secrets. If a client restart is needed, explain the next step
and identify any checks you could not run.
```

The server starts read-only. To enable changes later, explicitly ask your agent to set `ASC_ALLOW_WRITES=true` for POST/PATCH or `ASC_ALLOW_DELETES=true` for DELETE in the server's persistent configuration. The flags are independent. Restart or reconnect the server after changing them, and keep approval for changes in your agent's workflow.

## 2. Use primitives in a custom agent

For a Go agent, import `github.com/jamoowen/ai/appstoreconnect`. The package provides operation discovery, schema inspection, token signing, and a constrained API client without running an MCP server.

Use the same Apple key described above. Your application supplies the configuration; it does not read your Codex or Claude Code MCP settings automatically.

### Call the Go client directly

This read-only example inspects the known list-apps operation, makes a request for up to five apps, and prints the HTTP status. Replace the placeholder values with your application's configuration.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jamoowen/ai/appstoreconnect"
)

func main() {
	catalog, err := appstoreconnect.LoadCatalogSource(
		"", // Uses the cached specification and refreshes it from GitHub.
	)
	if err != nil {
		log.Fatal(err)
	}

	tokens, err := appstoreconnect.NewES256TokenSource(
		"YOUR_KEY_ID",
		"", // Individual key; supply an Issuer ID for a team key.
		"/absolute/private/path/AuthKey_YOUR_KEY_ID.p8",
		0,
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}

	client := &appstoreconnect.Client{Catalog: catalog, Tokens: tokens}

	// Inspect the same operation that will be invoked.
	const operationID = "apps_getCollection"
	description, err := catalog.Describe(operationID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("operation: %#v\n", description["operation"])

	response, err := client.Invoke(
		context.Background(),
		appstoreconnect.ReadOperation,
		appstoreconnect.Invocation{
			OperationID: operationID,
			Query:       map[string]any{"limit": 5},
		},
	)
	// Apple may return a response alongside an error; check both.
	if response != nil {
		fmt.Printf("status: %d\n", response.Status)
	}
	if err != nil {
		log.Fatal(err)
	}
}
```

The program above makes direct Go API calls. It does not register a tool that a model can select.

### Register it as a model tool

For an agent implemented inside this repository, wrap the client in `internal/tools.NewTool`. This creates a model-facing name, description, and JSON schema while keeping the Apple client and credentials in your application. This read-only wrapper fixes the operation class to `ReadOperation`, regardless of the model's arguments:

```go
import (
	"context"
	"encoding/json"

	"github.com/jamoowen/ai/appstoreconnect"
	"github.com/jamoowen/ai/internal/tools"
)

func newReadTool(client *appstoreconnect.Client) (tools.Tool, error) {
	return tools.NewTool[appstoreconnect.Invocation](
		"asc_read",
		"Read an App Store Connect GET operation after inspecting its schema.",
		func(ctx context.Context, input appstoreconnect.Invocation) (string, error) {
			response, err := client.Invoke(ctx, appstoreconnect.ReadOperation, input)
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				return "", err
			}
			return string(encoded), nil
		},
	)
}
```

Register the returned tool's `Name`, `Description`, and `Parameters` with your model provider. When the model selects it, pass its raw JSON arguments to `Handler` and add the returned text to the next model turn. Register similar read-only wrappers for `catalog.Search`, `catalog.Describe`, and `catalog.DescribeSchema` so the model can search, inspect an operation, inspect only the relevant schemas, then read an operation.

`internal/tools` can only be imported by code in this Go module. An external Go module must adapt the public `appstoreconnect` package to its own harness's tool interface, or use the MCP server. The current example loop in `internal/agents/simpleagentwithloop` does not register or execute tool calls, so it needs a model integration and tool-dispatch loop before it can use this wrapper.

Keep credentials in your application, never in model tool arguments.

**The Go primitives do not enforce `ASC_ALLOW_WRITES` or `ASC_ALLOW_DELETES`.** Your application must authorize changes before invoking `WriteOperation` or `DeleteOperation`. For a read-only agent, keep the wrapper restricted to `ReadOperation`.

### Already using an MCP framework?

Use the server from section 1 instead of importing the Go package. Launch the binary through your framework's stdio MCP client and supply the environment values below. Keep one session open for the agent loop, discover its tools, and forward tool calls and results. Check both transport errors and the tool result's `isError` field. An Apple API error result includes its status, content type, and safe rate-limit/request-ID headers, but never its raw response body.

This route also works for agents written in other languages. External Go modules should use the public package or the MCP binary, not `internal/appstoreconnectmcp`.

## Reference

### MCP tools

| Tool | Purpose |
| --- | --- |
| `asc_search_operations` | Find operations by search terms and optional HTTP method. |
| `asc_describe_operation` | Inspect an operation's method, path, parameters, request body, and response references. |
| `asc_describe_schema` | Inspect one named component schema referenced by an operation. |
| `asc_read` | Execute a GET operation. |
| `asc_write` | Execute POST/PATCH when writes are enabled. |
| `asc_delete` | Execute DELETE when deletes are enabled. |

For agent-selected operations, use **search → describe operation → describe relevant schemas → execute**. Use exact returned `operationId` and schema names. `asc_read` is the read-only tool; inspect request schemas before calling `asc_write` or `asc_delete`. Description results are capped at 32 KiB and fail clearly when a single operation or schema cannot fit.

### MCP environment

These values configure the MCP server process. The setup prompt supplies them through your client's configuration, not repeated shell exports.

| Variable | Value |
| --- | --- |
| `ASC_KEY_ID` | Required Apple API Key ID. |
| `ASC_PRIVATE_KEY_PATH` | Required absolute path to the `.p8` private key. |
| `ASC_ISSUER_ID` | Team Issuer ID; empty or unset for individual keys. |
| `ASC_OPENAPI_SOURCE` | Optional path or HTTPS URL to the API definition JSON. When unset, the server conditionally fetches the current checked-in specification from GitHub, caches it in the user's cache directory, and sends `If-None-Match` on later starts. Set this only to override that default, for example to use a local or pinned HTTPS definition. |
| `ASC_ALLOW_WRITES` | Set to `true` to allow POST/PATCH. Disabled by default. |
| `ASC_ALLOW_DELETES` | Set to `true` to allow DELETE. Disabled by default. |
| `ASC_MAX_RESPONSE_BYTES` | Maximum response size in bytes. Default: `1048576` (1 MiB). |

No OpenAPI source configuration is needed for normal use. The default source is the checked-in definition in this repository, fetched through GitHub's Contents API. It is cached at the operating system's user cache location under `appstoreconnect-mcp`; a 304 response reuses the validated cache without downloading the JSON again. If refresh fails, the server uses a previously validated cache and reports the condition on stderr. An explicit `ASC_OPENAPI_SOURCE` accepts a local path or HTTPS URL and bypasses this cache behavior. Set an absolute local path when using that override to avoid dependence on the server's working directory.

### Limits and security

The MCP server supports local stdio connections, not remote hosting. It does not provide automatic pagination, semantic search, high-level workflows, binary uploads/downloads, or live Apple integration tests. Request additional API pages explicitly.

The binary waits for MCP messages; it is not an interactive terminal application. Send diagnostic logs to stderr, not stdout, which is reserved for MCP traffic.

Keep private keys out of prompts, source control, and logs. Revoke a lost or exposed key in App Store Connect. Enabling writes or deletes allows those operations; your agent or application must still enforce authorization for changes.

### Documentation

[Apple API setup](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api) · [API keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api) · [Token generation](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests) · [Codex MCP](https://developers.openai.com/codex/mcp) · [Claude Code MCP](https://code.claude.com/docs/en/mcp)
