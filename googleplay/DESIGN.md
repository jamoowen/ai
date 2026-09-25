# Google Play Console MCP design

The server follows the App Store Connect MCP structure: a checked, searchable API catalog, a client whose endpoint and authentication stay outside model inputs, and a small MCP layer with explicit mutation gates.

Google publishes REST Discovery rather than OpenAPI. The catalog recursively walks `resources` and `methods`, retaining every method by its Discovery `id`. It validates method IDs, HTTP verbs and safe relative paths at load time. Discovery metadata is advisory only: requests always target `https://androidpublisher.googleapis.com/` and OAuth tokens always target `https://oauth2.googleapis.com/token`.

Path expansion validates declared parameter patterns, safely escapes normal values, and supports Google reserved templates such as `{+parent}`. The client permits only documented path/query/body fields, refuses media uploads and binary responses, bounds response data, and exposes only safe response metadata. It accepts only a service-account JSON file path and never receives credentials through MCP inputs.

PUT is treated as a write alongside POST and PATCH. Deletes and writes remain disabled until their separate environment variables are explicitly set.
