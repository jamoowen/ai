# App Store Connect MCP user-guide research

Research date: 2026-09-20.

This is a provenance record for [`../../appstoreconnect/README.md`](../../appstoreconnect/README.md), the sole user-facing setup guide. The guide's Apple credential, host-configuration, and SDK statements were checked against these primary sources:

- [Apple: App Store Connect API setup](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api)
- [Apple: Creating API keys](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api)
- [Apple: Generating tokens for API requests](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests)
- [OpenAI: Codex MCP](https://developers.openai.com/codex/mcp)
- [Anthropic: Claude Code MCP](https://code.claude.com/docs/en/mcp)
- [Model Context Protocol Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)
- [Go SDK v1.8.0 `CommandTransport` source](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/cmd.go)

The Go MCP SDK v1.8.0 types and methods used in the guide (`CommandTransport`, `NewClient`, `Connect`, `Tools`, `ListTools`, and `CallTool`) were also verified against the local module source. Repository behavior for environment handling, mutation policy, JWT claims, and the public-versus-`internal` package boundary was verified against the checked-in Go source.
