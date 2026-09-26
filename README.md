# What is this
1. AI agent building primitives && a coding harness built in GO
2. A way for me to learn more about AI agent architecture
3. A way for me to write some Go code (My day job is TS only)
4. MCP's and tools I have needed

## App Store Connect MCP

`appstoreconnect-mcp` is a local STDIO MCP server for Apple's App Store Connect API. It keeps credentials and HTTP transport outside the model's control while exposing OpenAPI-backed operations.

See the [App Store Connect MCP guide](appstoreconnect/README.md) for Apple credentials, Codex and Claude Code setup, custom-agent MCP integration, and direct Go usage.

## Google Play Console MCP

See the [Google Play Console MCP guide](googleplay/README.md) for service-account setup and agent MCP configuration.

## Some notes
- This codebase (for now) favours simplicity - no gui or tui frameworks to take my mental energy away from the actual agent bits
- This codebase is largely inspired my Pi and a handful of blog articles from Mario like [this one](https://mariozechner.at/posts/2025-11-30-pi-coding-agent/)

## Formatting

Run `make fmt` to format Go files and `make fmt-check` to verify formatting without making changes.
