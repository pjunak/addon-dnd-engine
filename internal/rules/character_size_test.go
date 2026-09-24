package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func sizeFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	species := recordByID(records, "species", input.Build.Species)
	species["size"] = "Tiny or Large"
	species["sizeOptions"] = []any{"Tiny", "Large"}
	records.byKind["species"][input.Build.Species] = mustJSON(species)
	return input, records, profile
}

func TestSpeciesSizeIsExplicitValidatedAndDetached(t *testing.T) {
	input, records, profile := sizeFixture(t)
	id := "species:" + input.Build.Species + ":size"
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if result.Ready || !truth(result.Guidance["canSave"]) || object(result.Sheet["derived"])["size"] != nil || !reflect.DeepEqual(input, before) {
		t.Fatal("unfinished size must stay unknown and saveable without inventing a choice", result.Issues)
	}
	options := objects(object(object(result.Guidance["choices"])[id])["options"])
	if len(options) != 2 || options[0]["labelKey"] != "Large" || options[1]["labelKey"] != "Tiny" {
		t.Fatal("provider options and translatable labels were lost", options)
	}
	for _, size := range []string{"Tiny", "Large"} {
		input.Build.Choices = []character.Choice{{ID: id, Value: mustJSON(size)}}
		before = cloneCharacter(input)
		result = EvaluateCharacter(input, records, profile)
		if !result.Ready || object(result.Sheet["derived"])["size"] != size || result.Explanations["derived.size"].Sources[0].ID != input.Build.Species || !reflect.DeepEqual(input, before) {
			t.Fatal("valid source-defined size failed or changed inputs", result.Issues)
		}
	}
	input.Build.Choices[0].Value = mustJSON("Medium")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+id+"#0") || truth(result.Guidance["canSave"]) || object(result.Sheet["derived"])["size"] != nil {
		t.Fatal("an undeclared size was accepted", result.Issues)
	}
	input.Build.Choices[0] = character.Choice{ID: id, Slot: 1, Value: mustJSON("Tiny")}
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "choice-count:"+id+"#1") || truth(result.Guidance["canSave"]) {
		t.Fatal("an extra size slot was accepted", result.Issues)
	}
}

func TestSpeciesSizeChangePreservesAuthoredStateForExplicitRepair(t *testing.T) {
	input, records, profile := sizeFixture(t)
	id := "species:" + input.Build.Species + ":size"
	input.Build.Choices = []character.Choice{{ID: id, Value: mustJSON("Tiny")}}
	input.Notes = "Keep my notes"
	input.Play.Currency["gp"] = 37
	input.Play.ResourceUses["hit-dice-d10"] = 1
	before := cloneCharacter(input)
	species := recordByID(records, "species", input.Build.Species)
	species["sizeOptions"] = []any{"Small", "Large"}
	records.byKind["species"][input.Build.Species] = mustJSON(species)
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+id+"#0") || !reflect.DeepEqual(result.Inputs, before) || !reflect.DeepEqual(input, before) {
		t.Fatal("source change silently rewrote the choice or play state", result.Issues)
	}
	records.byKind["species"]["other"] = mustJSON(Object{"id": "other", "kind": "species", "size": "Small"})
	input.Build.Species = "other"
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:"+id+"#0") || object(result.Sheet["derived"])["size"] != "Small" {
		t.Fatal("a choice crossed species ownership", result.Issues)
	}
	input.Build.Choices = nil
	result = EvaluateCharacter(input, records, profile)
	if !result.Ready || len(objects(result.Plan["creationChoices"])) != 0 {
		t.Fatal("fixed size incorrectly required a choice", result.Issues)
	}
}

func TestLegacyCompoundSizeDoesNotPretendToBeASelectedValue(t *testing.T) {
	input, records, profile := sizeFixture(t)
	species := recordByID(records, "species", input.Build.Species)
	delete(species, "sizeOptions")
	records.byKind["species"][input.Build.Species] = mustJSON(species)
	result := EvaluateCharacter(input, records, profile)
	if _, exists := object(result.Sheet["derived"])["size"]; exists || len(objects(result.Plan["creationChoices"])) != 0 {
		t.Fatal("legacy display prose was interpreted as a mechanical selection")
	}
}
