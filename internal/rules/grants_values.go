package rules

import (
	"encoding/json"
	"errors"
)

// The grants vocabulary has a few fields that take one of two JSON forms.
// Each type accepts only its forms and describes them to the generators.

// TextOrList is one string or a list of strings.
type TextOrList struct{}

func (*TextOrList) UnmarshalJSON(body []byte) error {
	var text string
	var list []string
	if json.Unmarshal(body, &text) != nil && json.Unmarshal(body, &list) != nil {
		return errors.New("must be a string or a list of strings")
	}
	return nil
}

func (TextOrList) JSONSchema() map[string]any {
	return map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
}
func (TextOrList) TypeScriptType() string { return "string | Array<string>" }

// TextOrNumber is a string or a number.
type TextOrNumber struct{}

func (*TextOrNumber) UnmarshalJSON(body []byte) error {
	var text string
	var number float64
	if json.Unmarshal(body, &text) != nil && json.Unmarshal(body, &number) != nil {
		return errors.New("must be a string or a number")
	}
	return nil
}

func (TextOrNumber) JSONSchema() map[string]any {
	return map[string]any{"type": []string{"string", "number"}}
}
func (TextOrNumber) TypeScriptType() string { return "string | number" }

// SpeedValue is a speed in the distance unit, or "speed" for the walking speed.
type SpeedValue struct{}

func (*SpeedValue) UnmarshalJSON(body []byte) error {
	var number float64
	if string(body) != `"speed"` && json.Unmarshal(body, &number) != nil {
		return errors.New(`must be a number or "speed"`)
	}
	return nil
}

func (SpeedValue) JSONSchema() map[string]any {
	return map[string]any{"oneOf": []any{map[string]any{"type": "number"}, map[string]any{"const": "speed"}}}
}
func (SpeedValue) TypeScriptType() string { return `number | "speed"` }

// SpellLevelLimit is a spell level, or "class" for the highest level the
// granting class can cast.
type SpellLevelLimit struct{}

func (*SpellLevelLimit) UnmarshalJSON(body []byte) error {
	var number int
	if string(body) != `"class"` && json.Unmarshal(body, &number) != nil {
		return errors.New(`must be a whole number or "class"`)
	}
	return nil
}

func (SpellLevelLimit) JSONSchema() map[string]any {
	return map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"const": "class"}}}
}
func (SpellLevelLimit) TypeScriptType() string { return `number | "class"` }

// ResourceRef is a resource key, or {key, cost} when one use spends more.
type ResourceRef struct{}

func (*ResourceRef) UnmarshalJSON(body []byte) error {
	var key string
	if json.Unmarshal(body, &key) == nil {
		return nil
	}
	var reference struct {
		Key  string `json:"key"`
		Cost int    `json:"cost,omitempty"`
	}
	if strictDecode(body, &reference) != nil || reference.Key == "" {
		return errors.New("must be a resource key or {key, cost}")
	}
	return nil
}

func (ResourceRef) JSONSchema() map[string]any {
	return map[string]any{"oneOf": []any{
		map[string]any{"type": "string"},
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"key"}, "properties": map[string]any{"key": map[string]any{"type": "string"}, "cost": map[string]any{"type": "integer"}}},
	}}
}
func (ResourceRef) TypeScriptType() string { return "string | { key: string; cost?: number }" }

// RechargeDecl is a rest name, or a list of {on, amount?, minLevel?} where
// amount is "full", "halfLevel", a number of uses or {abilityMod}.
type RechargeDecl struct{}

func (*RechargeDecl) UnmarshalJSON(body []byte) error {
	var rest string
	if json.Unmarshal(body, &rest) == nil {
		return nil
	}
	var steps []struct {
		On       string          `json:"on"`
		Amount   json.RawMessage `json:"amount,omitempty"`
		MinLevel int             `json:"minLevel,omitempty"`
	}
	if strictDecode(body, &steps) != nil {
		return errors.New("must be a rest name or a list of {on, amount, minLevel}")
	}
	for _, step := range steps {
		if len(step.Amount) > 0 {
			var amount any
			_ = json.Unmarshal(step.Amount, &amount)
			switch value := amount.(type) {
			case string, float64:
			case map[string]any:
				if ability, _ := value["abilityMod"].(string); ability == "" || len(value) != 1 {
					return errors.New("a recharge amount object needs only abilityMod")
				}
			default:
				return errors.New("a recharge amount is a name, a number or {abilityMod}")
			}
		}
	}
	return nil
}

func (RechargeDecl) JSONSchema() map[string]any {
	amount := map[string]any{"oneOf": []any{
		map[string]any{"type": "string"}, map[string]any{"type": "number"},
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"abilityMod"}, "properties": map[string]any{"abilityMod": map[string]any{"type": "string"}}},
	}}
	step := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"on"}, "properties": map[string]any{
		"on": map[string]any{"type": "string"}, "amount": amount, "minLevel": map[string]any{"type": "integer"},
	}}
	return map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "array", "items": step}}}
}
func (RechargeDecl) TypeScriptType() string {
	return `string | Array<{ on: string; amount?: string | number | { abilityMod: string }; minLevel?: number }>`
}
