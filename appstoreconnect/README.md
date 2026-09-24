# App Store Connect MCP

`appstoreconnect-mcp` lets an AI agent work with Apple's App Store Connect API safely. It knows Apple's OpenAPI specification, validates every request, sends it only to Apple, and creates the Apple login token itself. Your model does not receive a general HTTP tool or your Apple credentials.

Choose the path that fits your agent:

- **Codex or Claude Code:** configure the MCP host to start this local server.
- **Custom agent with MCP:** start the binary from your agent and adapt its discovered tools to your model provider.
- **Go-only agent loop:** import the public `appstoreconnect` package and use its catalog, token source, and client directly.

The server provides five tools: `asc_search_operations`, `asc_describe_operation`, `asc_read`, `asc_write`, and `asc_delete`.

## Create Apple credentials

Follow these steps before configuring the MCP.

### 1. Request API access

The **Account Holder** must request App Store Connect API access in **Users and Access → Integrations → App Store Connect API → Request Access**. Apple reviews the request before keys can be created. See Apple's [API setup guide](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api).

### 2. Choose the right role

Use the least powerful role that can do the job. If your agent must submit an app for review and release an approved version, use **App Manager**.

- **App Manager** is the least-privileged role that can submit apps for review and release them.
- **Admin** and **Account Holder** also work, but give the key more access.
- **Developer** can upload builds, but cannot submit an app or release it.

An Account Holder or Admin gives a user their role and app access. When creating a team key, select its role during key creation.

### 3. Choose an individual or team key

Both key types can submit and release apps when they have App Manager-level permission. The difference is who owns the key and which apps it can reach.

| If you are doing this... | Choose | Why |
| --- | --- | --- |
| Running an agent locally for yourself | **Individual App Manager key** | Recommended for personal use. It follows your user role and app access, so it can be limited to the apps you can access. |
| Running shared CI or an organization-owned service | **Team App Manager key** | It does not depend on one person's account and lets you manage separate service credentials. It reaches every app in the account. |

Individual keys:

- Inherit the user's App Store Connect role and app access.
- Can be limited through that user's app access.
- Are limited to one active individual key per user.
- Cannot use Provisioning, Sales and Finance, or `notaryTool`.

Team keys:

- Can be managed as separate credentials for people or services.
- Apply to every app in the App Store Connect account; Apple does not support app-scoped team keys.

For most personal/local agent setups, start with an **individual App Manager key**. For shared unattended automation, use a **team App Manager key** and remember that it can reach all account apps. Apple's [API key documentation](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api) describes both kinds of key.

### 4. Create the key and record its values

| Key type | Where to create it | Values to keep |
| --- | --- | --- |
| Individual | **Profile → Edit Profile → Individual API Key → Generate Key** | Key ID and the downloaded `.p8` file. Do **not** use an Issuer ID. |
| Team | **Users and Access → Integrations → Team Keys → Generate API Key** | Key ID, Issuer ID from the Integrations page, and the downloaded `.p8` file. Choose the App Manager role while creating the key. |

Individual-key tokens use `sub=user`, not an issuer. This server selects that form when `ASC_ISSUER_ID` is empty. See Apple's [token-generation documentation](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests).

### 5. Protect the private key

Apple lets you download the `.p8` private key once. It cannot give you another copy later.

- Keep the `.p8` file outside this repository, even though `*.p8` is ignored here.
- Never commit, share, or put the key in client-side code.
- Restrict its file permissions.
- Revoke the key in App Store Connect if it is lost or exposed.

For example:

```sh
chmod 600 /absolute/private/path/AuthKey_ABC123DEFG.p8
```

## Configure the environment

Set these values in the shell that starts Codex, Claude Code, or your own agent. Use absolute paths because an MCP host may start the binary from another directory.

### Individual key — recommended for personal use

```sh
export ASC_KEY_ID="your-key-id"
unset ASC_ISSUER_ID
export ASC_PRIVATE_KEY_PATH="/absolute/private/path/AuthKey_your-key-id.p8"
export ASC_OPENAPI_SOURCE="/absolute/path/to/ai/api/apple/app-store-connect.openapi.json"
```

### Team key — for shared or unattended automation

```sh
export ASC_KEY_ID="your-key-id"
export ASC_ISSUER_ID="your-team-issuer-id"
export ASC_PRIVATE_KEY_PATH="/absolute/private/path/AuthKey_your-key-id.p8"
export ASC_OPENAPI_SOURCE="/absolute/path/to/ai/api/apple/app-store-connect.openapi.json"
```

