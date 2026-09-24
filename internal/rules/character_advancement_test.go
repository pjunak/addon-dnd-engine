package rules

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestUnrestrictedAdvancementsUseAcquiredCharacterLevel(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	profile.Builder.AbilityScoreAdvancement.FeatCategories = []string{}
	profile.Builder.AbilityScoreAdvancement.CategoriesByLevel = map[string][]string{}
	records.byKind["feat"]["early"] = mustJSON(Object{"kind": "feat", "id": "early", "category": "origin"})
	records.byKind["feat"]["ordinary"] = mustJSON(Object{"kind": "feat", "id": "ordinary", "category": "general"})
	records.byKind["feat"]["late"] = mustJSON(Object{"kind": "feat", "id": "late", "category": "customBoon",
		"prerequisites": Object{"level": 19}, "grants": Object{"abilityScoreIncrease": Object{"choose": 1, "amount": 1, "from": anyStrings(Abilities[:]), "cap": 30}}})
	records.byKind["feat"]["reviewed"] = mustJSON(Object{"kind": "feat", "id": "reviewed", "category": "customGift", "prerequisites": Object{"text": "Requires a recorded ruling"}})
	fighter := recordByID(records, "class", "fighter")
	fighter["multiclassPrerequisites"] = Object{}
	records.byKind["class"]["fighter"] = mustJSON(fighter)
	records.byKind["class"]["scout"] = mustJSON(Object{"kind": "class", "id": "scout", "hitDie": "d8", "multiclassPrerequisites": Object{}})
	fighterLevels := append([]character.Level{}, input.Build.Levels[:4]...)
	input.Build.Levels = append([]character.Level{}, fighterLevels[:3]...)
	for i := 0; i < 15; i++ {
		input.Build.Levels = append(input.Build.Levels, character.Level{ID: fmt.Sprintf("scout-%d", i), ClassID: "scout"})
	}
	input.Build.Levels = append(input.Build.Levels, fighterLevels[3])
	input.Build.Choices = []character.Choice{
		{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("late")},
		{ID: "asi:fighter:4:featability", Value: mustJSON(Object{"STR": 1})},
	}
	input.Notes = "Keep advancement notes"
	input.Play.Currency["gp"] = 37
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	for _, id := range []string{"early", "ordinary", "late"} {
		if !offeredFeat(result, "asi:fighter:4", id) {
			t.Fatal("qualified category missing", id)
		}
	}
	if offeredFeat(result, "asi:fighter:4", "reviewed") || hasFeatBlock(result, "late") || !truth(result.Guidance["canSave"]) {
		t.Fatal("unrestricted categories bypassed prerequisites", result.Issues)
	}
	if integer(object(object(result.Sheet["abilities"])["STR"])["cap"], 0) != 30 || !reflect.DeepEqual(result.Inputs, before) || !reflect.DeepEqual(input, before) {
		t.Fatal("explicit feat cap or authored inputs changed")
	}
	input.Build.Levels = append(fighterLevels, before.Build.Levels[3:18]...)
	result = EvaluateCharacter(input, records, profile)
	if offeredFeat(result, "asi:fighter:4", "late") || !hasFeatBlock(result, "late") || truth(result.Guidance["canSave"]) {
		t.Fatal("later levels retroactively qualified an earlier feat", result.Issues)
	}
}

func TestFeatOptionFilteringUsesClassAndSpellcasterPredicates(t *testing.T) {
	for _, kind := range []string{"class", "spellcaster"} {
		t.Run(kind, func(t *testing.T) {
			input, records, profile := progressionChoiceInput(t)
			predicate := Object{"classes": Object{"fighter": 4}}
			if kind == "spellcaster" {
				predicate = Object{"spellcaster": true}
				records.byKind["spell"] = map[string]json.RawMessage{"light": mustJSON(Object{"kind": "spell", "id": "light", "name": "Light", "level": 0})}
				species := recordByID(records, "species", input.Build.Species)
				species["grants"] = Object{"castingAbility": Object{"fixed": "WIS"}, "spells": []any{Object{"ids": []any{"light"}, "alwaysPrepared": true}}}
				records.byKind["species"][input.Build.Species] = mustJSON(species)
			}
			records.byKind["feat"]["qualified"] = mustJSON(Object{"kind": "feat", "id": "qualified", "category": "general", "prerequisites": predicate})
			input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("qualified")}}
			result := EvaluateCharacter(input, records, profile)
			if hasFeatBlock(result, "qualified") || !offeredFeat(result, "asi:fighter:4", "qualified") || !truth(result.Guidance["canSave"]) {
				t.Fatal("guidance disagrees with valid acquired-state prerequisite", result.Issues)
			}
			if kind == "class" {
				predicate["classes"] = Object{"fighter": 5}
			} else {
				species := recordByID(records, "species", input.Build.Species)
				delete(species, "grants")
				records.byKind["species"][input.Build.Species] = mustJSON(species)
			}
			records.byKind["feat"]["qualified"] = mustJSON(Object{"kind": "feat", "id": "qualified", "category": "general", "prerequisites": predicate})
			result = EvaluateCharacter(input, records, profile)
			if !hasFeatBlock(result, "qualified") || offeredFeat(result, "asi:fighter:4", "qualified") || truth(result.Guidance["canSave"]) {
				t.Fatal("an unmet acquisition prerequisite was offered", result.Issues)
			}
		})
	}
}

