package rules

import (
	"encoding/json"

	"github.com/pjunak/addon-dnd-engine/character"
)

// view is a typed value's JSON form, with whole numbers as int, so assertions
// read a result the way consumers receive it.
func view(value any) Object { return jsonView(value) }

// hydratedView is Hydrate's result in JSON form.
type hydratedView struct {
	Sheet    Object
	Warnings []string
}

func hydrateView(decisions Object, records Records, profile *Ruleset) hydratedView {
	result := Hydrate(decisions, records, *profile)
	return hydratedView{Sheet: view(result.Sheet), Warnings: result.Warnings}
}

// applyPlay decodes a play change as the service does, then applies it.
func applyPlay(input character.Inputs, change Object, records Records, profile Ruleset) (character.Result, error) {
	body, _ := json.Marshal(change)
	command, err := character.DecodePlayCommand(body)
	if err != nil {
		return EvaluateCharacter(input, records, profile), err
	}
	return ApplyCharacterPlay(input, command, records, profile)
}

// decodeAs reads a fixture written as a map into a typed value.
func decodeAs[T any](value any) T {
	body, _ := json.Marshal(value)
	var result T
	_ = json.Unmarshal(body, &result)
	return result
}
