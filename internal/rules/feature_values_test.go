package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

// A level 3 fighter of the "edge" subclass with feature attacks, a lower
// critical range, feature save DCs, a subclass table column, combined
// resource formulas, extra attunement and an ability-based attack bonus.
func featureValueFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	input.Build.Subclasses = map[string]string{"fighter": "edge"}
	input.Build.BaseScores = map[string]int{"STR": 8, "DEX": 13, "CON": 14, "INT": 15, "WIS": 12, "CHA": 10}
	records.byKind["subclass"] = map[string]json.RawMessage{"edge": mustJSON(Object{"kind": "subclass", "id": "edge", "name": "Edge", "classId": "fighter", "subclassLevel": 3,
		"tableColumns": []any{Object{"key": "edge-die", "name": "Edge Die", "progression": []any{Object{"level": 3, "value": "1d8"}, Object{"level": 10, "value": "1d10"}}}}})}
	records.byKind["feature"] = map[string]json.RawMessage{
		"edge-blades": mustJSON(Object{"id": "edge-blades", "kind": "feature", "name": "Blades", "classId": "fighter", "subclassId": "edge", "level": 3,
			"grants": Object{
				"attacks": []any{
					Object{"id": "mind-blade", "name": "Mind Blade", "damage": "1d6", "damageType": "psychic", "range": "melee", "category": "simple",
						"properties": []any{"finesse", "thrown"}, "thrownRange": Object{"normal": 60, "long": 120}, "mastery": "Vex", "masteryActive": true},
					Object{"id": "arc-cannon", "name": "Arc Cannon", "damage": "1d6", "damageType": "lightning", "range": "ranged", "category": "simple",
						"rangeNormalLong": Object{"normal": 90, "long": 300}, "ability": "INT"},
				},
				"attackModifiers": []any{Object{"attackBonus": 1, "damageBonus": 1, "match": []any{Object{"attack": "arc-cannon"}}}},
				"criticalRange":   19,
				"saveDCs":         []any{Object{"name": "Edge maneuvers", "abilities": []any{"STR", "DEX"}}},
				"attunementLimit": 4,
				"resources": []any{
					Object{"key": "light", "name": "Light", "perLevel": 1, "add": 1, "recharge": "long"},
					Object{"key": "ward", "name": "Ward", "perLevel": 2, "abilityMod": "INT", "recharge": "long"},
					Object{"key": "store", "name": "Store", "abilityMod": "CHA", "multiplier": 2, "min": 2, "recharge": "long"},
				},
			},
			"activations": []any{Object{"id": "holy", "name": "Holy", "modifiers": []any{
				Object{"target": "attack", "attackBonusAbility": "CHA", "min": 1, "match": []any{Object{"range": "melee"}}},
			}}}}),
	}
	input.Play.HP = 1
	return input, records, profile
}

func TestFeatureAttacksUseTheirOwnFactsAndModifiers(t *testing.T) {
	input, records, profile := featureValueFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	blade, cannon := attackNamed(Object(result.Sheet), "Mind Blade"), attackNamed(Object(result.Sheet), "Arc Cannon")
	if blade == nil || text(blade["ability"]) != "DEX" || integer(blade["attackBonus"], 0) != 3 || text(blade["damage"]) != "1d6 +1" || !truth(blade["masteryActive"]) {
		t.Fatal("a finesse feature attack did not use its proficiency, ability and mastery", blade)
	}
	if range_ := object(blade["range"]); integer(range_["normal"], 0) != 60 || !truth(range_["thrown"]) || text(blade["featureAttack"]) != "mind-blade" || blade["ref"] != nil {
		t.Fatal("a feature attack lost its thrown range or identity", blade)
	}
	if cannon == nil || text(cannon["ability"]) != "INT" || integer(cannon["attackBonus"], 0) != 5 || text(cannon["damage"]) != "1d6 +3" {
		t.Fatal("a feature attack's own ability or matched modifier was not applied", cannon)
	}
	for _, row := range objects(result.Sheet["weapons"]) {
		if integer(row["criticalRange"], 0) != 19 {
			t.Fatal("the critical range did not reach every attack", row)
		}
	}
	input.Play.ActiveFeatures = map[string]bool{activationKey(result, "holy"): true}
	result = EvaluateCharacter(input, records, profile)
	if blade = attackNamed(Object(result.Sheet), "Mind Blade"); integer(blade["attackBonus"], 0) != 4 {
		t.Fatal("an ability-based attack bonus ignored its minimum", blade)
	}
	if cannon = attackNamed(Object(result.Sheet), "Arc Cannon"); integer(cannon["attackBonus"], 0) != 5 {
		t.Fatal("a melee-only attack bonus reached a ranged attack", cannon)
	}
}

func TestFeatureValuesSaveDCsColumnsResourcesAndAttunement(t *testing.T) {
	input, records, profile := featureValueFixture(t)
	result := EvaluateCharacter(input, records, profile)
	dcs := objects(result.Sheet["saveDCs"])
	if len(dcs) != 1 || integer(dcs[0]["value"], 0) != 8+2+1 || text(dcs[0]["ability"]) != "DEX" {
		t.Fatal("a feature save DC did not use the best named ability", dcs)
	}
	found := false
	for _, row := range objects(result.Sheet["classValues"]) {
		found = found || text(row["key"]) == "edge-die" && text(row["value"]) == "1d8" && text(row["subclassId"]) == "edge"
	}
	if !found {
		t.Fatal("a subclass table column was not listed", result.Sheet["classValues"])
	}
	maximum := map[string]int{}
	for _, resource := range objects(result.Sheet["resources"]) {
		maximum[text(resource["key"])] = integer(resource["max"], 0)
	}
	if maximum["light"] != 4 || maximum["ward"] != 2*3+2 || maximum["store"] != 2 {
		t.Fatal("combined resource formulas were not added up", maximum)
	}
	if integer(object(result.Sheet["attunement"])["limit"], 0) != 4 {
		t.Fatal("a feature did not raise the attunement limit", result.Sheet["attunement"])
	}
}
