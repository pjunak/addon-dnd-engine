package rules

import (
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestFeatOptionsUseTotalLevelAtOrderedClassAdvancement(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	records.byKind["feat"]["veteran"] = mustJSON(Object{"kind": "feat", "id": "veteran", "category": "general", "prerequisites": Object{"all": []any{Object{"level": 6}}}})
	records.byKind["class"]["scout"] = mustJSON(Object{"kind": "class", "id": "scout", "hitDie": "d8", "multiclassPrerequisites": Object{}})
	fighter := recordByID(records, "class", "fighter")
	fighter["multiclassPrerequisites"] = Object{}
	records.byKind["class"]["fighter"] = mustJSON(fighter)
	levels := append([]character.Level{}, input.Build.Levels...)
	scouts := []character.Level{{ID: "scout-one", ClassID: "scout"}, {ID: "scout-two", ClassID: "scout"}}
	input.Build.Levels = append(append(append([]character.Level{}, levels[:3]...), scouts...), levels[3:]...)
	if !offeredFeat(EvaluateCharacter(input, records, profile), "asi:fighter:4", "veteran") {
		t.Fatal("earlier multiclass levels did not qualify the advancement")
	}
	input.Build.Levels = append(levels, scouts...)
	if offeredFeat(EvaluateCharacter(input, records, profile), "asi:fighter:4", "veteran") {
		t.Fatal("later multiclass levels qualified an earlier advancement")
	}
}

func TestRepeatedFeatOptionsRespectEarlierAcquisitionWaivers(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	records.byKind["feat"]["veteran"] = mustJSON(Object{"kind": "feat", "id": "veteran", "category": "general", "repeatable": true, "prerequisites": Object{"text": "Needs adjudication"}})
	input.Build.Choices = []character.Choice{
		{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("veteran")},
		{ID: "asi:fighter:8", Value: mustJSON("feat")}, {ID: "asi:fighter:8:feat", Value: mustJSON("veteran")},
	}
	input.Grants = []character.Grant{{ID: "review", Name: "Reviewed training", ActorID: "dm", Reason: "Adjudication", GrantedAt: input.Play.AsOf, Active: true, Condition: "always", EffectiveLevel: 8, Waivers: []string{"feat:veteran"}}}
	result := EvaluateCharacter(input, records, profile)
	if !hasFeatBlock(result, "veteran") || offeredFeat(result, "asi:fighter:4", "veteran") || offeredFeat(result, "asi:fighter:8", "veteran") {
		t.Fatal("later waiver concealed an invalid earlier acquisition", result.Issues)
	}
	input.Grants[0].EffectiveLevel = 4
	result = EvaluateCharacter(input, records, profile)
	if hasFeatBlock(result, "veteran") || !offeredFeat(result, "asi:fighter:4", "veteran") || !offeredFeat(result, "asi:fighter:8", "veteran") {
		t.Fatal("timely waiver did not qualify each acquisition", result.Issues)
	}
}

func TestReplacingParentFeatWithdrawsItsDescendantsBeforeOptionFiltering(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	records.byKind["feat"]["mentor"] = mustJSON(Object{"kind": "feat", "id": "mentor", "category": "general", "grants": Object{"choices": []any{Object{"id": "student", "type": "feat", "count": 1, "from": []any{"training"}}}}})
	records.byKind["feat"]["training"] = mustJSON(Object{"kind": "feat", "id": "training", "category": "general", "prerequisites": Object{}})
	input.Build.Choices = []character.Choice{
		{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("mentor")},
		{ID: "feat:mentor:student", Value: mustJSON("training")},
	}
	result := EvaluateCharacter(input, records, profile)
	if !offeredFeat(result, "asi:fighter:4", "training") || offeredFeat(result, "asi:fighter:8", "training") {
		t.Fatal("replacement confused a withdrawn child with an independent acquisition")
	}
}
