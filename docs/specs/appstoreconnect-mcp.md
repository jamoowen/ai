# App Store Connect MCP v1

This implementation exposes the checked-in App Store Connect OpenAPI document through a local, STDIO MCP server. It has five tools: search, describe, read, write, and delete. Agents select an `operationId`; they never control a URL, HTTP method, headers, or authentication.

The reusable `appstoreconnect` package loads and validates the specification, indexes operations, validates constructed requests, pins requests to Apple's API host, signs short-lived ES256 team or individual JWTs, and bounds responses. The MCP adapter requires search before use by instruction, makes reads discoverable, and keeps writes/deletes independently opt-in.

Acceptance criteria: a host discovers exactly five tools over STDIO; invalid requests cannot reach transport; only GET may be read, POST/PATCH written, and DELETE deleted; OpenAPI selection is trusted startup configuration; JWTs are short-lived and do not escape the process; and setup documentation is sufficient for Codex without putting secrets in prompts or config files.
