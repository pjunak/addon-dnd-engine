package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func movementFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["species"]["dwarf"] = mustJSON(Object{"kind": "species", "id": "dwarf", "name": "Dwarf", "speeds": Object{"walk": 30, "climb": 20}})
	records.byKind["feature"] = map[string]json.RawMessage{
		"fighter-roving": mustJSON(Object{"id": "fighter-roving", "kind": "feature", "name": "Roving", "classId": "fighter", "level": 1,
			"grants": Object{"speedBonuses": []any{Object{"add": 10}}, "speeds": Object{"swim": "speed", "climb": "speed"}}}),
		"fighter-wings": mustJSON(Object{"id": "fighter-wings", "kind": "feature", "name": "Wings", "classId": "fighter", "level": 1,
			"activations": []any{Object{"id": "wings", "name": "Wings", "modifiers": []any{
				Object{"target": "flySpeed", "value": "speed"}, Object{"target": "swimSpeed", "value": "speed", "multiply": 2},
				Object{"target": "advantage", "on": "check", "abilities": []any{"STR"}},
			}}}}),
		"fighter-instinct": mustJSON(Object{"id": "fighter-instinct", "kind": "feature", "name": "Instinct", "classId": "fighter", "level": 1,
			"grants": Object{"advantages": []any{
				Object{"on": "save", "abilities": []any{"DEX"}}, Object{"on": "initiative"}, Object{"on": "check", "skills": []any{"stealth"}},
				Object{"on": "save", "when": "against spells"}, Object{"on": "concentration", "requires": Object{"noArmor": true}},
			}}}),
	}
	return input, records, profile
}

func TestSpecialSpeedsFollowTheFinalWalkingSpeed(t *testing.T) {
	input, records, profile := movementFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if integer(result.Sheet["speed"], 0) != 40 || integer(result.Sheet["swimSpeed"], 0) != 40 || integer(result.Sheet["climbSpeed"], 0) != 40 || integer(result.Sheet["flySpeed"], 1) != 0 {
		t.Fatal("passive speeds equal to Speed were not resolved", result.Sheet["speed"], result.Sheet["swimSpeed"], result.Sheet["climbSpeed"])
	}
	if _, exists := result.Sheet["burrowSpeed"]; exists || len(objects(result.Sheet["movement"])) != 3 {
		t.Fatal("ungranted movement was reported or a source row was lost", result.Sheet["movement"])
	}
	input.Play.ActiveFeatures = map[string]bool{activationKey(result, "wings"): true}
	input.Grants = []character.Grant{{ID: "haste", Name: "Haste", ActorID: "dm", Reason: "Blessing", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always",
		Effects: []character.Effect{{Target: "speed", Mode: "add", Value: 10}}}}
	result = EvaluateCharacter(input, records, profile)
	if integer(result.Sheet["speed"], 0) != 50 || integer(result.Sheet["flySpeed"], 0) != 50 || integer(result.Sheet["swimSpeed"], 0) != 100 || integer(result.Sheet["climbSpeed"], 0) != 50 {
		t.Fatal("active movement did not follow the speed after DM effects", result.Sheet["speed"], result.Sheet["flySpeed"], result.Sheet["swimSpeed"])
	}
	records.byKind["feature"] = map[string]json.RawMessage{}
	input.Play.ActiveFeatures = nil
	input.Grants = nil
	result = EvaluateCharacter(input, records, profile)
	if integer(result.Sheet["climbSpeed"], 0) != 20 || integer(result.Sheet["speed"], 0) != 30 {
		t.Fatal("the species climb speed was not reported", result.Sheet["climbSpeed"])
	}
}
