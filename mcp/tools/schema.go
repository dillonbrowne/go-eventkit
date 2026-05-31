package tools

import "github.com/google/jsonschema-go/jsonschema"

// emptyObjectSchema is the JSON Schema for a tool that takes no arguments:
//
//	{"type":"object","properties":{},"additionalProperties":false}
//
// The go-sdk infers a tool's input schema from its Go input struct via
// reflection. For an empty struct (`struct{}`) it leaves Schema.Properties
// nil, and the jsonschema marshaler omits a nil Properties map — so the wire
// schema becomes `{"type":"object","additionalProperties":false}` with no
// "properties" key at all.
//
// Claude tolerates that. ChatGPT does not: its connector validates every
// tool's input schema far more strictly, and OpenAI's function-calling spec
// shows no-argument functions carrying an explicit empty `properties:{}`.
// A tool whose schema omits "properties" can cause ChatGPT to reject the
// tool (or, in practice, the whole tool list — "connector adds but no tools
// appear"). Passing this schema explicitly via Tool.InputSchema forces the
// marshaler to emit `"properties":{}` (the marshaler renders a non-nil map,
// even when empty — see jsonschema/schema.go MarshalJSON).
func emptyObjectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           map[string]*jsonschema.Schema{},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}, // marshals to false
	}
}
