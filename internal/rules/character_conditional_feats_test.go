package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func conditionalFeatFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, records, profile := repeatableFixture(t)
	feat := recordByID(records, "feat", "training")
	feat["repeatable"] = Object{"by": "choice", "choice": "proficiencies"}
	feat["grants"] = Object{"choices": []any{Object{"id": "proficiencies", "type": "enumerated", "count": 1, "from": []any{"ember", "frost", "storm"}}}}
	records.byKind["feat"]["training"] = mustJSON(feat)
	input.Build.Choices = input.Build.Choices[:4]
	for index, id := range []string{trainingOrigin, trainingFour, trainingEight} {
		input.Build.Choices = append(input.Build.Choices, character.Choice{ID: id, Value: mustJSON([]string{"ember", "frost", "storm"}[index])})
	}
	return input, records, profile
}

func TestConditionalFeatChoicesReserveOnlyEarlierValidSelections(t *testing.T) {
	input, records, profile := conditionalFeatFixture(t)
	before, sources := cloneCharacter(input), string(mustJSON(records.byKind))
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !truth(result.Guidance["canSave"]) {
		t.Fatal(result.Issues)
	}
	if got := objects(object(object(result.Guidance["choices"])[trainingEight])["options"]); len(got) != 1 || text(got[0]["id"]) != "storm" {
		t.Fatal("later acquisition did not exclude earlier choices", got)
	}
	if !reflect.DeepEqual(input, before) || string(mustJSON(records.byKind)) != sources || !reflect.DeepEqual(result, EvaluateCharacter(input, records, profile)) {
		t.Fatal("evaluation is mutable or nondeterministic")
	}
	input.Build.Choices[6].Value = mustJSON("ember")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+trainingEight+"#0") || hasChoiceIssue(result, "invalid-option:"+trainingOrigin+"#0") || truth(result.Guidance["canSave"]) || !truth(builderIssue(result, trainingEight)["repair"]) {
		t.Fatal("duplicate was accepted or invalidated the earlier owner", result.Issues)
	}
	// Reordering authored entries must not change which acquisition owns an option.
	for left, right := 0, len(input.Build.Choices)-1; left < right; left, right = left+1, right-1 {
		input.Build.Choices[left], input.Build.Choices[right] = input.Build.Choices[right], input.Build.Choices[left]
	}
	reordered := EvaluateCharacter(input, records, profile)
	if !reflect.DeepEqual(result.Guidance, reordered.Guidance) || !reflect.DeepEqual(result.Issues, reordered.Issues) {
		t.Fatal("authored array order changed eligibility")
	}
}

func TestConditionalFeatIncompleteCorrectionAndWithdrawal(t *testing.T) {
	input, records, profile := conditionalFeatFixture(t)
	input.Build.Choices[4].Value = mustJSON("frost")
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+trainingFour+"#0") || hasChoiceIssue(result, "invalid-option:"+trainingEight+"#0") {
		t.Fatal(result.Issues)
	}
	// Removing only the invalid later pick leaves a legal, incomplete build.
	input.Build.Choices = append(input.Build.Choices[:5], input.Build.Choices[6:]...)
	result = EvaluateCharacter(input, records, profile)
	if result.Ready || !truth(result.Guidance["canSave"]) {
		t.Fatal("incomplete choice cannot save", result.Issues)
	}
	input.Build.Choices = append(input.Build.Choices, character.Choice{ID: trainingFour, Value: mustJSON("ember")})
	if result = EvaluateCharacter(input, records, profile); !result.Ready {
		t.Fatal("correction failed", result.Issues)
	}
	input.Build.Background = "other"
	input.Build.Choices = append(input.Build.Choices[:4], input.Build.Choices[5:]...)
	result = EvaluateCharacter(input, records, profile)
	options := objects(object(object(result.Guidance["choices"])[trainingFour])["options"])
	if len(options) != 3 || !truth(result.Guidance["canSave"]) {
		t.Fatal("withdrawing the origin did not release its option", options, result.Issues)
	}
}

func TestConditionalFeatCapacityAndUnknownPoliciesFailClosed(t *testing.T) {
	for _, policy := range []any{
		Object{"by": "choice", "choice": "proficiencies"},
		Object{"by": "damageType"},
		Object{"by": "choice", "choice": "missing"},
		Object{"by": "choice", "choice": "proficiencies", "unknown": true},
	} {
		input, records, profile := conditionalFeatFixture(t)
		feat := recordByID(records, "feat", "training")
		feat["repeatable"] = policy
		object(feat["grants"])["choices"] = []any{Object{"id": "proficiencies", "type": "enumerated", "count": 1, "from": []any{"ember", "frost"}}}
		records.byKind["feat"]["training"] = mustJSON(feat)
		result := EvaluateCharacter(input, records, profile)
		if !hasFeatBlock(result, "training") || truth(result.Guidance["canSave"]) || offeredFeat(result, "asi:fighter:8", "training") {
			t.Fatal("exhausted or unsupported repetition accepted", policy, result.Issues)
		}
	}
}

