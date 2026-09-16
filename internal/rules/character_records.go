package rules

import (
	"encoding/json"
	"sort"
)

// A character evaluation owns this cache. Source identity and policy are supplied
// by its provider snapshot; no decoded values survive into a later evaluation.
type characterRecordSnapshot struct {
	source  Records
	objects map[string]Object
	lists   map[string][]Object
}

func newCharacterRecordSnapshot(source Records) *characterRecordSnapshot {
	return &characterRecordSnapshot{source: source, objects: map[string]Object{}, lists: map[string][]Object{}}
}
func (records *characterRecordSnapshot) Value(kind, id string) (json.RawMessage, bool) {
	if records.source == nil {
		return nil, false
	}
	return records.source.Value(kind, id)
}
func (records *characterRecordSnapshot) ValueByName(string, string) (json.RawMessage, bool) {
	return nil, false
}
func (records *characterRecordSnapshot) Values(kind string) []json.RawMessage {
	if records.source == nil {
		return nil
	}
	return records.source.Values(kind)
}
func (records *characterRecordSnapshot) Provenance(kind, id string) SourceIdentity {
	if source, ok := records.source.(interface {
		Provenance(string, string) SourceIdentity
	}); ok {
		return source.Provenance(kind, id)
	}
	return SourceIdentity{}
}
func (records *characterRecordSnapshot) recordObject(kind, id string) Object {
	key := kind + ":" + id
	value, loaded := records.objects[key]
	if !loaded {
		body, _ := records.Value(kind, id)
		value, _ = DecodeObject(body)
		records.objects[key] = value
	}
	if value == nil {
		return nil
	}
	return copyRecordObject(value)
}
func (records *characterRecordSnapshot) recordCatalog(kind string) []Object {
	list, loaded := records.lists[kind]
	if !loaded {
		list = []Object{}
		for _, body := range records.Values(kind) {
			if value, valid := DecodeObject(body); valid {
				list = append(list, value)
			}
		}
		// Provider enumeration order is not an authored decision order.
		sort.Slice(list, func(i, j int) bool { return text(list[i]["id"]) < text(list[j]["id"]) })
		records.lists[kind] = list
	}
	return append([]Object(nil), list...)
}

// Individual lookups may be modified by calculations, so they stay detached.
func copyRecordObject(value Object) Object {
	result := make(Object, len(value))
	for key, child := range value {
		result[key] = copyRecordValue(child)
	}
	return result
}
func copyRecordValue(value any) any {
	switch value := value.(type) {
	case Object:
		return copyRecordObject(value)
	case map[string]any:
		return map[string]any(copyRecordObject(Object(value)))
	case []any:
		result := make([]any, len(value))
		for index, child := range value {
			result[index] = copyRecordValue(child)
		}
		return result
	default:
		return value
	}
}
