package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func conditionFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	profile.Constants.Character.ConditionRules = "table-states"
	records.byKind["rule"] = map[string]json.RawMessage{"table-states": mustJSON(Object{"id": "table-states", "kind": "rule", "name": "Synthetic condition rules", "book": "test", "conditionDefinitions": []conditionDefinition{
		{ID: "fatigue", Name: "Fatigue", MaximumLevel: 6, D20PenaltyPerLevel: 2, SpeedPenaltyPerLevel: 5, TerminalLevel: 6, Summary: "Synthetic tiered condition."},
		{ID: "held", Name: "Held", MaximumLevel: 1, SpeedZero: true},
		{ID: "dazed", Name: "Dazed", MaximumLevel: 1, Summary: "Situational only."},
	}})}
	return input, records, profile
}

func TestAuthoredConditionsPreserveInputsAndBoundCalculation(t *testing.T) {
	input, records, profile := conditionFixture(t)
	baseline := EvaluateCharacter(input, records, profile)
	if baseline.Inputs.Play.Conditions != nil {
		t.Fatal("omitted conditions acquired defaults")
	}
	input.Play.Conditions = []character.Condition{{ID: "fatigue", Level: 2}, {ID: "dazed", Level: 1}}
	before, _ := json.Marshal(input)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !truth(view(result.Guidance)["canSave"]) || !reflect.DeepEqual(input, result.Inputs) {
		t.Fatal(result.Issues)
	}
	if integer(object(view(result.Sheet)["derived"])["speed"], -1) != integer(object(view(baseline.Sheet)["derived"])["speed"], 0)-10 || object(view(result.Sheet)["conditionEffects"])["d20Adjustment"] != -4 {
		t.Fatal(result.Sheet)
	}
	for _, key := range []string{"abilities", "saves", "skills", "weapons", "spellcasting"} {
		if !reflect.DeepEqual(view(result.Sheet)[key], view(baseline.Sheet)[key]) {
			t.Fatal("condition altered base roll bonuses or DCs", key)
		}
	}
	if len(result.Explanations["derived.speed"].Sources) == 0 || len(objects(view(result.Sheet)["conditions"])) != 2 {
		t.Fatal("missing saved condition facts")
	}
	for _, change := range []Object{{"operation": "damage", "amount": float64(1)}, {"operation": "rest", "rest": "short"}, {"operation": "rest", "rest": "long"}} {
		played, err := applyPlay(input, change, records, profile)
		if err != nil || !reflect.DeepEqual(input.Play.Conditions, played.Inputs.Play.Conditions) {
			t.Fatal("play changed authored conditions", err)
		}
	}
	result.Inputs.Play.Conditions[0].Level = 4
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("returned state aliases inputs")
	}
	input.Play.Conditions = append(input.Play.Conditions, character.Condition{ID: "held", Level: 1})
	result = EvaluateCharacter(input, records, profile)
	if object(view(result.Sheet)["derived"])["speed"] != 0 || view(result.Sheet)["speed"] != 0 {
		t.Fatal("zero Speed did not win")
	}
	input.Play.Conditions = []character.Condition{{ID: "fatigue", Level: 6}}
	result = EvaluateCharacter(input, records, profile)
	if !truth(objects(view(result.Sheet)["conditions"])[0]["terminal"]) || result.Inputs.Play.HP != input.Play.HP {
		t.Fatal("terminal condition silently changed HP")
	}
	input.Play.Conditions = nil
	if !reflect.DeepEqual(EvaluateCharacter(input, records, profile).Sheet, baseline.Sheet) {
		t.Fatal("removing conditions did not restore base projection")
	}
}

