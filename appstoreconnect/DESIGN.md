# App Store Connect MCP design

This note records the constraints that shape the server. User setup and configuration belong in the [README](README.md).

## Tool boundary

The server exposes six tools: operation search, operation description, schema description, and separate read, write, and delete invocation tools. It does not expose an arbitrary URL, HTTP method, header, or credential interface. The model first discovers an operation in the checked OpenAPI catalog, then inspects the operation and any relevant schemas before invoking it. This keeps every request tied to a reviewed Apple API operation and lets the server enforce the operation class.

The catalog validates the OpenAPI document at startup and rejects missing or duplicate operation IDs. Operation and schema descriptions retain references rather than expanding them, and each serialized description is capped at 32 KiB. Those limits keep tool results bounded and avoid turning descriptions into unbounded document expansion.

## Specification source

With no explicit source, the server fetches the checked-in specification through GitHub's Contents API using its raw media type. It stores one atomically replaced cache record containing the ETag and validated raw specification in the user's cache directory. Later starts send `If-None-Match`; a 304 reuses the validated cache without downloading the document again. A failed, malformed, or oversized refresh falls back to a previously validated cache. If the cache cannot be located or written, a successfully downloaded and validated specification is still used for that process and the server reports a diagnostic on stderr.

`ASC_OPENAPI_SOURCE` deliberately bypasses the default cache. It is a trusted startup override for a local path or HTTPS URL, useful for development or a pinned source. HTTPS sources retain the timeout, HTTPS-only redirect policy, and response-size cap.

## Request and response boundary

All API calls target the fixed App Store Connect HTTPS host. Paths, query parameters, and JSON bodies are constructed from the catalog and validated against its OpenAPI route before transport. The invocation tools restrict methods to GET, POST/PATCH, or DELETE according to their class, so changing an operation ID cannot turn a read tool into a mutation.

Responses have a configurable size limit and only expose JSON values or text. The server preserves safe response metadata needed for diagnosis, while refusing binary response bodies and keeping bounded error detail.

## Credentials and process behavior

The private key remains at a configured filesystem path and JWT signing happens in the server process. Credentials and token values are never tool inputs or tool results. Read access is enabled by default; write and delete capabilities require separate opt-in flags so enabling one does not enable the other.

The server is a local stdio MCP process. Stdout carries only MCP protocol traffic. Startup and cache diagnostics go to stderr.
