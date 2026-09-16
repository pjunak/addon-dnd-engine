package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

const trainingOrigin = "feat:training@background%3Aartisan:proficiencies"
const trainingFour = "feat:training@asi%3Afighter%3A4%3Afeat:proficiencies"
const trainingEight = "feat:training@asi%3Afighter%3A8%3Afeat:proficiencies"

func repeatableFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, records, profile := progressionChoiceInput(t)
	records.byKind["background"]["artisan"] = mustJSON(Object{"kind": "background", "id": "artisan", "name": "Artisan", "originFeat": "training"})
	records.byKind["background"]["other"] = mustJSON(Object{"kind": "background", "id": "other"})
	records.byKind["feat"]["training"] = mustJSON(Object{"kind": "feat", "id": "training", "name": "Training", "category": "general", "repeatable": true,
		"prerequisites": Object{}, "grants": Object{"choices": []any{Object{"id": "proficiencies", "type": "proficiency", "count": 3,
			"from": []any{"skill:arcana", "skill:history", "skill:nature", "skill:medicine", "tool:flute", "tool:smiths-tools", "tool:drum", "tool:dice", "tool:lute"}}}}})
	input.Build.Choices = []character.Choice{
		{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("training")},
		{ID: "asi:fighter:8", Value: mustJSON("feat")}, {ID: "asi:fighter:8:feat", Value: mustJSON("training")},
	}
	for _, row := range []struct {
		id     string
		values []string
	}{
		{trainingOrigin, []string{"skill:arcana", "tool:flute", "tool:smiths-tools"}},
		{trainingFour, []string{"skill:history", "tool:drum", "tool:dice"}},
		{trainingEight, []string{"skill:nature", "skill:medicine", "tool:lute"}},
	} {
		for slot, value := range row.values {
			input.Build.Choices = append(input.Build.Choices, character.Choice{ID: row.id, Slot: slot, Value: mustJSON(value)})
		}
	}
	return input, records, profile
}

func TestRepeatableFeatChoicesBelongToIndependentAcquisitions(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	before, sources := cloneCharacter(input), mustJSON(records.byKind)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	for _, id := range []string{trainingOrigin, trainingFour, trainingEight} {
		entry := object(object(result.Guidance["choices"])[id])
		if integer(entry["picked"], 0) != 3 || !truth(entry["done"]) {
			t.Fatal("acquisition choices collapsed", id, entry)
		}
	}
	if text(object(object(result.Guidance["choices"])[trainingFour])["label"]) != "Training · Fighter level 4" {
		t.Fatal("missing source label", result.Guidance)
	}
	for _, skill := range []string{"arcana", "history", "nature", "medicine"} {
		if !truth(object(object(result.Sheet["skills"])[skill])["proficient"]) {
			t.Fatal("missing selected skill", skill)
		}
	}
	if tools := stringsOf(object(result.Sheet["proficiencies"])["tools"]); len(tools) != 5 {
		t.Fatal("missing independent tool choices", tools)
	}
	if !reflect.DeepEqual(input, before) || string(mustJSON(records.byKind)) != string(sources) || !reflect.DeepEqual(result, EvaluateCharacter(input, records, profile)) {
		t.Fatal("evaluation is mutable or nondeterministic")
	}
	// Origin replacement removes only its own grant, without renumbering later choices.
	input.Build.Background = "other"
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:"+trainingOrigin+"#0") || hasChoiceIssue(result, "unavailable-choice:"+trainingFour+"#0") || !truth(object(object(result.Sheet["skills"])["nature"])["proficient"]) || truth(object(object(result.Sheet["skills"])["arcana"])["proficient"]) {
		t.Fatal("replacement transferred or lost another acquisition", result.Issues)
	}
	// Removing the later advancement cannot take the earlier choices with it.
	input.Build.Levels = input.Build.Levels[:4]
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:"+trainingEight+"#2") || hasChoiceIssue(result, "unavailable-choice:"+trainingFour+"#2") || truth(object(object(result.Sheet["skills"])["nature"])["proficient"]) {
		t.Fatal("later removal affected the wrong acquisition", result.Issues)
	}
}

func TestRepeatableFeatChoicesRejectDuplicateSlotsAndForgedOwners(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	input.Build.Choices[len(input.Build.Choices)-1].Value = mustJSON("skill:nature")
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "duplicate-option:"+trainingEight+"#2") || truth(result.Guidance["canSave"]) {
		t.Fatal("duplicate slot accepted", result.Issues)
	}
	input.Build.Choices = append(input.Build.Choices, character.Choice{ID: "forged:feat", Value: mustJSON("training")},
		character.Choice{ID: "feat:training@forged%3Afeat:proficiencies", Value: mustJSON("skill:medicine")})
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:feat:training@forged%3Afeat:proficiencies#0") {
		t.Fatal("an undeclared feat suffix created a grant", result.Issues)
	}
}

