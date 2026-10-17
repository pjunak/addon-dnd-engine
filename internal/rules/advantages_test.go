package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestAdvantagesMarkTheirRollsAndListConditionalOnes(t *testing.T) {
	input, records, profile := movementFixture(t)
	result := EvaluateCharacter(input, records, profile)
	saves, skills := object(result.Sheet["saves"]), object(result.Sheet["skills"])
	if !truth(object(saves["DEX"])["advantage"]) || truth(object(saves["WIS"])["advantage"]) || !truth(result.Sheet["initiativeAdvantage"]) {
		t.Fatal("save or initiative advantage was not marked exactly", saves, result.Sheet["initiativeAdvantage"])
	}
	if !truth(object(skills["stealth"])["advantage"]) || truth(object(skills["athletics"])["advantage"]) || !truth(result.Sheet["concentrationAdvantage"]) {
		t.Fatal("skill or concentration advantage was not marked exactly", skills["stealth"], skills["athletics"])
	}
	conditional := 0
	for _, entry := range objects(result.Sheet["advantages"]) {
		if text(entry["when"]) == "against spells" {
			conditional++
		}
	}
	if conditional != 1 || len(objects(result.Sheet["advantages"])) != 5 {
		t.Fatal("advantages were not listed with their conditions", result.Sheet["advantages"])
	}
	input.Play.ActiveFeatures = map[string]bool{activationKey(result, "wings"): true}
	input.Play.Inventory = []character.Item{{ID: "mail", Name: "Mail", Quantity: 1, Location: "equipped", Reference: &character.Reference{Kind: "armor", ID: "mail"}}}
	records.byKind["armor"] = map[string]json.RawMessage{"mail": mustJSON(Object{"kind": "armor", "id": "mail", "name": "Mail", "armorType": "heavy", "baseAC": 16, "dexCap": 0})}
	result = EvaluateCharacter(input, records, profile)
	if !truth(object(object(result.Sheet["skills"])["athletics"])["advantage"]) || !truth(object(object(result.Sheet["abilities"])["STR"])["checkAdvantage"]) {
		t.Fatal("an active Strength check advantage was not marked", result.Sheet["skills"])
	}
	if truth(result.Sheet["concentrationAdvantage"]) {
		t.Fatal("an armor-restricted advantage applied while wearing armor")
	}
}
