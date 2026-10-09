package rules

import (
	"encoding/json"
	"reflect"
)

// copyJSONValue deep-copies the generic values rules data is decoded into.
// Other types keep the JSON round trip that detaches them as generic values,
// which callers read through object, values and number.
func copyJSONValue(value any) any {
	switch current := value.(type) {
	case nil, string, bool, float64, int, int64, json.Number:
		return current
	case Object:
		return copyJSONObject(current)
	case map[string]any:
		return map[string]any(copyJSONObject(Object(current)))
	case []any:
		result := make([]any, len(current))
		for index, item := range current {
			result[index] = copyJSONValue(item)
		}
		return result
	case []Object:
		result := make([]any, len(current))
		for index, item := range current {
			result[index] = copyJSONObject(item)
		}
		return result
	case []string:
		return anyStrings(current)
	default:
		body, err := json.Marshal(current)
		if err != nil {
			return nil
		}
		var result any
		if json.Unmarshal(body, &result) != nil {
			return nil
		}
		return result
	}
}

func copyJSONObject(source Object) Object {
	result := make(Object, len(source))
	for key, value := range source {
		result[key] = copyJSONValue(value)
	}
	return result
}

// deepCopy copies maps, slices, pointers and structs recursively, keeping
// every Go type. It detaches typed model values without a JSON round trip.
func deepCopy[T any](value T) T {
	return copyReflect(reflect.ValueOf(&value).Elem()).Interface().(T)
}

func copyReflect(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type()).Elem()
		result.Set(copyReflect(value.Elem()))
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(copyReflect(value.Elem()))
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		for entries := value.MapRange(); entries.Next(); {
			result.SetMapIndex(entries.Key(), copyReflect(entries.Value()))
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		if value.Type().Elem().Kind() == reflect.Uint8 {
			reflect.Copy(result, value)
			return result
		}
		for index := range value.Len() {
			result.Index(index).Set(copyReflect(value.Index(index)))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for index := range value.Len() {
			result.Index(index).Set(copyReflect(value.Index(index)))
		}
		return result
	case reflect.Struct:
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for index := range value.NumField() {
			if result.Field(index).CanSet() {
				result.Field(index).Set(copyReflect(value.Field(index)))
			}
		}
		return result
	default:
		return value
	}
}
