package character

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// TypeScript declares the given types and everything they reference, sorted
// by name. It reads the same fields as Schema, so the two always agree:
// optional properties are `name?: T`, nullable ones `name: T | null`, enums
// are string-literal unions and tagged unions are unions of object types.
func TypeScript(values ...any) string {
	declarations := map[string]string{}
	owners := map[string]reflect.Type{}
	var emit func(reflect.Type) string
	declare := func(t reflect.Type, body func() string) string {
		name := t.Name()
		if owner, seen := owners[name]; seen {
			if owner != t {
				panic(fmt.Sprintf("two types are named %s: %s and %s", name, owner.PkgPath(), t.PkgPath()))
			}
			return name
		}
		owners[name] = t
		declarations[name] = ""
		declarations[name] = body()
		return name
	}
	property := func(current field) string {
		optional, kind := "", emit(current.Type)
		if current.Optional {
			optional = "?"
		}
		if current.Nullable {
			kind += " | null"
		}
		return fmt.Sprintf("%s%s: %s;", current.Name, optional, kind)
	}
	emit = func(t reflect.Type) string {
		if t.Kind() == reflect.Pointer {
			return emit(t.Elem())
		}
		switch {
		case t == reflect.TypeFor[time.Time]():
			return "string"
		case implements(t, customJSONType):
			return instance[CustomJSON](t).TypeScriptType()
		case implements(t, unionType):
			return declare(t, func() string {
				tag, variants := instance[Union](t).UnionVariants()
				fields := map[string]field{}
				for _, current := range structFields(t) {
					fields[current.Name] = current
				}
				names := make([]string, 0, len(variants))
				for name := range variants {
					names = append(names, name)
				}
				sort.Strings(names)
				options := []string{}
				for _, name := range names {
					members := []string{fmt.Sprintf("%s: %q;", tag, name)}
					for _, member := range variants[name] {
						members = append(members, fmt.Sprintf("%s: %s;", member, emit(fields[member].Type)))
					}
					options = append(options, "  | { "+strings.Join(members, " ")+" }")
				}
				return "export type " + t.Name() + " =\n" + strings.Join(options, "\n") + ";"
			})
		case implements(t, enumType):
			return declare(t, func() string {
				quoted := []string{}
				for _, value := range instance[Enum](t).EnumValues() {
					quoted = append(quoted, fmt.Sprintf("%q", value))
				}
				return "export type " + t.Name() + " = " + strings.Join(quoted, " | ") + ";"
			})
		case t == rawMessageType:
			return "unknown"
		}
		switch t.Kind() {
		case reflect.String:
			return "string"
		case reflect.Bool:
			return "boolean"
		case reflect.Int, reflect.Int64, reflect.Float64:
			return "number"
		case reflect.Slice:
			return "Array<" + emit(t.Elem()) + ">"
		case reflect.Map:
			return "Record<string, " + emit(t.Elem()) + ">"
		case reflect.Struct:
			if t.Name() == "" {
				members := []string{}
				for _, current := range structFields(t) {
					members = append(members, property(current))
				}
				return "{ " + strings.Join(members, " ") + " }"
			}
			return declare(t, func() string {
				lines := []string{"export interface " + t.Name() + " {"}
				for _, current := range structFields(t) {
					lines = append(lines, "  "+property(current))
				}
				return strings.Join(append(lines, "}"), "\n")
			})
		}
		return "unknown"
	}
	for _, value := range values {
		emit(reflect.TypeOf(value))
	}
	names := make([]string, 0, len(declarations))
	for name := range declarations {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, declarations[name])
	}
	return strings.Join(parts, "\n\n") + "\n"
}
