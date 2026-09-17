package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func originChoiceFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	background := recordByID(records, "background", "artisan")
	background["originFeat"] = "performer"
	background["toolProficiencyChoice"] = Object{"count": 1, "from": []any{"flute", "drum", "lute", "lyre"}}
	records.byKind["background"]["artisan"] = mustJSON(background)
	records.byKind["background"]["other"] = mustJSON(Object{"kind": "background", "id": "other"})
	species := recordByID(records, "species", "dwarf")
	species["grants"] = Object{"choices": []any{
		Object{"id": "skill", "type": "skillProficiency", "count": 1},
		Object{"id": "training", "type": "feat", "count": 1, "category": "origin"},
	}}
	records.byKind["species"]["dwarf"] = mustJSON(species)
	records.byKind["feat"] = map[string]json.RawMessage{
		"performer": mustJSON(Object{"kind": "feat", "id": "performer", "category": "origin", "grants": Object{"choices": []any{
			Object{"id": "instruments", "type": "toolProficiency", "count": 3, "from": []any{"flute", "drum", "lute", "lyre"}},
		}}}),
		"training": mustJSON(Object{"kind": "feat", "id": "training", "category": "origin", "repeatable": true, "grants": Object{"choices": []any{
			Object{"id": "proficiencies", "type": "proficiency", "count": 2, "from": []any{"skill:arcana", "skill:history", "skill:animal-handling", "tool:flute", "tool:horn"}},
		}}}),
		"advanced": mustJSON(Object{"kind": "feat", "id": "advanced", "category": "general"}),
	}
	return input, records, profile
}

func TestOriginToolChoicesRemainDistinctAcrossBackgroundAndFeat(t *testing.T) {
	input, records, profile := originChoiceFixture(t)
	input.Build.Choices = []character.Choice{
		{ID: "background:artisan:tool", Value: mustJSON("flute")},
		{ID: "feat:performer:instruments", Slot: 0, Value: mustJSON("drum")},
		{ID: "feat:performer:instruments", Slot: 1, Value: mustJSON("lute")},
		{ID: "feat:performer:instruments", Slot: 2, Value: mustJSON("lyre")},
	}
	result := EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) || len(stringsOf(object(result.Sheet["proficiencies"])["tools"])) != 4 {
		t.Fatal("separate background and feat grants did not combine", result.Issues)
	}
	if !contains(expertiseOptions(result, "background:artisan:tool"), "flute") || contains(expertiseOptions(result, "feat:performer:instruments"), "flute") || !contains(expertiseOptions(result, "feat:performer:instruments"), "lyre") {
		t.Fatal("choice pools rejected their own selection or offered an earlier proficiency")
	}
	input.Build.Choices[3].Value = mustJSON("flute")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:feat:performer:instruments#2") || hasChoiceIssue(result, "invalid-option:background:artisan:tool#0") || truth(result.Guidance["canSave"]) {
		t.Fatal("a later tool duplicate invalidated the wrong owner", result.Issues)
	}
}

func TestDirectOriginSkillsPrecedeMixedFeatChoicesWithoutDependingOnInputOrder(t *testing.T) {
	input, records, profile := originChoiceFixture(t)
	input.Build.Background = "other"
	fighter := recordByID(records, "class", "fighter")
	fighter["startingProficiencies"] = Object{"skills": Object{"choose": 1, "from": []any{"history", "arcana"}}}
	records.byKind["class"]["fighter"] = mustJSON(fighter)
	const mixed = "feat:training@species%3Adwarf%3Atraining:proficiencies"
	input.Build.Choices = []character.Choice{
		{ID: mixed, Value: mustJSON("tool:flute")}, {ID: mixed, Slot: 1, Value: mustJSON("tool:horn")},
		{ID: "species:dwarf:training", Value: mustJSON("training")},
		{ID: "species:dwarf:skill", Value: mustJSON("animalHandling")},
		{ID: "skills:fighter", Value: mustJSON("history")},
	}
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !reflect.DeepEqual(input, before) {
		t.Fatal("complete origin decisions failed or mutated input", result.Issues)
	}
	if contains(expertiseOptions(result, mixed), "skill:animal-handling") || contains(expertiseOptions(result, mixed), "skill:history") || contains(expertiseOptions(result, "species:dwarf:skill"), "history") {
		t.Fatal("typed skills did not reserve canonical proficiency IDs")
	}
	reversed := cloneCharacter(input)
	for left, right := 0, len(reversed.Build.Choices)-1; left < right; left, right = left+1, right-1 {
		reversed.Build.Choices[left], reversed.Build.Choices[right] = reversed.Build.Choices[right], reversed.Build.Choices[left]
	}
	repeated := EvaluateCharacter(reversed, records, profile)
	if !reflect.DeepEqual(result.Guidance, repeated.Guidance) || !reflect.DeepEqual(result.Sheet, repeated.Sheet) {
		t.Fatal("input order changed origin ownership")
	}
	input.Build.Choices[1].Value = mustJSON("skill:animal-handling")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+mixed+"#1") || hasChoiceIssue(result, "invalid-option:species:dwarf:skill#0") {
		t.Fatal("mixed alias bypassed the earlier species proficiency", result.Issues)
	}
}

