package rules

import (
	"encoding/json"
	"math"
	"strings"
)

type Object map[string]any

type Records interface {
	Value(kind, id string) (json.RawMessage, bool)
	ValueByName(kind, name string) (json.RawMessage, bool)
	Values(kind string) []json.RawMessage
}

func DecodeObject(body json.RawMessage) (Object, bool) {
	var value Object
	if len(body) == 0 || json.Unmarshal(body, &value) != nil || value == nil {
		return nil, false
	}
	return value, true
}

func recordByID(records Records, kind, id string) Object {
	if records == nil || id == "" {
		return nil
	}
	body, exists := records.Value(kind, id)
	if !exists {
		return nil
	}
	value, _ := DecodeObject(body)
	return value
}

func recordByName(records Records, kind, name string) Object {
	if records == nil || name == "" {
		return nil
	}
	body, exists := records.ValueByName(kind, name)
	if !exists {
		return nil
	}
	value, _ := DecodeObject(body)
	return value
}

func recordList(records Records, kind string) []Object {
	if records == nil {
		return []Object{}
	}
	result := make([]Object, 0)
	for _, body := range records.Values(kind) {
		if value, valid := DecodeObject(body); valid {
			result = append(result, value)
		}
	}
	return result
}

func object(value any) Object {
	switch current := value.(type) {
	case Object:
		return current
	case map[string]any:
		return Object(current)
	default:
		return nil
	}
}

func objects(value any) []Object {
	items, _ := value.([]any)
	result := make([]Object, 0, len(items))
	for _, item := range items {
		if current := object(item); current != nil {
			result = append(result, current)
		}
	}
	return result
}

func values(value any) []any {
	result, _ := value.([]any)
	return result
}

func stringsOf(value any) []string {
	items := values(value)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if current, ok := item.(string); ok {
			result = append(result, current)
		}
	}
	return result
}

func number(value any, fallback float64) float64 {
	var result float64
	switch current := value.(type) {
	case float64:
		result = current
	case float32:
		result = float64(current)
	case int:
		result = float64(current)
	case int64:
		result = float64(current)
	case json.Number:
		parsed, err := current.Float64()
		if err != nil {
			return fallback
		}
		result = parsed
	case string:
		parsed, err := json.Number(strings.TrimSpace(current)).Float64()
		if err != nil {
			return fallback
		}
		result = parsed
	default:
		return fallback
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return fallback
	}
	return result
}

func integer(value any, fallback int) int {
	return int(math.Floor(number(value, float64(fallback))))
}

func text(value any) string {
	result, _ := value.(string)
	return result
}

func truth(value any) bool {
	result, _ := value.(bool)
	return result
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
