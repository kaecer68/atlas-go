package server

import (
	"encoding/json"
	"fmt"
	"slices"
)

const (
	maxElicitSchemaBytes     = 16 * 1024
	maxElicitPropertyCount   = 20
	maxElicitPropertyNameLen = 64
)

func validateElicitSchema(raw map[string]any) error {
	if len(raw) == 0 {
		return fmt.Errorf("schema is empty")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("schema is not valid JSON: %w", err)
	}
	if len(b) > maxElicitSchemaBytes {
		return fmt.Errorf("schema exceeds %d bytes (got %d)", maxElicitSchemaBytes, len(b))
	}
	if hasExternalRef(raw) {
		return fmt.Errorf("schema contains external $ref or $dynamicRef (not allowed in elicitation)")
	}
	props, ok := raw["properties"]
	if ok {
		pm, isMap := props.(map[string]any)
		if !isMap {
			return fmt.Errorf("schema.properties must be an object")
		}
		if len(pm) > maxElicitPropertyCount {
			return fmt.Errorf("schema has %d properties (max %d)", len(pm), maxElicitPropertyCount)
		}
		for name := range pm {
			if len(name) > maxElicitPropertyNameLen {
				return fmt.Errorf("property name exceeds %d chars", maxElicitPropertyNameLen)
			}
		}
	}
	return nil
}

func hasExternalRef(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if k == "$ref" || k == "$dynamicRef" {
				if s, ok := vv.(string); ok && isExternalRefTarget(s) {
					return true
				}
			}
			if hasExternalRef(vv) {
				return true
			}
		}
	case []any:
		if slices.ContainsFunc(t, hasExternalRef) {
			return true
		}
	}
	return false
}

func isExternalRefTarget(s string) bool {
	if len(s) == 0 {
		return false
	}
	if s[0] == '#' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}
