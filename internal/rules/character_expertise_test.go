package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func expertiseFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["background"]["artisan"] = mustJSON(Object{"kind": "background", "id": "artisan", "skillProficiencies": []any{"arcana", "history", "perception", "animal-handling"}})
	records.byKind["feature"] = map[string]json.RawMessage{
		"early-study": mustJSON(Object{"kind": "feature", "id": "early-study", "classId": "fighter", "level": 1,
			"grants": Object{"choices": []any{Object{"id": "early-study", "type": "expertise", "count": 2}}}}),
		"late-study": mustJSON(Object{"kind": "feature", "id": "late-study", "classId": "fighter", "level": 3,
			"grants": Object{"skills": []any{"nature"}, "choices": []any{Object{"id": "late-study", "type": "expertise", "count": 1}}}}),
	}
	return input, records, profile
}
func expertiseOptions(result character.Result, id string) []string {
	options := []string{}
	for _, option := range objects(object(object(result.Guidance["choices"])[id])["options"]) {
		options = append(options, text(option["id"]))
	}
	return options
}
func hasChoiceIssue(result character.Result, id string) bool {
	for _, issue := range result.Issues {
		if issue.ID == id {
			return true
		}
	}
	return false
}
func TestCharacterExpertiseUsesAcquisitionSkillsAndKeepsItsOwnSelections(t *testing.T) {
	input, records, profile := expertiseFixture(t)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	input.Build.Choices = []character.Choice{
		{ID: "early-study", Slot: 0, Value: mustJSON("arcana")},
		{ID: "early-study", Slot: 1, Value: mustJSON("animalHandling")},
		{ID: "late-study", Value: mustJSON("nature")},
	}
	before, sources := cloneCharacter(input), mustJSON(records.byKind)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !truth(result.Guidance["canSave"]) {
		t.Fatal(result.Issues)
	}
	if contains(expertiseOptions(result, "early-study"), "nature") || !contains(expertiseOptions(result, "late-study"), "nature") {
		t.Fatal("Later proficiency qualified an earlier Expertise choice", result.Guidance)
	}
	if contains(expertiseOptions(result, "late-study"), "arcana") || contains(expertiseOptions(result, "late-study"), "animalHandling") {
		t.Fatal("Later Expertise offered already selected skills")
	}
	skill := object(object(result.Sheet["skills"])["arcana"])
	if !truth(skill["expertise"]) || integer(skill["total"], 0) != 5 {
		t.Fatal("Expertise calculation", skill)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatalf("Evaluation changed input: before=%+v after=%+v", before, input)
	}
	if string(sources) != string(mustJSON(records.byKind)) {
		t.Fatal("Evaluation changed source records")
	}
	if again := EvaluateCharacter(input, records, profile); !reflect.DeepEqual(result, again) {
		left, right := string(mustJSON(result)), string(mustJSON(again))
		for index := 0; index < min(len(left), len(right)); index++ {
			if left[index] != right[index] {
				t.Fatalf("Nondeterministic evaluation near byte %d: %s / %s", index, left[max(0, index-80):min(len(left), index+140)], right[max(0, index-80):min(len(right), index+140)])
			}
		}
		t.Fatal("Nondeterministic evaluation length")
	}
	// Input array order cannot make the later acquisition win a duplicate.
	input.Build.Choices = append([]character.Choice{{ID: "late-study", Value: mustJSON("arcana")}}, input.Build.Choices[:2]...)
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:late-study#0") || hasChoiceIssue(result, "invalid-option:early-study#0") || truth(result.Guidance["canSave"]) {
		t.Fatal("Duplicate Expertise did not preserve the earlier choice", result.Issues)
	}
	if truth(object(object(result.Guidance["choices"])["late-study"])["done"]) {
		t.Fatal("Invalid expertise marked complete")
	}
	input.Build.Choices[0].Value = mustJSON("history")
	records.byKind["background"]["artisan"] = mustJSON(Object{"kind": "background", "id": "artisan", "skillProficiencies": []any{"history", "animalHandling"}})
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:early-study#0") || hasChoiceIssue(result, "invalid-option:early-study#1") {
		t.Fatal("Earlier proficiency replacement invalidated the wrong slots", result.Issues)
	}
}
func TestCharacterExpertiseHonorsClassLevelRestrictedPoolsAndCombinedGrants(t *testing.T) {
	input, records, profile := expertiseFixture(t)
	records.byKind["class"]["scout"] = mustJSON(Object{"kind": "class", "id": "scout", "multiclassPrerequisites": Object{}})
	class := recordByID(records, "class", "fighter")
	class["multiclassPrerequisites"] = Object{}
	records.byKind["class"]["fighter"] = mustJSON(class)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "scout"}, character.Level{ID: "three", ClassID: "scout"})
	result := EvaluateCharacter(input, records, profile)
	if object(result.Guidance["choices"])["late-study"] != nil {
		t.Fatal("Total level granted a later class feature")
	}
	early := recordByID(records, "feature", "early-study")
	object(early["grants"])["choices"] = []any{Object{"id": "early-study", "type": "expertise", "count": 1, "from": []any{"history", "stealth"}}}
	records.byKind["feature"]["early-study"] = mustJSON(early)
	records.byKind["species"]["dwarf"] = mustJSON(Object{"kind": "species", "id": "dwarf", "grants": Object{
		"expertise": []any{"history"}, "choices": []any{Object{"id": "training", "type": "skillExpertise", "count": 1}}}})
	input.Build.Choices = []character.Choice{{ID: "species:dwarf:training", Value: mustJSON("stealth")}}
	result = EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) {
		t.Fatal("Combined proficiency and Expertise required an earlier proficiency", result.Issues)
	}
	if !truth(object(object(result.Sheet["skills"])["stealth"])["expertise"]) {
		t.Fatal("Combined grant lost Expertise")
	}
	if len(expertiseOptions(result, "early-study")) != 0 {
		t.Fatal("Fixed or independently selected Expertise remained eligible", expertiseOptions(result, "early-study"))
	}
}

