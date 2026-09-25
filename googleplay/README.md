# Google Play Console MCP

`googleplay-mcp` gives a local AI agent constrained access to the Google Play Developer API. It reads Google's Discovery document, fixes API and token hosts, and keeps the service-account key outside model inputs. See [DESIGN.md](DESIGN.md) for the security invariants.

Use an existing agent through the setup prompt below, or import the public `github.com/jamoowen/ai/googleplay` Go package for the catalog, service-account token source, and constrained client.

## Set up an existing AI agent

First create and authorize a service account:

1. In Google Cloud, create or select a project and enable **Google Play Android Developer API**.
2. Open **IAM & Admin → Service Accounts**, create or select the account, then choose **Keys → Add key → Create new key → JSON** and download the key.
3. In Play Console, invite the service-account email through **Users and permissions**, with access to the intended developer account and app. For an initial review check, account-level **View app information and download bulk reports (read-only)** or app-level **View app information (read-only)** is enough; **Reply to reviews** is unnecessary.

Keep the key outside this repository and never paste its contents into a prompt, config, log, or commit.

```sh
mkdir -p ~/.secrets/googleplay
mv ~/Downloads/service-account.json ~/.secrets/googleplay/
chmod 600 ~/.secrets/googleplay/service-account.json
```

With Go installed, open this repository in Codex, Claude Code, or another agent. Replace the placeholders and send:

```text
Set up this repository's Google Play Console MCP server for me.

MCP client: <Codex / Claude Code / another local MCP client>
Service account key path: <absolute path to downloaded JSON key>
Known Android package name for a read-only check: <package name>

Inspect the repository and my client's existing MCP configuration. Use its
current supported configuration format and current official docs; do not guess.

Build from the repository root:
go build -o bin/googleplay-mcp ./cmd/googleplay-mcp

Register that absolute binary path as a persistent local stdio MCP server.
Update any existing Google Play registration instead of adding a duplicate,
and preserve unrelated settings.

Persist GP_SERVICE_ACCOUNT_KEY_PATH using the absolute key path. Keep
GP_ALLOW_WRITES and GP_ALLOW_DELETES disabled, including inherited or existing
opt-ins. Do not set either to true.

Confirm the key is outside the repository and chmod 600 on macOS/Linux. Do
not open, print, copy into configuration, log, or commit its contents.

Verify MCP initialization and all six gp_* tools. Search for
androidpublisher.reviews.list, describe its operation and relevant schema, then
make one read-only gp_read reviews.list request using the supplied package name.
Also search for androidpublisher.applications.tracks.releases.list and make one
read-only gp_read request with path parameter
parent=applications/<package-name>/tracks/production to check the latest
production releases. Report draft releases separately from published release
states. Do not modify Google Play data.

Show relevant configuration changes and test results without exposing secrets
or unrelated settings. Explain any required client restart and checks that
could not run.
```

The server is read-only by default. Set `GP_ALLOW_WRITES=true` only for POST, PUT, and PATCH, and `GP_ALLOW_DELETES=true` separately for DELETE; reconnect after changing either value.

## Go package and MCP route

Import `github.com/jamoowen/ai/googleplay` to load a catalog, create `NewServiceAccountTokenSource`, and invoke a fixed `ReadOperation`, `WriteOperation`, or `DeleteOperation`. Direct callers own mutation authorization. For model-facing use, run the `googleplay-mcp` binary through a stdio MCP client; external Go modules should use this public package or the binary instead of `internal/googleplaymcp`.

## MCP reference

| Tool | Purpose |
| --- | --- |
| `gp_search_operations` | Find operations by ID, path, description, and method. |
| `gp_describe_operation` | Inspect a method, parameters, request, and response. |
| `gp_describe_schema` | Inspect a named Discovery schema. |
| `gp_read` | Execute documented GET operations. |
| `gp_write` | Execute POST, PUT, and PATCH when enabled. |
| `gp_delete` | Execute DELETE when enabled. |

Use **search → describe operation → describe relevant schemas → execute**. Description output is capped at 32 KiB.

| Variable | Value |
| --- | --- |
| `GP_SERVICE_ACCOUNT_KEY_PATH` | Required absolute JSON key path. |
| `GP_DISCOVERY_SOURCE` | Optional local path or HTTPS URL. Empty conditionally refreshes the official Google endpoint and uses a validated ETag cache. |
| `GP_MAX_RESPONSE_BYTES` | Maximum response size; default 1 MiB. |
| `GP_ALLOW_WRITES` | Exactly `true` enables POST, PUT, and PATCH. |
| `GP_ALLOW_DELETES` | Exactly `true` enables DELETE. |

`api/google/androidpublisher.v3.discovery.json` is a checked-in compatibility snapshot and optional offline `GP_DISCOVERY_SOURCE`. Google can change the live Discovery document, so update this snapshot deliberately and run the test suite.

The API cannot list every app an account can access, so provide a known package for `reviews.list`. The server is local stdio only, does not paginate automatically, and rejects media upload methods and binary responses. It never permits arbitrary URLs, methods, headers, or token endpoints. MCP traffic uses stdout; diagnostics use stderr.

[Google Play API getting started](https://developers.google.com/android-publisher/getting_started) · [Play Console permissions](https://support.google.com/googleplay/android-developer/answer/10019561) · [Create keys](https://cloud.google.com/iam/docs/keys-create-delete) · [Codex MCP](https://developers.openai.com/codex/mcp) · [Claude Code MCP](https://code.claude.com/docs/en/mcp)
