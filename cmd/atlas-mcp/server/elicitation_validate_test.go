package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidateElicitSchema_Valid(t *testing.T) {
	cases := []map[string]any{
		{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}},
		{"type": "object", "properties": map[string]any{"age": map[string]any{"type": "integer"}}},
		{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "boolean"}}},
		{"type": "object"},
		{"type": "object", "properties": map[string]any{}},
		{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}, "$ref": "#/definitions/foo"},
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("valid-%d", i), func(t *testing.T) {
			if err := validateElicitSchema(c); err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestValidateElicitSchema_RejectsEmpty(t *testing.T) {
	if err := validateElicitSchema(map[string]any{}); err == nil {
		t.Fatal("empty schema must be rejected")
	}
	if err := validateElicitSchema(nil); err == nil {
		t.Fatal("nil schema must be rejected")
	}
}

func TestValidateElicitSchema_RejectsExternalRef(t *testing.T) {
	cases := []map[string]any{
		{"type": "object", "$ref": "http://internal.svc/secrets"},
		{"type": "object", "$ref": "https://evil.example.com/"},
		{"type": "object", "$ref": "file:///etc/passwd"},
		{"type": "object", "properties": map[string]any{"x": map[string]any{"$ref": "http://evil/"}}},
		{"type": "object", "$dynamicRef": "https://evil/"},
	}
	for i, c := range cases {
		t.Run(fmt.Sprintf("external-ref-%d", i), func(t *testing.T) {
			err := validateElicitSchema(c)
			if err == nil {
				t.Fatal("external $ref must be rejected")
			}
			if !strings.Contains(err.Error(), "external") {
				t.Errorf("error should mention external, got: %v", err)
			}
		})
	}
}

func TestValidateElicitSchema_AcceptsLocalRef(t *testing.T) {
	schemas := []map[string]any{
		{"type": "object", "$ref": "#/definitions/Foo"},
		{"type": "object", "properties": map[string]any{"x": map[string]any{"$ref": "#/definitions/X"}}},
	}
	for i, s := range schemas {
		t.Run(fmt.Sprintf("local-ref-%d", i), func(t *testing.T) {
			if err := validateElicitSchema(s); err != nil {
				t.Errorf("local $ref must be allowed, got: %v", err)
			}
		})
	}
}

func TestValidateElicitSchema_RejectsTooManyProperties(t *testing.T) {
	props := make(map[string]any, maxElicitPropertyCount+1)
	for i := 0; i <= maxElicitPropertyCount; i++ {
		props[fmt.Sprintf("p%d", i)] = map[string]any{"type": "string"}
	}
	schema := map[string]any{"type": "object", "properties": props}
	err := validateElicitSchema(schema)
	if err == nil {
		t.Fatal("too many properties must be rejected")
	}
	if !strings.Contains(err.Error(), "properties") {
		t.Errorf("error should mention properties, got: %v", err)
	}
}

func TestValidateElicitSchema_RejectsLongPropertyName(t *testing.T) {
	longName := strings.Repeat("a", maxElicitPropertyNameLen+1)
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{longName: map[string]any{"type": "string"}},
	}
	err := validateElicitSchema(schema)
	if err == nil {
		t.Fatal("long property name must be rejected")
	}
	if !strings.Contains(err.Error(), "property name") {
		t.Errorf("error should mention property name, got: %v", err)
	}
}

func TestValidateElicitSchema_RejectsNonMapProperties(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": []any{"not", "a", "map"},
	}
	if err := validateElicitSchema(schema); err == nil {
		t.Fatal("properties as array must be rejected")
	}
}

func TestValidateElicitSchema_RejectsOversized(t *testing.T) {
	bigValue := strings.Repeat("a", maxElicitSchemaBytes)
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{"type": "string", "description": bigValue}},
	}
	err := validateElicitSchema(schema)
	if err == nil {
		t.Fatal("oversized schema must be rejected")
	}
	if !strings.Contains(err.Error(), "bytes") {
		t.Errorf("error should mention bytes, got: %v", err)
	}
}

func TestIsExternalRefTarget(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"#/definitions/Foo", false},
		{"", false},
		{"http://example.com", true},
		{"https://example.com", true},
		{"file:///etc/passwd", true},
		{"urn:isbn:1234", true},
		{"relative/path", false},
		{"/absolute/path", false},
	}
	for _, c := range cases {
		if got := isExternalRefTarget(c.in); got != c.want {
			t.Errorf("isExternalRefTarget(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidateElicitSchema_RealisticMCPForm(t *testing.T) {
	// This is the exact shape that a real MCP elicitation form would use.
	b, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string", "title": "Full Name", "minLength": 1},
			"email": map[string]any{"type": "string", "format": "email"},
			"age":   map[string]any{"type": "integer", "minimum": 0, "maximum": 150},
		},
		"required": []any{"name", "email"},
	})
	var schema map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatal(err)
	}
	if err := validateElicitSchema(schema); err != nil {
		t.Errorf("realistic form must pass, got: %v", err)
	}
}