func TestCharacterExpertiseUsesDeclaredReplacementAndEffectiveDMProficiency(t *testing.T) {
	input, records, profile := expertiseFixture(t)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	input.Grants = []character.Grant{{ID: "training", Name: "Training", ActorID: "dm", Reason: "Reward", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 2, Condition: "always", Effects: []character.Effect{{Target: "proficiency", Key: "stealth", Mode: "set", Value: 1}}}}
	result := EvaluateCharacter(input, records, profile)
	if contains(expertiseOptions(result, "early-study"), "stealth") || !contains(expertiseOptions(result, "late-study"), "stealth") {
		t.Fatal("DM proficiency ignored its effective level")
	}
	early := recordByID(records, "feature", "early-study")
	objects(object(early["grants"])["choices"])[0]["changeOn"] = "longRest"
	records.byKind["feature"]["early-study"] = mustJSON(early)
	if !contains(expertiseOptions(EvaluateCharacter(input, records, profile), "early-study"), "stealth") {
		t.Fatal("Source-declared replacement could not use current proficiencies")
	}
}

func TestInvalidExpertiseAliasCannotReserveAnotherGrantsSkill(t *testing.T) {
	input, records, profile := expertiseFixture(t)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	early := recordByID(records, "feature", "early-study")
	object(early["grants"])["choices"] = []any{Object{"id": "early-study", "type": "expertise", "count": 1, "from": []any{"animal-handling"}}}
	records.byKind["feature"]["early-study"] = mustJSON(early)
	input.Build.Choices = []character.Choice{{ID: "early-study", Value: mustJSON("animalHandling")}, {ID: "late-study", Value: mustJSON("animalHandling")}}
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:early-study#0") || hasChoiceIssue(result, "invalid-option:late-study#0") {
		t.Fatal("Ineligible raw option reserved a canonical skill for another choice", result.Issues)
	}
}
