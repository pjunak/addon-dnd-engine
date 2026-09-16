package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func builderIssue(result character.Result, target string) Object {
	for _, section := range objects(result.Guidance["sections"]) {
		for _, issue := range objects(section["issues"]) {
			if text(issue["id"]) == target {
				return issue
			}
		}
	}
	return nil
}

func TestCharacterGuidanceRequiresTheFirstClass(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Build.Levels = nil
	result := EvaluateCharacter(input, records, profile)
	issue := builderIssue(result, "add-class")
	if issue == nil || text(issue["tab"]) != "add-class" || truth(result.Guidance["ready"]) {
		t.Fatal("missing first class has no actionable guidance", result.Guidance)
	}
	if !truth(result.Guidance["canSave"]) {
		t.Fatal("unfinished first class must still save", result.Issues)
	}
}

func TestCharacterGuidanceMarksInvalidAdvancementForRepair(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	input.Build.Levels = input.Build.Levels[:4]
	records.byKind["feat"]["scholar"] = mustJSON(Object{"kind": "feat", "id": "scholar", "name": "Scholar", "category": "general", "prerequisites": Object{"abilities": Object{"INT": 13}}})
	input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("scholar")}}
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	entry := object(object(result.Guidance["choices"])["asi:fighter:4"])
	issue := builderIssue(result, "asi:fighter:4")
	if truth(entry["done"]) || issue == nil || !truth(issue["repair"]) || text(issue["tab"]) != "fighter" {
		t.Fatal("ineligible saved advancement appears complete or cannot be repaired", entry, issue)
	}
	section := objects(result.Guidance["sections"])[1]
	if integer(section["complete"], 0) >= integer(section["total"], 0) || truth(result.Guidance["canSave"]) {
		t.Fatal("invalid advancement counted complete or became saveable", section)
	}
	if !reflect.DeepEqual(input, before) || !reflect.DeepEqual(result, EvaluateCharacter(input, records, profile)) {
		t.Fatal("guidance mutated input or was nondeterministic")
	}
	input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("asi")}, {ID: "asi:fighter:4:ability", Value: mustJSON(Object{"INT": 2})}}
	repaired := EvaluateCharacter(input, records, profile)
	if builderIssue(repaired, "asi:fighter:4") != nil || !truth(object(object(repaired.Guidance["choices"])["asi:fighter:4"])["done"]) {
		t.Fatal("valid replacement still marked for repair", repaired.Guidance)
	}
}

func TestCharacterGuidanceDoesNotCountInvalidArrayAsComplete(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Build.Method = "array"
	for _, ability := range Abilities {
		input.Build.BaseScores[ability] = 12
	}
	result := EvaluateCharacter(input, records, profile)
	if issue := builderIssue(result, "abilities"); issue == nil || text(issue["tab"]) != "character" {
		t.Fatal("invalid array had no ability repair target", result.Guidance)
	}
}

func TestBuilderGuidancePreservesAuthoredNamesInLabels(t *testing.T) {
	label := builderGuidanceText("{0} level {1} advancement", "Class {1}", 4)
	if text(label["label"]) != "Class {1} level 4 advancement" {
		t.Fatal("formatting changed an authored record name", label)
	}
}