func TestRepeatableFeatLegacyChoicesAreNeverDuplicatedOrRebound(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	input.Build.Levels = input.Build.Levels[:1]
	input.Build.Choices = []character.Choice{{ID: "feat:training:proficiencies", Slot: 2, Value: mustJSON("tool:flute")}}
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if result.Inputs.Build.Choices[0].ID != trainingOrigin || result.Inputs.Build.Choices[0].Slot != 2 || !truth(result.Guidance["canSave"]) || !reflect.DeepEqual(input, before) {
		t.Fatal("single legacy acquisition lost its saved slot", result.Inputs, result.Issues)
	}
	input.Grants = []character.Grant{{ID: "reward", Name: "Training reward", Reason: "Study", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, Condition: "always", EffectiveLevel: 1, Feat: &character.Reference{Kind: "feat", ID: "training"}}}
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "ambiguous-feat-choice:feat:training:proficiencies#2") || hasChoiceIssue(result, "unavailable-choice:feat:training:proficiencies#2") || truth(result.Guidance["canSave"]) {
		t.Fatal("ambiguous legacy input was copied or scheduled for withdrawal", result.Issues)
	}
	// Once bound, removing its source must not move that slot to the remaining grant.
	input.Build.Choices[0].ID = trainingOrigin
	input.Build.Background = "other"
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:"+trainingOrigin+"#2") || len(stringsOf(object(result.Sheet["proficiencies"])["tools"])) != 0 {
		t.Fatal("withdrawn choice was rebound to a different acquisition", result.Issues)
	}
}

func TestFeatChoicesFollowSpeciesSelectionAndDMGrantLifetime(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	input.Build.Levels = input.Build.Levels[:1]
	input.Build.Background = "other"
	species := recordByID(records, "species", "dwarf")
	species["grants"] = Object{"choices": []any{Object{"id": "training", "type": "feat", "count": 1, "from": []any{"training"}}}}
	records.byKind["species"]["dwarf"] = mustJSON(species)
	const speciesChoice = "feat:training@species%3Adwarf%3Atraining:proficiencies"
	input.Build.Choices = []character.Choice{{ID: speciesChoice, Value: mustJSON("skill:arcana")}, {ID: "species:dwarf:training", Value: mustJSON("training")},
		{ID: "feat:training@grant%3Areward:proficiencies", Value: mustJSON("tool:drum")}}
	input.Grants = []character.Grant{{ID: "reward", Name: "Second training", Reason: "Study", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, Condition: "always", EffectiveLevel: 1, Feat: &character.Reference{Kind: "feat", ID: "training"}}}
	result := EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) || !truth(object(object(result.Sheet["skills"])["arcana"])["proficient"]) || !contains(stringsOf(object(result.Sheet["proficiencies"])["tools"]), "drum") {
		t.Fatal("child sorted before parent lost its choices", result.Issues)
	}
	input.Grants[0].Active = false
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:feat:training@grant%3Areward:proficiencies#0") || hasChoiceIssue(result, "unavailable-choice:"+speciesChoice+"#0") {
		t.Fatal("DM withdrawal lost another source", result.Issues)
	}
}

func TestNonrepeatableFeatCannotBeAcquiredAgain(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	feat := recordByID(records, "feat", "training")
	feat["repeatable"] = false
	feat["grants"] = Object{}
	records.byKind["feat"]["training"] = mustJSON(feat)
	input.Build.Choices = input.Build.Choices[:4]
	result := EvaluateCharacter(input, records, profile)
	if !hasFeatBlock(result, "training") || truth(result.Guidance["canSave"]) || offeredFeat(result, "asi:fighter:8", "training") {
		t.Fatal("duplicate nonrepeatable feat accepted", result.Issues)
	}
}

func TestRepeatedTrainingReservesEarlierChoicesAndFixedProficiencies(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	input.Build.Choices[len(input.Build.Choices)-1].Value = mustJSON("tool:flute")
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+trainingEight+"#2") || hasChoiceIssue(result, "invalid-option:"+trainingOrigin+"#1") || contains(expertiseOptions(result, trainingEight), "tool:flute") {
		t.Fatal("later training invalidated an earlier choice or offered it again", result.Issues)
	}
	background := recordByID(records, "background", "artisan")
	background["skillProficiencies"] = []any{"arcana"}
	records.byKind["background"]["artisan"] = mustJSON(background)
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "invalid-option:"+trainingOrigin+"#0") {
		t.Fatal("fixed proficiency was offered again", result.Issues)
	}
}

func TestNestedFeatAcquisitionDoesNotDuplicateItsNormalizedSummary(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	input.Build.Levels = input.Build.Levels[:1]
	records.byKind["feat"]["mentor"] = mustJSON(Object{"kind": "feat", "id": "mentor", "name": "Mentor", "grants": Object{"choices": []any{Object{"id": "student", "type": "feat", "count": 1, "from": []any{"training"}}}}})
	records.byKind["background"]["artisan"] = mustJSON(Object{"kind": "background", "id": "artisan", "originFeat": "mentor"})
	input.Build.Choices = []character.Choice{{ID: "feat:mentor:student", Value: mustJSON("training")}, {ID: "feat:training@feat%3Amentor%3Astudent:proficiencies", Value: mustJSON("tool:flute")}}
	result := EvaluateCharacter(input, records, profile)
	if !truth(result.Guidance["canSave"]) || !contains(stringsOf(object(result.Sheet["proficiencies"])["tools"]), "flute") {
		t.Fatal(result.Issues)
	}
	detached := character.Result{}
	decisions := characterDecisions(input, records, profile, &detached)
	normalized := NormalizeBuilderDecisions(decisions, records, profile)
	before, after := BuilderPlan(decisions, records, profile), BuilderPlan(normalized, records, profile)
	if !reflect.DeepEqual(before["creationChoices"], after["creationChoices"]) {
		t.Fatal("normalized summary created a phantom acquisition", before["creationChoices"], after["creationChoices"])
	}
}
