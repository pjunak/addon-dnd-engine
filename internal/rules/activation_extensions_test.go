package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func activationFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "level-two", ClassID: "fighter"})
	records.byKind["feature"] = map[string]json.RawMessage{
		"fighter-fury": mustJSON(Object{"id": "fighter-fury", "kind": "feature", "name": "Fury", "classId": "fighter", "level": 1,
			"grants":      Object{"resources": []any{Object{"key": "fury", "name": "Fury", "fixed": 3, "recharge": "long"}}},
			"activations": []any{Object{"id": "fury", "name": "Fury", "resource": "fury", "modifiers": []any{Object{"target": "resistance", "value": "fire"}}}}}),
		"fighter-surge": mustJSON(Object{"id": "fighter-surge", "kind": "feature", "name": "Surge", "classId": "fighter", "level": 1,
			"activations": []any{Object{"id": "surge", "name": "Surge", "resource": Object{"key": "fury", "cost": 2}, "modifiers": []any{}}}}),
		"fighter-deep-fury": mustJSON(Object{"id": "fighter-deep-fury", "kind": "feature", "name": "Deep Fury", "classId": "fighter", "level": 2,
			"activations": []any{Object{"extends": "fury", "modifiers": []any{Object{"target": "conditionImmunity", "value": "charmed"}}}}}),
		"fighter-late-fury": mustJSON(Object{"id": "fighter-late-fury", "kind": "feature", "name": "Late Fury", "classId": "fighter", "level": 2,
			"activations": []any{Object{"extends": []any{"fury"}, "minLevel": 5, "modifiers": []any{Object{"target": "resistance", "value": "cold"}}}}}),
	}
	input.Play.HP = 1
	return input, records, profile
}

func activationKey(result character.Result, id string) string {
	for _, activation := range objects(view(result.Sheet)["activations"]) {
		if text(activation["id"]) == id {
			return text(activation["key"])
		}
	}
	return ""
}

func TestActivationsSpendNamedResourcesAndCosts(t *testing.T) {
	input, records, profile := activationFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || len(objects(view(result.Sheet)["activations"])) != 2 {
		t.Fatal("extensions must not add their own toggles", result.Issues, view(result.Sheet)["activations"])
	}
	fury := activationKey(result, "fury")
	result, err := applyPlay(result.Inputs, Object{"operation": "toggle-feature", "key": fury, "enabled": true}, records, profile)
	if err != nil || result.Inputs.Play.ResourceUses["fury"] != 1 {
		t.Fatal("a resource named by its key was not spent", err, result.Inputs.Play.ResourceUses)
	}
	result, err = applyPlay(result.Inputs, Object{"operation": "toggle-feature", "key": fury, "enabled": false}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	surge := activationKey(result, "surge")
	result, err = applyPlay(result.Inputs, Object{"operation": "toggle-feature", "key": surge, "enabled": true}, records, profile)
	if err != nil || result.Inputs.Play.ResourceUses["fury"] != 3 {
		t.Fatal("an activation cost was not spent", err, result.Inputs.Play.ResourceUses)
	}
	if _, err = applyPlay(result.Inputs, Object{"operation": "toggle-feature", "key": fury, "enabled": true}, records, profile); err == nil {
		t.Fatal("an exhausted resource still activated its feature")
	}
}

func TestActivationExtensionsApplyWhileTheirActivationIsOn(t *testing.T) {
	input, records, profile := activationFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if contains(stringsOf(view(result.Sheet)["conditionImmunities"]), "charmed") {
		t.Fatal("an extension applied while its activation was off")
	}
	input.Play.ActiveFeatures = map[string]bool{activationKey(result, "fury"): true}
	result = EvaluateCharacter(input, records, profile)
	if !contains(stringsOf(view(result.Sheet)["conditionImmunities"]), "charmed") || !contains(stringsOf(view(result.Sheet)["resistances"]), "fire") {
		t.Fatal("an extension did not apply with its activation", view(result.Sheet)["conditionImmunities"])
	}
	if contains(stringsOf(view(result.Sheet)["resistances"]), "cold") {
		t.Fatal("an extension applied below its level", view(result.Sheet)["resistances"])
	}
	input.Build.Levels = input.Build.Levels[:1]
	result = EvaluateCharacter(input, records, profile)
	if contains(stringsOf(view(result.Sheet)["conditionImmunities"]), "charmed") {
		t.Fatal("an extension applied before its feature was acquired")
	}
}

func TestWhileOptionsApplyOnlyDuringTheirActivationAndKeepTheChoice(t *testing.T) {
	input, records, profile := activationFixture(t)
	records.byKind["feature"]["fighter-aspect"] = mustJSON(Object{"id": "fighter-aspect", "kind": "feature", "name": "Aspect", "classId": "fighter", "level": 1,
		"activations": []any{
			Object{"id": "aspect-stone", "name": "Aspect: Stone", "exclusiveGroup": "aspect", "while": "fury", "modifiers": []any{Object{"target": "resistance", "value": "thunder"}}},
			Object{"id": "aspect-wind", "name": "Aspect: Wind", "exclusiveGroup": "aspect", "while": []any{"fury"}, "modifiers": []any{Object{"target": "resistance", "value": "lightning"}}},
		}})
	result := EvaluateCharacter(input, records, profile)
	fury, stone, wind := activationKey(result, "fury"), activationKey(result, "aspect-stone"), activationKey(result, "aspect-wind")
	if stone == "" || wind == "" {
		t.Fatal("a while option lost its own toggle", view(result.Sheet)["activations"])
	}
	toggle := func(key string, enabled bool) {
		t.Helper()
		var err error
		result, err = applyPlay(result.Inputs, Object{"operation": "toggle-feature", "key": key, "enabled": enabled}, records, profile)
		if err != nil {
			t.Fatal(err)
		}
	}
	inEffect := func(id string) bool {
		for _, activation := range objects(view(result.Sheet)["activations"]) {
			if text(activation["id"]) == id {
				return truth(activation["active"])
			}
		}
		return false
	}
	toggle(stone, true)
	if contains(stringsOf(view(result.Sheet)["resistances"]), "thunder") || inEffect("aspect-stone") || result.Inputs.Play.ResourceUses["fury"] != 0 {
		t.Fatal("a while option applied without its activation", view(result.Sheet)["resistances"], view(result.Sheet)["activations"])
	}
	toggle(fury, true)
	if !contains(stringsOf(view(result.Sheet)["resistances"]), "thunder") || !inEffect("aspect-stone") {
		t.Fatal("a while option did not apply with its activation", view(result.Sheet)["resistances"])
	}
	toggle(fury, false)
	if contains(stringsOf(view(result.Sheet)["resistances"]), "thunder") || inEffect("aspect-stone") || !result.Inputs.Play.ActiveFeatures[stone] {
		t.Fatal("ending the activation must stop the option but keep its choice", view(result.Sheet)["resistances"], result.Inputs.Play.ActiveFeatures)
	}
	toggle(fury, true)
	if !contains(stringsOf(view(result.Sheet)["resistances"]), "thunder") {
		t.Fatal("the remembered option did not return with its activation", view(result.Sheet)["resistances"])
	}
	toggle(wind, true)
	resistances := stringsOf(view(result.Sheet)["resistances"])
	if contains(resistances, "thunder") || !contains(resistances, "lightning") || result.Inputs.Play.ActiveFeatures[stone] {
		t.Fatal("while options lost their exclusive group", resistances, result.Inputs.Play.ActiveFeatures)
	}
}
