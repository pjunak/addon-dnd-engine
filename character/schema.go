package character

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// Enum is a closed string set; generators emit its values.
type Enum interface{ EnumValues() []string }

// Union is a tagged union kept in one struct: the tag field's value selects
// which other fields a value carries. Generators emit one closed object per
// variant; the type's own JSON methods enforce the same shape.
type Union interface {
	UnionVariants() (tag string, variants map[string][]string)
}

// CustomJSON is a type with its own JSON form.
type CustomJSON interface {
	JSONSchema() map[string]any
	TypeScriptType() string
}

var (
	enumType       = reflect.TypeFor[Enum]()
	unionType      = reflect.TypeFor[Union]()
	customJSONType = reflect.TypeFor[CustomJSON]()
	rawMessageType = reflect.TypeFor[json.RawMessage]()
)

// field is one JSON property of a struct, with embedded structs inlined.
// Optional properties may be absent and are never null; nullable ones are
// always present and may be null. Slices and maps are never null.
type field struct {
	Name     string
	Type     reflect.Type
	Optional bool
	Nullable bool
}

func structFields(t reflect.Type) []field {
	result := []field{}
	for index := range t.NumField() {
		current := t.Field(index)
		if !current.IsExported() {
			continue
		}
		tag := strings.Split(current.Tag.Get("json"), ",")
		if tag[0] == "-" {
			continue
		}
		if current.Anonymous && tag[0] == "" {
			embedded, optional := current.Type, false
			if embedded.Kind() == reflect.Pointer {
				embedded, optional = embedded.Elem(), true
			}
			for _, inner := range structFields(embedded) {
				inner.Optional = inner.Optional || optional
				result = append(result, inner)
			}
			continue
		}
		name := tag[0]
		if name == "" {
			name = current.Name
		}
		omit := slices.Contains(tag[1:], "omitempty") || slices.Contains(tag[1:], "omitzero")
		result = append(result, field{Name: name, Type: current.Type, Optional: omit, Nullable: !omit && current.Type.Kind() == reflect.Pointer})
	}
	return result
}

func implements(t, contract reflect.Type) bool {
	return t.Implements(contract) || reflect.PointerTo(t).Implements(contract)
}

func instance[T any](t reflect.Type) T {
	if t.Implements(reflect.TypeFor[T]()) {
		return reflect.Zero(t).Interface().(T)
	}
	return reflect.New(t).Interface().(T)
}

// Schema derives the JSON Schema of a public type. Package tooling uses it for
// provider and consumer schemas; neither consumer copies the model. Sizes are
// bounded by the request limit, not by the schema, so record data the rules
// echo can never make a response invalid. Named structs are emitted once under
// $defs and referenced, which also allows recursive types.
func Schema(value any) map[string]any {
	generator := &schemaGenerator{defs: map[string]any{}, owners: map[string]reflect.Type{}}
	root := reflect.TypeOf(value)
	for root.Kind() == reflect.Pointer {
		root = root.Elem()
	}
	schema := generator.inline(root)
	if len(generator.defs) > 0 {
		schema["$defs"] = generator.defs
	}
	return schema
}

type schemaGenerator struct {
	defs   map[string]any
	owners map[string]reflect.Type
}

func (generator *schemaGenerator) schemaType(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		return generator.schemaType(t.Elem())
	}
	if t.Kind() == reflect.Struct && t.Name() != "" && !implements(t, customJSONType) {
		name := t.Name()
		if owner, seen := generator.owners[name]; seen {
			if owner != t {
				panic("two types are named " + name + ": " + owner.PkgPath() + " and " + t.PkgPath())
			}
		} else {
			generator.owners[name] = t
			generator.defs[name] = map[string]any{}
			generator.defs[name] = generator.inline(t)
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	}
	return generator.inline(t)
}

func (generator *schemaGenerator) inline(t reflect.Type) map[string]any {
	switch {
	case implements(t, customJSONType):
		return instance[CustomJSON](t).JSONSchema()
	case implements(t, unionType):
		return generator.unionSchema(t)
	case implements(t, enumType):
		return map[string]any{"type": "string", "enum": instance[Enum](t).EnumValues()}
	case t == rawMessageType:
		// The only raw value in the model is a build choice: an option ID or
		// an ability assignment.
		return map[string]any{"oneOf": []any{map[string]any{"type": "string", "maxLength": 300}, map[string]any{"type": "object", "maxProperties": 6, "additionalProperties": map[string]any{"type": "integer", "minimum": 0, "maximum": 30}}}}
	}
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for _, current := range structFields(t) {
			properties[current.Name] = generator.propertySchema(current)
			if !current.Optional {
				required = append(required, current.Name)
			}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": generator.schemaType(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": generator.schemaType(t.Elem())}
	default:
		// Display values (explanations and differences) are any JSON value.
		return map[string]any{}
	}
}

func (generator *schemaGenerator) propertySchema(current field) map[string]any {
	schema := generator.schemaType(current.Type)
	if current.Nullable {
		return map[string]any{"oneOf": []any{schema, map[string]any{"type": "null"}}}
	}
	return schema
}

func (generator *schemaGenerator) unionSchema(t reflect.Type) map[string]any {
	tag, variants := instance[Union](t).UnionVariants()
	properties := map[string]field{}
	for _, current := range structFields(t) {
		properties[current.Name] = current
	}
	names := make([]string, 0, len(variants))
	for name := range variants {
		names = append(names, name)
	}
	sort.Strings(names)
	options := []any{}
	for _, name := range names {
		variant := map[string]any{tag: map[string]any{"const": name}}
		required := []string{tag}
		for _, property := range variants[name] {
			variant[property] = generator.schemaType(properties[property].Type)
			required = append(required, property)
		}
		options = append(options, map[string]any{"type": "object", "additionalProperties": false, "properties": variant, "required": required})
	}
	return map[string]any{"oneOf": options}
}