func TestOriginFeatPoolRequiresExplicitEligibleSelectionAndKeepsDependentOwners(t *testing.T) {
	input, records, profile := originChoiceFixture(t)
	result := EvaluateCharacter(input, records, profile)
	options := expertiseOptions(result, "species:dwarf:training")
	if contains(options, "performer") || contains(options, "advanced") || !contains(options, "training") {
		t.Fatal("origin feat pool ignored category or duplicate eligibility", options)
	}
	if object(result.Guidance["choices"])["feat:training@species%3Adwarf%3Atraining:proficiencies"] != nil {
		t.Fatal("unselected species feat was applied automatically")
	}
	const mixed = "feat:training@species%3Adwarf%3Atraining:proficiencies"
	input.Build.Choices = []character.Choice{{ID: mixed, Value: mustJSON("skill:arcana")}, {ID: "species:dwarf:training", Value: mustJSON("training")}}
	result = EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) || !truth(object(object(result.Sheet["skills"])["arcana"])["proficient"]) {
		t.Fatal("dependent feat selection was not resolved", result.Issues)
	}
	input.Build.Choices[1].Value = mustJSON("advanced")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:species:dwarf:training#0") || !hasChoiceIssue(result, "unavailable-choice:"+mixed+"#0") {
		t.Fatal("ineligible feat or withdrawn child survived", result.Issues)
	}
}

func TestLaterFixedProficiencyDoesNotInvalidateEarlierOriginTraining(t *testing.T) {
	input, records, profile := originChoiceFixture(t)
	input.Build.Background = "other"
	records.byKind["feature"] = map[string]json.RawMessage{"later-training": mustJSON(Object{"kind": "feature", "id": "later-training", "classId": "fighter", "level": 3, "grants": Object{"skills": []any{"arcana"}}})}
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	input.Build.Choices = []character.Choice{{ID: "species:dwarf:skill", Value: mustJSON("arcana")}}
	result := EvaluateCharacter(input, records, profile)
	if hasChoiceIssue(result, "invalid-option:species:dwarf:skill#0") || !contains(expertiseOptions(result, "species:dwarf:skill"), "arcana") {
		t.Fatal("later fixed grant invalidated the earlier species selection", result.Issues)
	}
}

func TestCombinedExpertiseDoesNotDisqualifyAnEarlierProficiencyChoice(t *testing.T) {
	input, records, profile := originChoiceFixture(t)
	input.Build.Background = "other"
	records.byKind["feature"] = map[string]json.RawMessage{"study": mustJSON(Object{"kind": "feature", "id": "study", "classId": "fighter", "level": 1, "grants": Object{"choices": []any{Object{"id": "study", "type": "skillExpertise", "count": 1}}}})}
	input.Build.Choices = []character.Choice{{ID: "species:dwarf:skill", Value: mustJSON("arcana")}, {ID: "study", Value: mustJSON("arcana")}}
	result := EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) || !truth(object(object(result.Sheet["skills"])["arcana"])["expertise"]) {
		t.Fatal("combined training invalidated an existing proficiency", result.Issues)
	}
	input.Build.Choices[0].Value = mustJSON("religion")
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	records.byKind["feature"]["later"] = mustJSON(Object{"kind": "feature", "id": "later", "classId": "fighter", "level": 3, "grants": Object{"choices": []any{Object{"id": "later", "type": "skillProficiency", "count": 1}}}})
	result = EvaluateCharacter(input, records, profile)
	if contains(expertiseOptions(result, "later"), "arcana") {
		t.Fatal("later training ignored an earlier combined proficiency")
	}
}

func TestReplaceableProficiencyUsesCurrentFixedAndPermanentTraining(t *testing.T) {
	input, records, profile := originChoiceFixture(t)
	input.Build.Background = "other"
	species := recordByID(records, "species", "dwarf")
	species["grants"] = Object{"choices": []any{
		Object{"id": "skill", "type": "skillProficiency", "count": 1, "changeOn": "longRest"},
		Object{"id": "permanent", "type": "skillProficiency", "count": 1},
	}}
	records.byKind["species"]["dwarf"] = mustJSON(species)
	records.byKind["feature"] = map[string]json.RawMessage{"later-training": mustJSON(Object{"kind": "feature", "id": "later-training", "classId": "fighter", "level": 3, "grants": Object{"skills": []any{"arcana"}}})}
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	input.Build.Choices = []character.Choice{{ID: "species:dwarf:skill", Value: mustJSON("arcana")}, {ID: "species:dwarf:permanent", Value: mustJSON("history")}}
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:species:dwarf:skill#0") || contains(expertiseOptions(result, "species:dwarf:skill"), "history") || hasChoiceIssue(result, "invalid-option:species:dwarf:permanent#0") {
		t.Fatal("rest replacement ignored current proficiencies or withdrew permanent training", result.Issues)
	}
	input.Build.Choices[0].Value = mustJSON("religion")
	result = EvaluateCharacter(input, records, profile)
	if !result.Ready || !contains(expertiseOptions(result, "species:dwarf:skill"), "religion") {
		t.Fatal("rest replacement lost its own valid choice", result.Issues)
	}
}
