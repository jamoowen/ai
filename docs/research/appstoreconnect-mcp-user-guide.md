# App Store Connect MCP user-guide research

Research date: 2026-09-20. This is a primary-source note for the dedicated user guide, not the guide itself.

## Apple credentials

Apple must first approve the App Store Connect API for the account. Only the Account Holder can request that access: **Users and Access → Integrations → App Store Connect API → Request Access**. Apple reviews requests case by case. After access is available, an Account Holder or Admin can create a team key under **Users and Access → Integrations → Team Keys** and choose its role. Team keys apply across all apps; they cannot be restricted to selected apps. [Apple: App Store Connect API setup](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api)

Individual keys belong to a user and carry that user's app access and permissions. Apple lists Account Holder, Admin, App Manager, Customer Support, Developer, and Marketing as eligible roles; users can create an individual key by default unless an Account Holder or Admin removes the `Generate Individual API Keys` permission. A user can have only one active individual key. The UI path is **username → Edit Profile → Individual API Key → Generate Key**. [Apple: App Store Connect API setup](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api) Individual keys cannot use Provisioning endpoints, Sales and Finance, or `notaryTool`. [Apple: Creating API Keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api)

For either type, download the `.p8` private key immediately. The download is offered only once and Apple does not retain a copy. Apple explicitly says not to share it, put it in a source repository, or include it in client-side code; revoke it if it is lost or compromised. [Apple: Creating API Keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api)

Credential-to-environment mapping:

| Server setting | Apple value | Where to find it |
| --- | --- | --- |
| `ASC_KEY_ID` | Key ID for the downloaded private key | Team: **Users and Access → Integrations**, in the Active keys table. Individual: the **Individual API Key** section of the user's profile. [Apple: Generating Tokens](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests) |
| `ASC_ISSUER_ID` | Team issuer ID | Near the top of **Users and Access → Integrations**. It is used only for team-key JWTs. [Apple: Generating Tokens](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests) |
| `ASC_PRIVATE_KEY_PATH` | Absolute path to the one-time `.p8` download | User-controlled secure storage; this project reads the file from that path at startup. |

The team/individual distinction is important: Apple's team-key JWT payload uses `iss=<issuer ID>`, while an individual-key payload must omit `iss` and use `sub=user`. [Apple: Generating Tokens](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests) This repository implements that choice from `ASC_ISSUER_ID`: non-empty means `iss`; empty means `sub=user`. See [`token.go`](../../appstoreconnect/token.go#L65-L70). Therefore the guide should say **omit/unset `ASC_ISSUER_ID` for an individual key**, not invent an issuer ID.

Suggested shell setup (values are examples, and the key itself remains in the `.p8` file):

```sh
export ASC_KEY_ID="ABC123DEFG"
export ASC_ISSUER_ID="00000000-0000-0000-0000-000000000000" # team key only
export ASC_PRIVATE_KEY_PATH="/absolute/private/path/AuthKey_ABC123DEFG.p8"
export ASC_OPENAPI_SOURCE="/absolute/path/to/ai/api/apple/app-store-connect.openapi.json"
```

For an individual key, leave `ASC_ISSUER_ID` unset:

```sh
unset ASC_ISSUER_ID
```

## Claude Code: local STDIO MCP configuration

