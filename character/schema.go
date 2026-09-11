package character

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Schema derives the wire structure from the public DTOs. Package tooling uses
// it for both provider and consumer schemas; neither consumer copies the model.
func Schema(value any) map[string]any { return schemaType(reflect.TypeOf(value)) }
func schemaType(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		return schemaType(t.Elem())
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return map[string]any{"oneOf": []any{map[string]any{"type": "string", "maxLength": 300}, map[string]any{"type": "object", "maxProperties": 6, "additionalProperties": map[string]any{"type": "integer", "minimum": 0, "maximum": 30}}}}
	}
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")
			if tag[0] == "-" {
				continue
			}
			name := tag[0]
			if name == "" {
				name = f.Name
			}
			properties[name] = schemaType(f.Type)
			if len(tag) == 1 {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	case reflect.String:
		return map[string]any{"type": "string", "maxLength": 30000}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer", "minimum": -1000000, "maximum": 1000000}
	case reflect.Float64:
		return map[string]any{"type": "number", "minimum": -1000000000, "maximum": 1000000000}
	case reflect.Slice:
		return map[string]any{"type": []string{"array", "null"}, "maxItems": 500, "items": schemaType(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": []string{"object", "null"}, "maxProperties": 1000, "additionalProperties": schemaType(t.Elem())}
	default:
		return map[string]any{}
	}
}