func TestConditionalFeatDMGrantOwnershipAndRevocation(t *testing.T) {
	input, records, profile := conditionalFeatFixture(t)
	input.Build.Choices = []character.Choice{input.Build.Choices[4]}
	input.Build.Levels = input.Build.Levels[:1]
	input.Grants = []character.Grant{{ID: "reward", Name: "Training reward", ActorID: "dm", Reason: "Earned", GrantedAt: input.Play.AsOf, Active: true, Condition: "always", EffectiveLevel: 1, Feat: &character.Reference{Kind: "feat", ID: "training"}}}
	id := "feat:training@grant%3Areward:proficiencies"
	input.Build.Choices = append(input.Build.Choices, character.Choice{ID: id, Value: mustJSON("ember")})
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+id+"#0") || hasChoiceIssue(result, "invalid-option:"+trainingOrigin+"#0") {
		t.Fatal(result.Issues)
	}
	input.Grants[0].Active = false
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:"+id+"#0") || hasChoiceIssue(result, "invalid-option:"+trainingOrigin+"#0") {
		t.Fatal("revocation affected the wrong acquisition", result.Issues)
	}
}

func TestConditionalFeatUsesAcquisitionOrderAcrossClasses(t *testing.T) {
	input, records, profile := conditionalFeatFixture(t)
	records.byKind["class"]["scout"] = mustJSON(Object{"kind": "class", "id": "scout", "name": "Scout", "hitDie": "d8", "multiclassPrerequisites": Object{}})
	fighter := recordByID(records, "class", "fighter")
	fighter["multiclassPrerequisites"] = Object{}
	records.byKind["class"]["fighter"] = mustJSON(fighter)
	input.Build.Choices = append(input.Build.Choices[:2], input.Build.Choices[4:6]...)
	input.Build.Levels = input.Build.Levels[:4]
	input.Build.Levels = append([]character.Level{{ID: "scout-first", ClassID: "scout"}}, input.Build.Levels...)
	input.Grants = []character.Grant{{ID: "study", Name: "Earlier training", ActorID: "dm", Reason: "Earned", GrantedAt: input.Play.AsOf, Active: true, Condition: "always", EffectiveLevel: 4, Feat: &character.Reference{Kind: "feat", ID: "training"}}}
	id := "feat:training@grant%3Astudy:proficiencies"
	input.Build.Choices = append(input.Build.Choices, character.Choice{ID: id, Value: mustJSON("frost")})
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+trainingFour+"#0") || hasChoiceIssue(result, "invalid-option:"+id+"#0") {
		t.Fatal("class level was mistaken for total acquisition level", result.Issues)
	}
	// Moving the scout level later puts Fighter 4 before the DM reward.
	input.Build.Levels = append(input.Build.Levels[1:], character.Level{ID: "scout-last", ClassID: "scout"})
	input.Grants[0].EffectiveLevel = 5
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+id+"#0") || hasChoiceIssue(result, "invalid-option:"+trainingFour+"#0") {
		t.Fatal("later grant invalidated earlier class choice", result.Issues)
	}
}

func TestConditionalFeatSourceChangeDoesNotReuseEligibility(t *testing.T) {
	input, records, profile := conditionalFeatFixture(t)
	if result := EvaluateCharacter(input, records, profile); !result.Ready {
		t.Fatal(result.Issues)
	}
	before := cloneCharacter(input)
	feat := recordByID(records, "feat", "training")
	object(feat["grants"])["choices"] = []any{Object{"id": "proficiencies", "type": "enumerated", "count": 1, "from": []any{"mist", "frost", "storm"}}}
	records.byKind["feat"]["training"] = mustJSON(feat)
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+trainingOrigin+"#0") || !reflect.DeepEqual(input, before) {
		t.Fatal("source change reused old eligibility or modified authored selections", result.Issues)
	}
	for _, id := range []string{trainingFour, trainingEight} {
		if hasChoiceIssue(result, "invalid-option:"+id+"#0") {
			t.Fatal("source change removed an unaffected choice", result.Issues)
		}
	}
}
