// Package tools defines the MCP tool surface that wraps the eventkit
// REST API. Each tool's input/output is a small Go struct with
// json+jsonschema tags; the SDK auto-derives the JSON Schema from those.
//
// The SDK's defaults for ToolAnnotations.DestructiveHint and OpenWorldHint
// are "true" (per MCP spec — assume the worst until told otherwise).
// To mark a tool as non-destructive you must explicitly set
// DestructiveHint to a pointer to false. The helpers in this file keep
// each tool's annotation declaration to one line.
package tools

import (
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// boolPtr is a tiny convenience for the *bool fields in ToolAnnotations.
func boolPtr(b bool) *bool { return &b }

// readOnly produces annotations for a tool that only reads data.
func readOnly() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(true), // touches macOS state
	}
}

// nonDestructiveWrite produces annotations for create-style writes that
// add data but don't remove or mutate existing data.
func nonDestructiveWrite() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(true),
	}
}

// idempotentWrite produces annotations for updates / status changes that
// produce the same final state when retried.
func idempotentWrite() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		IdempotentHint:  true,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(true),
	}
}

// destructive produces annotations for tools that remove data.
// IdempotentHint is true because a second delete of the same id is a no-op.
func destructive() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(true),
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(true),
	}
}