The current official syntax for a local subprocess is `claude mcp add [options] <name> -- <command> [args...]`. `--` is significant: Claude Code parses options before it and passes everything after it to the subprocess unchanged. `--transport stdio` is explicit but optional because STDIO is the default. `--env`/`-e` accepts `KEY=value` settings for the child. [Claude Code: install a local STDIO server](https://code.claude.com/docs/en/mcp#option-3-add-a-local-stdio-server)

A private, current-project registration for a team key can therefore have this shape (place `--transport stdio` between the final `--env` and the server name, as the official parser guidance recommends):

```sh
claude mcp add \
  --scope local \
  --env ASC_KEY_ID="$ASC_KEY_ID" \
        ASC_ISSUER_ID="$ASC_ISSUER_ID" \
        ASC_PRIVATE_KEY_PATH="$ASC_PRIVATE_KEY_PATH" \
        ASC_OPENAPI_SOURCE="$ASC_OPENAPI_SOURCE" \
  --transport stdio app-store-connect -- \
  /absolute/path/to/ai/bin/appstoreconnect-mcp
```

For an individual key, omit the `ASC_ISSUER_ID` `--env` pair. The three Claude Code scopes are: `local` (the default, current project only, stored privately under the project path in `~/.claude.json`), `project` (current project and shared through the repository-root `.mcp.json`), and `user` (all projects for that user, stored in `~/.claude.json`). [Claude Code: MCP installation scopes](https://code.claude.com/docs/en/mcp#mcp-installation-scopes)

For a team-shared `project` configuration, do not commit resolved credential values. Claude Code officially supports `${VAR}` and `${VAR:-default}` expansion in `.mcp.json`, including in `command`, `args`, and `env`. [Claude Code: environment-variable expansion](https://code.claude.com/docs/en/mcp#environment-variable-expansion-in-mcp-json) A shareable shape is:

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

Each developer exports the referenced variables locally. Claude Code prompts for approval before an interactive session first uses a project-scoped server from `.mcp.json`. [Claude Code: project scope](https://code.claude.com/docs/en/mcp#project-scope)

Verification commands are:

```sh
claude mcp get app-store-connect
claude mcp list
```

Inside Claude Code, `/mcp` shows connection status. `claude mcp list` reports states such as Connected, Needs authentication, Failed to connect, or Pending approval; a project server that is pending approval must be reviewed from an interactive `claude` session. [Claude Code: managing servers and server status](https://code.claude.com/docs/en/mcp#managing-your-servers)

## Go SDK v1.8.0: using the MCP from a custom agent

The checked-in dependency is `github.com/modelcontextprotocol/go-sdk v1.8.0`. At that version, the client-side STDIO transport is `mcp.CommandTransport{Command: *exec.Cmd}`; `mcp.NewClient` creates a client, and `client.Connect(ctx, transport, nil)` starts/initializes a `*mcp.ClientSession`. Close the session to close STDIN and terminate the child cleanly. [Go SDK v1.8.0: protocol transports](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/protocol.md) [Go SDK v1.8.0 source: `CommandTransport`](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/cmd.go)

`ClientSession.Tools(ctx, nil)` is the convenient all-pages iterator; `ClientSession.ListTools(ctx, *mcp.ListToolsParams)` is the lower-level paginated method. Invoke a discovered tool with `ClientSession.CallTool(ctx, &mcp.CallToolParams{Name: ..., Arguments: ...})`; arguments may be any JSON-marshalable value. [Go SDK v1.8.0: tools and pagination](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/server.md#tools)

Compilable-shape example:

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
	cmd.Env = os.Environ() // includes the exported ASC_* variables

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "my-agent",
		Version: "v1.0.0",
	}, nil)
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
		// tool.InputSchema is the JSON Schema to expose to the model.
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "asc_search_operations",
		Arguments: map[string]any{
			"query":  "apps",
			"method": "GET",
			"limit":  10,
		},
	})
	if err != nil {
		log.Fatal(err) // transport or MCP protocol failure
	}
	if result.IsError {
		log.Fatalf("tool failed: %#v", result.Content)
	}

	encoded, err := json.MarshalIndent(result.StructuredContent, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(encoded))
}
```

`os.Environ()` makes the current process environment explicit on the child command; an `exec.Cmd` with a nil `Env` also inherits the current process environment. [Go standard library: `exec.Cmd.Env`](https://pkg.go.dev/os/exec#Cmd) The App Store Connect server reads `ASC_OPENAPI_SOURCE`, `ASC_KEY_ID`, `ASC_ISSUER_ID`, and `ASC_PRIVATE_KEY_PATH` from that environment at startup, then reads the private key internally. See [`main.go`](../../cmd/appstoreconnect-mcp/main.go#L14-L31) and [`token.go`](../../appstoreconnect/token.go#L26-L55).

For an LLM agent loop, the SDK stops at the MCP boundary. The host should convert every discovered `mcp.Tool`'s name, description, and `InputSchema` into the model provider's tool format; send those definitions with the model request; forward the model's chosen tool name and JSON arguments unchanged through `CallTool`; check both the Go `error` and `CallToolResult.IsError`; then append the MCP result content to the next model turn. The SDK's official tool docs distinguish protocol failures from tool results and document `StructuredContent`, `Content`, and `IsError`. [Go SDK v1.8.0: tools](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/server.md#tools)

There is no unresolved API-name uncertainty in the snippet: all named types and methods were checked against the repository's local v1.8.0 module source. Provider-specific conversion of MCP schemas/content is intentionally outside this SDK and needs to be shown separately for whichever model API the final guide chooses.

## Repository-specific security and policy facts

- Never commit the `.p8`; this is Apple's explicit security requirement. [Apple: Creating API Keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api) The final guide should also recommend keeping it outside the repository and using an absolute `ASC_PRIVATE_KEY_PATH`.
- The MCP executable accepts credential identifiers and the private-key path only through its process environment, creates JWTs internally, and does not include credentials in any registered tool input. See [`main.go`](../../cmd/appstoreconnect-mcp/main.go#L14-L31), [`token.go`](../../appstoreconnect/token.go#L26-L79), and [`server.go`](../../internal/appstoreconnectmcp/server.go#L27-L45).
- Reads are always enabled. Writes require exactly `ASC_ALLOW_WRITES=true`; deletes independently require exactly `ASC_ALLOW_DELETES=true`. Otherwise the discoverable tools return policy errors before invoking Apple. See [`main.go`](../../cmd/appstoreconnect-mcp/main.go#L31) and [`server.go`](../../internal/appstoreconnectmcp/server.go#L43-L56).
- At the time of this research, the checked-in `.gitignore` did **not** include `*.p8`; the guide implementation added that rule. The guide must not imply that an ignore rule automatically protects a key file.