| Variable | What it is |
| --- | --- |
| `ASC_KEY_ID` | Your Apple API key ID. Required. |
| `ASC_ISSUER_ID` | Your Apple team Issuer ID. Use it only for a team key. |
| `ASC_PRIVATE_KEY_PATH` | Absolute path to the `.p8` file. Required. |
| `ASC_OPENAPI_SOURCE` | The OpenAPI document path or HTTPS URL. Prefer the checked-in [`../api/apple/app-store-connect.openapi.json`](../api/apple/app-store-connect.openapi.json). The public copy is `https://raw.githubusercontent.com/jamoowen/ai/refs/heads/main/api/apple/app-store-connect.openapi.json`. |

Reads are enabled by default. Changes to your App Store Connect data require separate, deliberate opt-ins:

```sh
export ASC_ALLOW_WRITES=true  # Allows POST and PATCH through asc_write.
export ASC_ALLOW_DELETES=true # Allows DELETE through asc_delete.
```

Set either flag independently, and export it only when you want the MCP to make that kind of change. `ASC_MAX_RESPONSE_BYTES=1048576` is optional; it changes the response-size limit from its default of 1 MiB.

## Build and run

From the repository root, build the local server:

```sh
go build -o bin/appstoreconnect-mcp ./cmd/appstoreconnect-mcp
```

If you run `bin/appstoreconnect-mcp` yourself, it waits for MCP/JSON-RPC messages on standard input. It is not an interactive command. Normally Codex, Claude Code, or your custom agent starts the binary and communicates with it.

## Connect Codex

Codex can start local STDIO MCP servers from `~/.codex/config.toml` or a trusted project's `.codex/config.toml`. This recommended individual-key configuration forwards only the required local secrets. See the official [Codex MCP documentation](https://developers.openai.com/codex/mcp).

```toml
[mcp_servers.app_store_connect]
command = "/absolute/path/to/ai/bin/appstoreconnect-mcp"
env_vars = ["ASC_KEY_ID", "ASC_PRIVATE_KEY_PATH"]

[mcp_servers.app_store_connect.env]
ASC_OPENAPI_SOURCE = "/absolute/path/to/ai/api/apple/app-store-connect.openapi.json"
```

For a team key, add `"ASC_ISSUER_ID"` to `env_vars`. Add write or delete variables only when you intentionally want this server to make those changes.

For example, replace the earlier `env_vars` line in the `[mcp_servers.app_store_connect]` table with this individual-key line to forward the write opt-in to the MCP subprocess:

```toml
env_vars = ["ASC_KEY_ID", "ASC_PRIVATE_KEY_PATH", "ASC_ALLOW_WRITES"]
```

Add `"ASC_ALLOW_DELETES"` separately only when intended. A mutation flag that is exported in your shell must also appear in `env_vars`, or Codex will not pass it to the server.

Verify the registration:

```sh
codex mcp list
```

Inside Codex, use `/mcp` to inspect the connection and its tools.

## Connect Claude Code

Claude Code can register a local STDIO server with `claude mcp add`. This recommended individual-key command keeps the registration local to this project. The one `--env` option forwards all three variables; `--` separates Claude Code options from the server command. See the official [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp).

```sh
claude mcp add \
  --scope local \
  --env ASC_KEY_ID="$ASC_KEY_ID" \
        ASC_PRIVATE_KEY_PATH="$ASC_PRIVATE_KEY_PATH" \
        ASC_OPENAPI_SOURCE="$ASC_OPENAPI_SOURCE" \
  --transport stdio \
  app-store-connect -- \
  /absolute/path/to/ai/bin/appstoreconnect-mcp
```

For a team key, add `ASC_ISSUER_ID="$ASC_ISSUER_ID"` to that same `--env` list. To enable mutations, add these assignments inside the existing single `--env` list, before `--transport stdio`:

```sh
        ASC_ALLOW_WRITES="$ASC_ALLOW_WRITES" \
        ASC_ALLOW_DELETES="$ASC_ALLOW_DELETES" \
```

