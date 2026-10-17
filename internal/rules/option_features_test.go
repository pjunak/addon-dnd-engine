package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

// A class feature offers "vision" options. Sight is free, Deep needs level 2
// and Sight, Lesson is repeatable and offers a feat of its own.
func optionFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["feature"] = map[string]json.RawMessage{
		"fighter-visions": mustJSON(Object{"id": "fighter-visions", "kind": "feature", "name": "Visions", "classId": "fighter", "level": 1,
			"grants": Object{"choices": []any{Object{"id": "visions", "type": "invocation", "count": 1, "countByLevel": Object{"2": 3}, "fromCategory": "vision"}}}}),
		"vision-sight": mustJSON(Object{"id": "vision-sight", "kind": "feature", "name": "Sight", "classId": "fighter", "level": 1, "category": "vision",
			"grants": Object{"senses": Object{"darkvision": 60}}}),
		"vision-deep": mustJSON(Object{"id": "vision-deep", "kind": "feature", "name": "Deep Sight", "classId": "fighter", "level": 2, "category": "vision",
			"prerequisite": Object{"text": "Level 2+, Sight", "level": 2, "feature": "vision-sight"}, "grants": Object{"senses": Object{"truesight": 30}}}),
		"vision-lesson": mustJSON(Object{"id": "vision-lesson", "kind": "feature", "name": "Lesson", "classId": "fighter", "level": 1, "category": "vision", "repeatable": true,
			"grants": Object{"choices": []any{Object{"id": "lesson-feat", "type": "feat", "count": 1, "category": "origin"}}}}),
	}
	records.byKind["feat"] = map[string]json.RawMessage{
		"alert":  mustJSON(Object{"id": "alert", "kind": "feat", "name": "Alert", "category": "origin", "prerequisites": Object{}}),
		"lucky":  mustJSON(Object{"id": "lucky", "kind": "feat", "name": "Lucky", "category": "origin", "prerequisites": Object{}}),
		"strong": mustJSON(Object{"id": "strong", "kind": "feat", "name": "Strong", "category": "general", "prerequisites": Object{}}),
	}
	return input, records, profile
}

func sheetFeature(result character.Result, id string) Object {
	for _, feature := range objects(result.Sheet["features"]) {
		if text(feature["id"]) == id {
			return feature
		}
	}
	return nil
}

func TestOnlyChosenOptionsBecomeFeaturesAndGrants(t *testing.T) {
	input, records, profile := optionFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if sheetFeature(result, "vision-sight") != nil || integer(object(result.Sheet["senses"])["darkvision"], 0) != 120 {
		t.Fatal("an unchosen option became a feature or granted its senses", result.Sheet["features"], result.Sheet["senses"])
	}
	if classGrantOffers(result, "visions", "vision-deep") || !classGrantOffers(result, "visions", "vision-sight") {
		t.Fatal("option prerequisites did not filter the level 1 choice")
	}
	input.Build.Choices = []character.Choice{{ID: "visions", Value: mustJSON("vision-sight")}}
	result = EvaluateCharacter(input, records, profile)
	feature := sheetFeature(result, "vision-sight")
	if !result.Ready || feature == nil || text(feature["category"]) != "vision" || text(object(feature["source"])["id"]) != "fighter" {
		t.Fatal("the chosen option is not an acquired class feature", result.Issues, result.Sheet["features"])
	}
	if _, retained := evidenceFor(result, "feature", "vision-sight"); !retained {
		t.Fatal("the chosen option's text is not retained for the sheet")
	}
	if _, retained := evidenceFor(result, "feature", "vision-deep"); retained {
		t.Fatal("an unchosen option was retained as character evidence")
	}
}

func TestOptionPrerequisitesAndRepeatableOptions(t *testing.T) {
	input, records, profile := optionFixture(t)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "level-two", ClassID: "fighter"})
	input.Build.Choices = []character.Choice{
		{ID: "visions", Slot: 0, Value: mustJSON("vision-sight")},
		{ID: "visions", Slot: 1, Value: mustJSON("vision-deep")},
		{ID: "visions", Slot: 2, Value: mustJSON("vision-lesson")},
	}
	result := EvaluateCharacter(input, records, profile)
	if integer(object(result.Sheet["senses"])["truesight"], 0) != 30 || sheetFeature(result, "vision-deep") == nil {
		t.Fatal("a qualified option did not apply", result.Issues, result.Sheet["senses"])
	}
	input.Build.Choices[0].Value = mustJSON("vision-lesson")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:visions#1") {
		t.Fatal("an option kept its place after losing its prerequisite", result.Issues)
	}
	if hasChoiceIssue(result, "duplicate-option:visions#2") {
		t.Fatal("a repeatable option could not fill a second slot", result.Issues)
	}
	if lesson := sheetFeature(result, "vision-lesson"); integer(lesson["count"], 0) != 2 {
		t.Fatal("a repeated option lost its count", lesson)
	}
	choice := object(object(result.Guidance["choices"])["lesson-feat"])
	if integer(choice["required"], 0) != 2 || !classGrantOffers(result, "lesson-feat", "alert") || classGrantOffers(result, "lesson-feat", "strong") {
		t.Fatal("the repeated option did not offer one feat per pick", choice)
	}
	input.Build.Choices[1].Value = mustJSON("vision-sight")
	input.Build.Choices = append(input.Build.Choices,
		character.Choice{ID: "lesson-feat", Slot: 0, Value: mustJSON("alert")}, character.Choice{ID: "lesson-feat", Slot: 1, Value: mustJSON("lucky")})
	result = EvaluateCharacter(input, records, profile)
	if !result.Ready || len(objects(result.Sheet["feats"])) != 2 {
		t.Fatal("feats from a repeated option were not acquired", result.Issues, result.Sheet["feats"])
	}
	input.Build.Choices[2].Value = mustJSON("vision-sight")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "duplicate-option:visions#2") {
		t.Fatal("an ordinary option was accepted twice", result.Issues)
	}
}

func evidenceFor(result character.Result, kind, id string) (character.Evidence, bool) {
	for _, entry := range result.Evidence {
		if entry.Reference.Kind == kind && entry.Reference.ID == id {
			return entry, true
		}
	}
	return character.Evidence{}, false
}
