// Package mcp exposes the eventkit REST API as a Streamable HTTP
// Model Context Protocol server.
//
// The server is a thin translation in front of the REST API
// (`http://127.0.0.1:8765` by default): every MCP tool call becomes one
// HTTP request to the REST surface. The REST layer's defense-in-depth
// (policy gate, post-fetch verify, rate limit, idempotency, audit log)
// is inherited unchanged.
//
// Authentication and remote exposure are deliberately out of scope. The
// MCP server binds 127.0.0.1 by default and serves plain HTTP. Operators
// who want remote access put any tunnel + identity layer of their
// choice in front (Cloudflare Tunnel + Access, Tailscale Funnel, etc.)
// — see docs/operator-playbook.md.
//
// # Tool surface
//
// 24 tools mirroring the REST surface, one per OperationID. Tools that
// only read are tagged with the MCP "readOnlyHint"; tools that delete
// get "destructiveHint"; reversible writes get "idempotentHint". All
// 24 carry "openWorldHint" because they touch macOS state.
//
// # Natural-language dates
//
// Tools that take dates accept either RFC 3339 or a natural-language
// string (parsed via the dateparser package). "tomorrow 2pm" becomes
// the appropriate timestamp in the operator's time zone before being
// forwarded to the REST API.
//
// # User-data framing
//
// Event titles, notes, reminder bodies, and attendee fields are wrapped
// in `<USER_DATA>...</USER_DATA>` delimiters and truncated to 512 chars
// before being returned to the MCP client. This is a defense in depth
// against prompt-injection — the LLM client should treat text inside
// the delimiters as untrusted and not interpret it as instructions.
package mcp