Add each one only when intended; do not add another `--env`. Even if you add only one flag, keep its trailing `\` because `--transport stdio` follows. Use a broader scope only when you deliberately want the server available outside this project.

Verify the registration:

```sh
claude mcp get app-store-connect
claude mcp list
```

Inside Claude Code, use `/mcp` for connection status.

For a shareable project configuration, a repository-root `.mcp.json` can refer to each developer's environment without committing resolved secrets:

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

Here, a blank `ASC_ISSUER_ID` means an individual key. Project-scoped servers require user approval before use.

## Use the MCP tools

| Tool | Use it for |
| --- | --- |
| `asc_search_operations` | Find an App Store Connect operation from plain-language words and an optional HTTP method. |
| `asc_describe_operation` | See an operation's parameters, request body, and schemas. |
| `asc_read` | Run a GET operation. |
| `asc_write` | Run a POST or PATCH operation when writes are enabled. |
| `asc_delete` | Run a DELETE operation when deletes are enabled. |

Use the tools in this order:

1. Call `asc_search_operations` with what you want to do and, if useful, an HTTP method.
2. Call `asc_describe_operation` for the returned `operationId`.
3. Inspect the parameters and schemas, then call `asc_read`, `asc_write`, or `asc_delete` with that exact ID.

Never invent an operation ID. Inspect schemas before a mutation, and request every page explicitly: the server does not automatically paginate or perform high-level workflows.

For example, search first:

```json
{"query":"list apps","method":"GET","limit":10}
```

Then describe the selected operation:

```json
{"operationId":"apps_getCollection"}
```

Then invoke it. Include only the parameters the description requires:

```json
{
  "operationId": "apps_getCollection",
  "query": {"limit": 5}
}
```

The server validates the path, query parameters, request body, and destination before it sends anything to Apple.

## Use the MCP from a custom agent

If your agent framework already supports MCP, point it at the built binary and pass the same environment variables. If it has its own tool abstraction, use this pattern:

1. Start one MCP session when the agent loop starts, and keep it alive for the loop.
2. Discover the server's tools.
3. Register each tool's name, description, and input schema with your model provider.
4. Send the model's selected tool name and JSON arguments to MCP unchanged.
5. Check the transport/protocol error and the tool result's error indicator (`isError` in MCP, or the equivalent your SDK exposes). Add the returned content to the next model turn.

Keep MCP tool annotations in your own policy. Require explicit authorization or confirmation before `asc_write` or `asc_delete`.

External modules should launch the binary. They cannot import this repository's `internal/appstoreconnectmcp` package because Go's `internal` rule makes it private to this module.

### Go MCP SDK example

This example uses `github.com/modelcontextprotocol/go-sdk v1.8.0` to start the local server and call a discovered tool. Keep the session for the life of the agent loop, then close it.

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
	cmd.Env = os.Environ() // Includes exported ASC_* variables.

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

`session.Tools` handles tool-list pagination. If you need finer control, use `session.ListTools`. For every call, check both the Go `error` and `result.IsError`. The [Go MCP SDK v1.8.0 documentation](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0) explains the protocol and transports.

## Use the Go primitives directly

For a Go-only agent loop, import `github.com/jamoowen/ai/appstoreconnect`. It gives you a catalog, token source, and constrained client without MCP.

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

`Client.Invoke` can return both a response and an error, such as when Apple returns a non-2xx status. Check both. The package limits calls to `ReadOperation`, `WriteOperation`, and `DeleteOperation`, but it does **not** enforce `ASC_ALLOW_WRITES` or `ASC_ALLOW_DELETES`. Your direct agent loop must require authorization and confirmation before `WriteOperation` or `DeleteOperation`.

## Troubleshooting and limits

- **The schema cannot be found:** set `ASC_OPENAPI_SOURCE` to an absolute checked-in path or HTTPS URL. The default is relative to the binary's working directory.
- **Apple rejects authentication:** use `ASC_ISSUER_ID` only for a team key. Leave it unset for an individual key, and check that the `.p8` file matches `ASC_KEY_ID`.
- **A mutation is disabled:** set `ASC_ALLOW_WRITES=true` or `ASC_ALLOW_DELETES=true` in the process that starts the server.
- **Output looks like JSON-RPC:** that is normal MCP protocol traffic. Do not write ordinary logs to standard output; write diagnostics to standard error.
- **A response body is rejected:** the server supports JSON values and text only, not binary response bodies.

The server intentionally does not provide automatic pagination, embeddings or semantic search, high-level workflow tools, binary upload/download support, remote or streamable MCP hosting, or live Apple integration tests.

## References

- [Apple: App Store Connect API setup](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api)
- [Apple: Creating API keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api)
- [Apple: Generating tokens for API requests](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests)
- [OpenAI: Codex MCP](https://developers.openai.com/codex/mcp)
- [Anthropic: Claude Code MCP](https://code.claude.com/docs/en/mcp)
- [Model Context Protocol Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)
