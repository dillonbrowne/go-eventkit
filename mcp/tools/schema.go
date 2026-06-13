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
		AdditionalProperties: falseSchema(), // marshals to false
	}
}

// falseSchema is the JSON Schema literal `false` (rejects everything),
// used as additionalProperties to forbid unknown keys.
func falseSchema() *jsonschema.Schema { return &jsonschema.Schema{Not: &jsonschema.Schema{}} }

// strictInput builds the input JSON Schema for a tool input type T and makes
// it strict-compatible for BOTH OpenAI and Anthropic MCP clients:
//
//   - every object carries additionalProperties:false and an explicit
//     properties map (ChatGPT rejects an object schema with no properties);
//   - every OPTIONAL property (one not in the object's required set) is
//     rendered nullable (type:["x","null"]) so an OpenAI-style client that
//     passes an explicit null still validates, while `required` stays minimal
//     so an Anthropic client that omits optionals still passes the go-sdk's
//     required-check (it rejects calls missing a required property).
//
// It reuses the same reflection the SDK applies (jsonschema.For), so field
// descriptions and `jsonschema_enum` struct tags are preserved. Pass the
// result as Tool.InputSchema. It supersedes emptyObjectSchema: an empty input
// struct yields {"type":"object","properties":{},"additionalProperties":false}.
func strictInput[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](&jsonschema.ForOptions{})
	if err != nil {
		panic("tools: building strict input schema: " + err.Error())
	}
	makeStrictCompat(s)
	return s
}

// withEnum constrains a top-level property of an input schema to a fixed set
// of values. jsonschema-go's reflection cannot express an enum via struct
// tags (the `jsonschema` tag is description-only), so enums must be applied
// here, after strictInput. If the property is nullable (optional fields are),
// null is added as a member so the nullable type remains meaningful. Panics
// if field is not a property — catches typos at server start.
func withEnum(s *jsonschema.Schema, field string, values ...any) *jsonschema.Schema {
	prop, ok := s.Properties[field]
	if !ok {
		panic("tools: withEnum: no property " + field)
	}
	prop.Enum = append([]any(nil), values...)
	if schemaIsNullable(prop) {
		prop.Enum = append(prop.Enum, nil)
	}
	return s
}

func schemaIsNullable(s *jsonschema.Schema) bool {
	if s.Type == "null" {
		return true
	}
	for _, t := range s.Types {
		if t == "null" {
			return true
		}
	}
	return false
}

// makeStrictCompat applies the strict-compatible transform described on
// strictInput, recursing into nested objects and array items. Idempotent.
func makeStrictCompat(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if isObjectSchema(s) {
		if s.Properties == nil {
			s.Properties = map[string]*jsonschema.Schema{}
		}
		if s.AdditionalProperties == nil {
			s.AdditionalProperties = falseSchema()
		}
		required := make(map[string]bool, len(s.Required))
		for _, r := range s.Required {
			required[r] = true
		}
		for name, prop := range s.Properties {
			makeStrictCompat(prop) // handle nested schemas first
			if !required[name] {
				makeNullable(prop)
			}
		}
	}
	if s.Items != nil {
		makeStrictCompat(s.Items)
	}
	for _, it := range s.ItemsArray {
		makeStrictCompat(it)
	}
}

func isObjectSchema(s *jsonschema.Schema) bool {
	if s.Type == "object" || s.Properties != nil {
		return true
	}
	for _, t := range s.Types {
		if t == "object" {
			return true
		}
	}
	return false
}

// makeNullable renders a schema as also accepting JSON null. Idempotent. When
// the schema carries an enum, null is added as an explicit member (an enum
// constrains values regardless of the declared type).
func makeNullable(s *jsonschema.Schema) {
	if s == nil || s.Type == "null" {
		return
	}
	for _, t := range s.Types {
		if t == "null" {
			return
		}
	}
	switch {
	case s.Type != "":
		s.Types = []string{s.Type, "null"}
		s.Type = ""
	case len(s.Types) > 0:
		s.Types = append(s.Types, "null")
	}
	if len(s.Enum) > 0 {
		for _, e := range s.Enum {
			if e == nil {
				return
			}
		}
		s.Enum = append(s.Enum, nil)
	}
}