func TestConditionGuidanceRespectsImmunityAndEveryMovementMode(t *testing.T) {
	input, records, profile := conditionFixture(t)
	input.Play.Conditions = []character.Condition{{ID: "held", Level: 1}, {ID: "fatigue", Level: 2}}
	calculation := &character.Calculation{Derived: character.Derived{Speed: 40}, Speed: 40, FlySpeed: 60, SwimSpeed: 20, ClimbSpeed: 8, BurrowSpeed: 5, ConditionImmunities: []string{"held"}}
	result := character.Result{Sheet: character.Sheet{Calculation: calculation}, Explanations: map[string]character.Explanation{}}
	characterConditions(input, records, profile, &result)
	if calculation.Derived.Speed != 30 || calculation.FlySpeed != 50 || calculation.SwimSpeed != 10 || calculation.ClimbSpeed != 0 || calculation.BurrowSpeed != 0 {
		t.Fatal(result.Sheet)
	}
	if objects(view(result.Sheet)["conditions"])[0]["status"] != "immune" || objects(object(view(result.Guidance)["conditions"])["options"])[1]["canAdd"] != false {
		t.Fatal(result.Guidance)
	}
	calculation.ConditionImmunities = nil
	calculation.Derived.Speed, calculation.FlySpeed, calculation.SwimSpeed = 40, 60, 20
	characterConditions(input, records, profile, &result)
	if calculation.Derived.Speed != 0 || calculation.FlySpeed != 0 || calculation.SwimSpeed != 0 {
		t.Fatal("zero Speed failed for a special movement mode")
	}
}

func TestConditionsFailClosedAndKeepInvalidSelectionsForRepair(t *testing.T) {
	input, records, profile := conditionFixture(t)
	for _, conditions := range [][]character.Condition{{{ID: "unknown", Level: 1}}, {{ID: "fatigue", Level: 0}}, {{ID: "fatigue", Level: 7}}, {{ID: "held", Level: 2}}, {{ID: "held", Level: 1}, {ID: "held", Level: 1}}} {
		input.Play.Conditions = conditions
		result := EvaluateCharacter(input, records, profile)
		if result.Ready || truth(view(result.Guidance)["canSave"]) || !reflect.DeepEqual(result.Inputs.Play.Conditions, conditions) {
			t.Fatal(result.Issues)
		}
	}
	input.Play.Conditions = []character.Condition{{ID: "held", Level: 1}}
	for _, body := range []string{`null`, `[]`, `[{"id":"held","name":"Held","maximumLevel":1,"unexpected":true}]`, `[{"id":"held","name":"Held","maximumLevel":1,"speedPenaltyPerLevel":-2}]`, `[{"id":"held","name":"Held","maximumLevel":1},{"id":"held","name":"Copy","maximumLevel":1}]`} {
		records.byKind["rule"]["table-states"] = json.RawMessage(`{"conditionDefinitions":` + body + `}`)
		result := EvaluateCharacter(input, records, profile)
		if truth(view(result.Guidance)["canSave"]) || truth(object(view(result.Guidance)["conditions"])["available"]) || !reflect.DeepEqual(input.Play.Conditions, result.Inputs.Play.Conditions) {
			t.Fatal("malformed source normalized saved conditions")
		}
	}
	input, records, profile = conditionFixture(t)
	blank := character.Blank()
	blank.Play.Conditions = []character.Condition{{ID: "fatigue", Level: 1}}
	result := EvaluateCharacter(blank, records, profile)
	if !truth(view(result.Guidance)["canSave"]) || view(result.Sheet)["status"] != "needs-choices" || len(objects(view(result.Sheet)["conditions"])) != 1 {
		t.Fatal("incomplete build lost conditions", result.Issues)
	}
	profile.Constants.Character.ConditionRules = "missing"
	if truth(view(EvaluateCharacter(blank, records, profile).Guidance)["canSave"]) {
		t.Fatal("withdrawn source erased conditions")
	}
	blank.Play.Conditions = nil
	if !truth(view(EvaluateCharacter(blank, records, profile).Guidance)["canSave"]) {
		t.Fatal("old provider broke a character without conditions")
	}
}
