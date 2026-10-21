package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestAdvantagesMarkTheirRollsAndListConditionalOnes(t *testing.T) {
	input, records, profile := movementFixture(t)
	result := EvaluateCharacter(input, records, profile)
	saves, skills := object(view(result.Sheet)["saves"]), object(view(result.Sheet)["skills"])
	if !truth(object(saves["DEX"])["advantage"]) || truth(object(saves["WIS"])["advantage"]) || !truth(view(result.Sheet)["initiativeAdvantage"]) {
		t.Fatal("save or initiative advantage was not marked exactly", saves, view(result.Sheet)["initiativeAdvantage"])
	}
	if !truth(object(skills["stealth"])["advantage"]) || truth(object(skills["athletics"])["advantage"]) || !truth(view(result.Sheet)["concentrationAdvantage"]) {
		t.Fatal("skill or concentration advantage was not marked exactly", skills["stealth"], skills["athletics"])
	}
	conditional := 0
	for _, entry := range objects(view(result.Sheet)["advantages"]) {
		if text(entry["when"]) == "against spells" {
			conditional++
		}
	}
	if conditional != 1 || len(objects(view(result.Sheet)["advantages"])) != 5 {
		t.Fatal("advantages were not listed with their conditions", view(result.Sheet)["advantages"])
	}
	input.Play.ActiveFeatures = map[string]bool{activationKey(result, "wings"): true}
	input.Play.Inventory = []character.Item{{ID: "mail", Name: "Mail", Quantity: 1, Location: "equipped", Reference: &character.Reference{Kind: "armor", ID: "mail"}}}
	records.byKind["armor"] = map[string]json.RawMessage{"mail": mustJSON(Object{"kind": "armor", "id": "mail", "name": "Mail", "armorType": "heavy", "baseAC": 16, "dexCap": 0})}
	result = EvaluateCharacter(input, records, profile)
	if !truth(object(object(view(result.Sheet)["skills"])["athletics"])["advantage"]) || !truth(object(object(view(result.Sheet)["abilities"])["STR"])["checkAdvantage"]) {
		t.Fatal("an active Strength check advantage was not marked", view(result.Sheet)["skills"])
	}
	if truth(view(result.Sheet)["concentrationAdvantage"]) {
		t.Fatal("an armor-restricted advantage applied while wearing armor")
	}
}

func TestConditionAndArmorDisadvantageCancelFeatureAdvantage(t *testing.T) {
	input, records, profile := movementFixture(t)
	profile.Constants.Character.ConditionRules = "table-states"
	records.byKind["rule"] = map[string]json.RawMessage{"table-states": mustJSON(Object{"id": "table-states", "kind": "rule", "name": "Synthetic condition rules", "book": "test", "conditionDefinitions": []conditionDefinition{
		{ID: "sickened", Name: "Sickened", MaximumLevel: 1, Rolls: []conditionRoll{{On: "attack", Mode: "disadvantage"}, {On: "check", Mode: "disadvantage"}}},
		{ID: "hobbled", Name: "Hobbled", MaximumLevel: 1, Rolls: []conditionRoll{{On: "save", Mode: "disadvantage", Abilities: []string{"DEX"}}}},
		{ID: "spooked", Name: "Spooked", MaximumLevel: 1, Rolls: []conditionRoll{{On: "attack", Mode: "disadvantage", When: "while the source is in sight"}}},
	}})}
	input.Play.Conditions = []character.Condition{{ID: "sickened", Level: 1}, {ID: "hobbled", Level: 1}, {ID: "spooked", Level: 1}}
	result := EvaluateCharacter(input, records, profile)
	saves, skills := object(view(result.Sheet)["saves"]), object(view(result.Sheet)["skills"])
	// The feature's DEX save and Stealth advantages meet condition disadvantage and cancel.
	if truth(object(saves["DEX"])["advantage"]) || truth(object(saves["DEX"])["disadvantage"]) ||
		truth(object(skills["stealth"])["advantage"]) || truth(object(skills["stealth"])["disadvantage"]) {
		t.Fatal("opposite marks did not cancel", saves["DEX"], skills["stealth"])
	}
	if !truth(object(skills["athletics"])["disadvantage"]) || !truth(view(result.Sheet)["attackDisadvantage"]) || !truth(view(result.Sheet)["initiativeAdvantage"]) {
		t.Fatal("condition disadvantage was not marked", skills["athletics"], view(result.Sheet)["attackDisadvantage"])
	}
	situational := 0
	for _, entry := range objects(view(result.Sheet)["advantages"]) {
		if object(entry["source"])["type"] == "condition" && text(entry["when"]) != "" {
			situational++
		}
	}
	if situational != 1 {
		t.Fatal("situational condition effect was not listed", view(result.Sheet)["advantages"])
	}
	input.Play.Conditions = nil
	input.Play.Inventory = []character.Item{{ID: "plate", Name: "Plate", Quantity: 1, Location: "equipped", Reference: &character.Reference{Kind: "armor", ID: "plate"}}}
	records.byKind["armor"] = map[string]json.RawMessage{"plate": mustJSON(Object{"kind": "armor", "id": "plate", "name": "Plate", "armorType": "heavy", "baseAC": 18, "dexCap": 0, "stealthDisadvantage": true})}
	result = EvaluateCharacter(input, records, profile)
	// The feature's Stealth advantage meets the plate's Stealth disadvantage.
	if stealth := object(object(view(result.Sheet)["skills"])["stealth"]); truth(stealth["advantage"]) || truth(stealth["disadvantage"]) {
		t.Fatal("armor disadvantage did not cancel the Stealth advantage", stealth)
	}
	found := false
	for _, entry := range objects(view(result.Sheet)["advantages"]) {
		if object(entry["source"])["type"] == "armor" && text(entry["name"]) == "Plate" && text(entry["mode"]) == "disadvantage" {
			found = true
		}
	}
	if !found {
		t.Fatal("armor Stealth disadvantage was not listed", view(result.Sheet)["advantages"])
	}
}

func TestMalformedConditionRollsFailClosed(t *testing.T) {
	input, records, profile := conditionFixture(t)
	records.byKind["rule"]["table-states"] = mustJSON(Object{"id": "table-states", "kind": "rule", "name": "Synthetic condition rules", "book": "test", "conditionDefinitions": []any{
		Object{"id": "odd", "name": "Odd", "maximumLevel": 1, "rolls": []any{Object{"on": "dance", "mode": "disadvantage"}}},
	}})
	if truth(object(view(EvaluateCharacter(input, records, profile).Guidance)["conditions"])["available"]) {
		t.Fatal("an unknown roll kind was accepted")
	}
}
