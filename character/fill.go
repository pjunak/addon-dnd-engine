package character

import "reflect"

// FillEmpty replaces nil slices and maps that the schema requires with empty
// ones, so a value never encodes them as null. Optional (omitempty) fields
// stay nil and are omitted. Pass a pointer.
func FillEmpty(value any) {
	fill(reflect.ValueOf(value))
}

func fill(value reflect.Value) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			fill(value.Elem())
		}
	case reflect.Struct:
		fillStruct(value)
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		for index := range value.Len() {
			element := value.Index(index)
			fillRequired(element)
			fill(element)
		}
	case reflect.Map:
		for entries := value.MapRange(); entries.Next(); {
			element := reflect.New(entries.Value().Type()).Elem()
			element.Set(entries.Value())
			fillRequired(element)
			fill(element)
			value.SetMapIndex(entries.Key(), element)
		}
	}
}

func fillStruct(value reflect.Value) {
	optional := map[string]bool{}
	for _, current := range structFields(value.Type()) {
		optional[current.Name] = current.Optional
	}
	for index := range value.NumField() {
		described := value.Type().Field(index)
		current := value.Field(index)
		if !described.IsExported() || !current.CanSet() {
			continue
		}
		if described.Anonymous {
			fill(current)
			continue
		}
		if !optional[jsonName(described)] {
			fillRequired(current)
		}
		fill(current)
	}
}

func fillRequired(value reflect.Value) {
	switch value.Kind() {
	case reflect.Slice:
		if value.IsNil() && value.Type() != rawMessageType {
			value.Set(reflect.MakeSlice(value.Type(), 0, 0))
		}
	case reflect.Map:
		if value.IsNil() {
			value.Set(reflect.MakeMap(value.Type()))
		}
	}
}

func jsonName(described reflect.StructField) string {
	tag := described.Tag.Get("json")
	for index := range len(tag) {
		if tag[index] == ',' {
			tag = tag[:index]
			break
		}
	}
	if tag == "" {
		return described.Name
	}
	return tag
}