func TestSubclassFeaturePrerequisitesUseCanonicalAcquiredIdentity(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	input.Build.Levels = input.Build.Levels[:4]
	input.Build.Subclasses = map[string]string{"fighter": "mystic"}
	records.byKind["subclass"] = map[string]json.RawMessage{
		"mystic": mustJSON(Object{"id": "mystic", "classId": "fighter", "features": []any{
			Object{"id": "casting", "name": "Casting", "level": 3},
			Object{"id": "future", "name": "Future", "level": 7},
			Object{"id": "inline-only", "name": "Inline only", "level": 3},
		}}),
	}
	records.byKind["feature"] = map[string]json.RawMessage{
		"mystic-casting": mustJSON(Object{"id": "mystic-casting", "localId": "casting", "name": "Casting", "classId": "fighter", "subclassId": "mystic", "level": 3}),
		"mystic-future":  mustJSON(Object{"id": "mystic-future", "localId": "future", "name": "Future", "classId": "fighter", "subclassId": "mystic", "level": 7}),
		"mystic-extra":   mustJSON(Object{"id": "mystic-extra", "name": "Extra", "classId": "fighter", "subclassId": "mystic", "level": 3}),
		"other-casting":  mustJSON(Object{"id": "other-casting", "localId": "casting", "name": "Casting", "classId": "fighter", "subclassId": "other", "level": 3}),
	}
	records.byKind["feat"]["qualified"] = mustJSON(Object{"id": "qualified", "category": "general", "prerequisites": Object{"feature": "mystic-casting"}})
	input.Build.Choices = []character.Choice{{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("qualified")}}
	result := EvaluateCharacter(input, records, profile)
	if hasFeatBlock(result, "qualified") || !offeredFeat(result, "asi:fighter:4", "qualified") || !truth(result.Guidance["canSave"]) {
		t.Fatal("canonical subclass prerequisite did not resolve", result.Issues)
	}
	for _, level := range []int{2, 4} {
		sheet := Hydrate(Object{"classes": []any{Object{"classId": "fighter", "subclass": "mystic", "level": level}}}, records, &profile).Sheet
		counts := map[string]int{}
		for _, feature := range objects(sheet["features"]) {
			counts[text(feature["id"])]++
		}
		for _, id := range []string{"mystic-casting", "mystic-extra", "inline-only"} {
			if counts[id] != conditionalInt(level >= 3, 1) {
				t.Fatal("wrong acquired feature count", level, id, counts)
			}
		}
		for _, id := range []string{"casting", "mystic-future", "future", "other-casting"} {
			if counts[id] != 0 {
				t.Fatal("local, future or unrelated identity leaked", level, counts)
			}
		}
	}
}

func TestWithdrawnFeatAbilityIsAnUnavailableChoice(t *testing.T) {
	input, records, profile := progressionChoiceInput(t)
	records.byKind["feat"]["boost"] = mustJSON(Object{"id": "boost", "category": "general", "grants": Object{
		"abilityScoreIncrease": Object{"choose": 1, "amount": 1, "from": []any{"INT"}, "cap": 30},
	}})
	records.byKind["feat"]["plain"] = mustJSON(Object{"id": "plain", "category": "general"})
	input.Build.Choices = []character.Choice{
		{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("boost")},
		{ID: "asi:fighter:4:featability", Value: mustJSON(Object{"INT": 1})},
	}
	before := EvaluateCharacter(input, records, profile)
	if !truth(before.Guidance["canSave"]) {
		t.Fatal(before.Issues)
	}
	input.Build.Choices[1].Value = mustJSON("plain")
	authored := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:asi:fighter:4:featability#0") ||
		hasChoiceIssue(result, "ability-assignment:asi:fighter:4:featability#0") || truth(result.Guidance["canSave"]) {
		t.Fatal("withdrawn source was not marked for saved-choice repair", result.Issues)
	}
	if !reflect.DeepEqual(input, authored) || !reflect.DeepEqual(result.Inputs, authored) {
		t.Fatal("validation silently changed authored inputs")
	}
	input.Build.Choices = input.Build.Choices[:2]
	if result := EvaluateCharacter(input, records, profile); !truth(result.Guidance["canSave"]) {
		t.Fatal("explicit withdrawal did not repair the build", result.Issues)
	}
}

func TestAcquiredFeatProjectionPreservesRepeatCountsAndWithdrawal(t *testing.T) {
	input, records, profile := repeatableFixture(t)
	records.byKind["feat"]["unselected"] = mustJSON(Object{"id": "unselected", "name": "Unselected", "category": "general"})
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	want := []any{Object{"id": "training", "name": "Training", "count": 3}}
	if !reflect.DeepEqual(result.Sheet["feats"], want) || !reflect.DeepEqual(result.Inputs, before) {
		t.Fatal("acquired feat output lost counts or included catalog candidates", result.Sheet["feats"])
	}
	input.Build.Background = "other"
	result = EvaluateCharacter(input, records, profile)
	want = []any{Object{"id": "training", "name": "Training", "count": 2}}
	if !reflect.DeepEqual(result.Sheet["feats"], want) {
		t.Fatal("origin withdrawal did not update acquired count", result.Sheet["feats"])
	}
}
