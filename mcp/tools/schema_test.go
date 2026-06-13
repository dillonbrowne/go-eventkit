package tools

import (
	"encoding/json"
	"testing"
)

// strictSample exercises every shape strictInput must handle: a required
// field, optional scalars, an enum-candidate field, and an optional slice.
type strictSample struct {
	Req   string   `json:"req" jsonschema:"required field"`
	Opt   string   `json:"opt,omitempty" jsonschema:"optional string"`
	OptN  int      `json:"optN,omitempty"`
	Span  string   `json:"span,omitempty"`
	Items []string `json:"items,omitempty"`
}

func decodeSchema(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode schema: %v (raw: %s)", err, b)
	}
	return m
}

// typeSet returns the JSON-schema "type" as a set, whether it was a string
// or a []string.
func typeSet(prop map[string]any) map[string]bool {
	out := map[string]bool{}
	switch tv := prop["type"].(type) {
	case string:
		out[tv] = true
	case []any:
		for _, x := range tv {
			if s, ok := x.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

func TestStrictInput_NullableOptionalsRequiredMinimal(t *testing.T) {
	m := decodeSchema(t, strictInput[strictSample]())

	if m["type"] != "object" {
		t.Errorf("type = %v, want object", m["type"])
	}
	if m["additionalProperties"] != false {
		t.Errorf("additionalProperties = %v, want false", m["additionalProperties"])
	}

	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing/!object: %v", m["properties"])
	}

	// required is exactly {req}.
	req := map[string]bool{}
	if rs, ok := m["required"].([]any); ok {
		for _, r := range rs {
			req[r.(string)] = true
		}
	}
	if len(req) != 1 || !req["req"] {
		t.Errorf("required = %v, want [req]", m["required"])
	}

	// Required field: single type, not nullable.
	if ts := typeSet(props["req"].(map[string]any)); !ts["string"] || ts["null"] {
		t.Errorf("req type = %v, want {string} (not nullable)", ts)
	}

	// Optional scalars: nullable.
	for _, name := range []string{"opt", "optN", "items"} {
		ts := typeSet(props[name].(map[string]any))
		if !ts["null"] {
			t.Errorf("optional %q type = %v, want it to include null", name, ts)
		}
	}

	// Description preserved on opt.
	if props["opt"].(map[string]any)["description"] != "optional string" {
		t.Errorf("opt description not preserved: %v", props["opt"])
	}
}

func TestWithEnum_NullableOptionalGetsNullMember(t *testing.T) {
	// span is optional → strictInput makes it nullable → withEnum must add
	// null as an enum member so the nullable type stays meaningful.
	s := withEnum(strictInput[strictSample](), "span", "this", "future")
	m := decodeSchema(t, s)
	span := m["properties"].(map[string]any)["span"].(map[string]any)
	enum, _ := span["enum"].([]any)
	var hasThis, hasFuture, hasNull bool
	for _, e := range enum {
		switch e {
		case "this":
			hasThis = true
		case "future":
			hasFuture = true
		case nil:
			hasNull = true
		}
	}
	if !hasThis || !hasFuture {
		t.Errorf("span enum lost values: %v", enum)
	}
	if !hasNull {
		t.Errorf("optional span enum should include null: %v", enum)
	}
}

func TestWithEnum_UnknownFieldPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("withEnum on a missing field should panic")
		}
	}()
	withEnum(strictInput[strictSample](), "nope", "x")
}

func TestStrictInput_EmptyStructHasPropertiesObject(t *testing.T) {
	type noArgs struct{}
	m := decodeSchema(t, strictInput[noArgs]())
	if m["type"] != "object" {
		t.Errorf("type = %v, want object", m["type"])
	}
	if _, ok := m["properties"]; !ok {
		t.Errorf("empty-struct schema must still carry a properties object: %v", m)
	}
	if m["additionalProperties"] != false {
		t.Errorf("additionalProperties = %v, want false", m["additionalProperties"])
	}
}

func TestMakeStrictCompat_Idempotent(t *testing.T) {
	// Running the transform twice must not double-add "null" to type sets or
	// to enum members.
	once := withEnum(strictInput[strictSample](), "span", "this", "future")
	twice := withEnum(strictInput[strictSample](), "span", "this", "future")
	makeStrictCompat(twice) // second pass

	m1 := decodeSchema(t, once)["properties"].(map[string]any)
	m2 := decodeSchema(t, twice)["properties"].(map[string]any)

	if len(typeSet(m1["opt"].(map[string]any))) != len(typeSet(m2["opt"].(map[string]any))) {
		t.Errorf("type set not idempotent: %v vs %v", m1["opt"], m2["opt"])
	}
	e1, _ := m1["span"].(map[string]any)["enum"].([]any)
	e2, _ := m2["span"].(map[string]any)["enum"].([]any)
	if len(e1) != len(e2) {
		t.Errorf("enum not idempotent: %v vs %v", e1, e2)
	}
}
