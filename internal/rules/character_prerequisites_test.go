package rules

import (
	"encoding/json"
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
	"testing"
)

func progressionChoiceInput(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	input.Build.Levels = nil
	for level := 1; level <= 8; level++ {
		input.Build.Levels = append(input.Build.Levels, character.Level{ID: fmt.Sprintf("level-%d", level), ClassID: "fighter"})
	}
	source.(memoryRecords).byKind["feat"] = map[string]json.RawMessage{}
	return input, source.(memoryRecords), profile
}
func hasFeatBlock(result character.Result, id string) bool {
	for _, issue := range result.Issues {
		if issue.ID == "feat:"+id && issue.Severity == "blocker" {
			return true
		}
	}
	return false
}
func offeredFeat(result character.Result, choice, id string) bool {
	for _, option := range objects(object(object(result.Guidance["choices"])[choice])["featOptions"]) {
		if text(option["id"]) == id {
			return true
		}
	}
	return false
}

func TestSelectedFeatCannotMeetItsOwnPrerequisite(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	records.byKind["feat"]["scholar"] = mustJSON(Object{"kind": "feat", "id": "scholar", "name": "Scholar", "category": "general",
		"prerequisites": Object{"abilities": Object{"INT": 13}},
		"grants":        Object{"abilityScoreIncrease": Object{"from": []any{"INT"}, "amount": 1}}})
	input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("scholar")}}
	result := EvaluateCharacter(input, records, profile)
	if !hasFeatBlock(result, "scholar") || truth(result.Guidance["canSave"]) || offeredFeat(result, "asi:fighter:4", "scholar") {
		t.Fatal("feat qualified itself", result.Issues)
	}
	// A genuine earlier advancement can qualify a later choice.
	input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("asi")}, {ID: "asi:fighter:4:ability", Value: mustJSON(Object{"INT": 2})},
		{ID: "asi:fighter:8", Value: mustJSON("feat")}, {ID: "asi:fighter:8:feat", Value: mustJSON("scholar")}}
	result = EvaluateCharacter(input, records, profile)
	if hasFeatBlock(result, "scholar") || !offeredFeat(result, "asi:fighter:8", "scholar") {
		t.Fatal("earlier advancement ignored", result.Issues)
	}
}

func TestFeatPrerequisitesUseAcquisitionLevelAndExactWaivers(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	records.byKind["feat"]["veteran"] = mustJSON(Object{"kind": "feat", "id": "veteran", "name": "Veteran", "category": "general", "prerequisites": Object{"level": 8}})
	input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("veteran")}}
	result := EvaluateCharacter(input, records, profile)
	if !hasFeatBlock(result, "veteran") || offeredFeat(result, "asi:fighter:4", "veteran") {
		t.Fatal("later level qualified an earlier feat")
	}
	input.Build.Choices = nil
	if !offeredFeat(EvaluateCharacter(input, records, profile), "asi:fighter:8", "veteran") {
		t.Fatal("qualified later option hidden")
	}
	records.byKind["feat"]["veteran"] = mustJSON(Object{"kind": "feat", "id": "veteran", "name": "Veteran", "category": "general", "prerequisites": Object{"text": "Needs adjudication"}})
	input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("veteran")}}
	input.Grants = []character.Grant{{ID: "review", Name: "Reviewed requirement", ActorID: "dm", Reason: "Adjudication", GrantedAt: input.Play.AsOf, Active: true, Condition: "always", EffectiveLevel: 1, Waivers: []string{"feat:another"}}}
	if !hasFeatBlock(EvaluateCharacter(input, records, profile), "veteran") {
		t.Fatal("unrelated waiver accepted")
	}
	input.Grants[0].Waivers = []string{"feat:veteran"}
	result = EvaluateCharacter(input, records, profile)
	if hasFeatBlock(result, "veteran") || !offeredFeat(result, "asi:fighter:4", "veteran") {
		t.Fatal("exact waiver ignored", result.Issues)
	}
}
