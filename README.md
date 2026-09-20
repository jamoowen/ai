# What is this
1. AI agent building primitives && a coding harness built in GO
2. A way for me to learn more about AI agent architecture
3. A way for me to write some Go code (My day job is TS only)
4. MCP's and tools I have needed

## App Store Connect MCP

`appstoreconnect-mcp` is a local STDIO MCP server for Apple's App Store Connect API. It uses the checked-in OpenAPI specification in `api/apple/app-store-connect.openapi.json` and exposes exactly five tools: `asc_search_operations`, `asc_describe_operation`, `asc_read`, `asc_write`, and `asc_delete`.

The server is deliberately constrained: agents choose an OpenAPI `operationId`, parameters, and JSON body only. It pins the destination to `https://api.appstoreconnect.apple.com/`, resolves the path and method from the spec, validates the request before sending it, and creates the bearer JWT internally. It never gives agents a general HTTP client or credentials.

Build it with:

```sh
go build -o bin/appstoreconnect-mcp ./cmd/appstoreconnect-mcp
```

Create an App Store Connect API key in App Store Connect, store its downloaded `.p8` file somewhere private, and set these environment variables in the MCP host:

```sh
ASC_KEY_ID=your-key-id
ASC_ISSUER_ID=your-issuer-id       # omit for an individual key
ASC_PRIVATE_KEY_PATH=/absolute/path/AuthKey_ABC123.p8
```

`ASC_OPENAPI_SOURCE` is trusted startup configuration and defaults to the checked-in file. It may be a local path or an HTTPS URL, for example:

```sh
ASC_OPENAPI_SOURCE=api/apple/app-store-connect.openapi.json
ASC_OPENAPI_SOURCE=https://raw.githubusercontent.com/jamoowen/ai/refs/heads/main/api/apple/app-store-connect.openapi.json
```

Reads are enabled by default. Writes and deletes need separate explicit opt-ins:

```sh
ASC_ALLOW_WRITES=true
ASC_ALLOW_DELETES=true
ASC_MAX_RESPONSE_BYTES=1048576
```

A Codex configuration should use an explicit schema source as well as an absolute binary path: MCP hosts do not necessarily start in this repository, so the server's relative default may not resolve. Keep secrets in environment variables rather than the configuration file.

```toml
[mcp_servers.app_store_connect]
command = "/absolute/path/to/ai/bin/appstoreconnect-mcp"
env_vars = ["ASC_KEY_ID", "ASC_ISSUER_ID", "ASC_PRIVATE_KEY_PATH"]

[mcp_servers.app_store_connect.env]
ASC_OPENAPI_SOURCE = "/absolute/path/to/ai/api/apple/app-store-connect.openapi.json"
```

v1 intentionally excludes remote/streamable MCP hosting, binary uploads/downloads, automatic pagination, embeddings or semantic search, high-level workflow tools, and live Apple integration tests.

## Some notes
- This codebase (for now) favours simplicity - no gui or tui frameworks to take my mental energy away from the actual agent bits
- This codebase is largely inspired my Pi and a handful of blog articles from Mario like [this one](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/)

## Formatting

Run `make fmt` to format Go files and `make fmt-check` to verify formatting without making changes.
