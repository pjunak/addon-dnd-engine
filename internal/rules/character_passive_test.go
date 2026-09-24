package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func passiveFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "second", ClassID: "fighter"})
	rolled := 3
	input.Build.Levels[1].HitPoints = &rolled
	input.Play.HP, input.Play.TemporaryHP = 7, 4
	input.Play.Currency["gp"], input.Notes = 23, "Keep these notes"
	extra := newMemoryRecords([]Object{
		{"kind": "feat", "id": "vitality", "name": "Vitality", "grants": Object{"hpBonus": 17, "hpPerLevel": 2, "speedBonus": 12, "senses": Object{"blindsight": 10, "truesight": 40}}},
		{"kind": "feat", "id": "guard", "name": "Guard", "grants": Object{"acBonuses": []any{Object{"add": 2, "requires": Object{"armorTypes": []any{"light", "medium", "heavy"}}}}}},
		{"kind": "armor", "id": "soft-suit", "name": "Soft suit", "armorType": "light", "baseAC": 11},
		{"kind": "armor", "id": "linked-suit", "name": "Linked suit", "armorType": "medium", "baseAC": 14, "dexCap": 2},
		{"kind": "magic-item", "id": "hard-suit", "name": "Hard suit", "armorType": "heavy", "baseAC": 18, "dexCap": 0, "characterMechanics": "narrative"},
		{"kind": "armor", "id": "round-guard", "name": "Round guard", "armorType": "shield", "acBonus": 2},
	})
	for kind, entries := range extra.byKind {
		records.byKind[kind] = entries
	}
	for _, id := range []string{"vitality", "guard"} {
		input.Grants = append(input.Grants, character.Grant{ID: id, Name: id, Reason: "Training", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always", Feat: &character.Reference{Kind: "feat", ID: id}})
	}
	return input, records, profile
}

func TestPassiveBonusesRecalculateWithoutChangingAuthoredState(t *testing.T) {
	input, records, profile := passiveFixture(t)
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	// Two recorded level gains: (10+1+2) + (3+1+2), then one fixed 17.
	if !result.Ready || integer(object(result.Sheet["derived"])["maxHp"], 0) != 36 || integer(result.Sheet["speed"], 0) != 42 {
		t.Fatal("fixed/per-level grants or recorded dice were lost", result.Issues, result.Sheet["hp"], result.Sheet["speed"])
	}
	if !reflect.DeepEqual(input, before) || !reflect.DeepEqual(result.Inputs, before) || !reflect.DeepEqual(result, EvaluateCharacter(input, records, profile)) {
		t.Fatal("evaluation changed authored state or was nondeterministic")
	}
	if !passiveTerm(result, "derived.maxHp", "vitality", "") || !passiveTerm(result, "derived.speed", "vitality", "") {
		t.Fatal("numeric bonuses lost source explanations")
	}
	if integer(object(result.Sheet["senses"])["blindsight"], 0) != 10 || integer(object(result.Sheet["senses"])["truesight"], 0) != 40 {
		t.Fatal("sense grants missing")
	}
	input.Grants[0].Active = false
	removed := EvaluateCharacter(input, records, profile)
	if integer(object(removed.Sheet["derived"])["maxHp"], 0) != 15 || integer(removed.Sheet["speed"], 0) != 30 || object(removed.Sheet["senses"])["blindsight"] != nil {
		t.Fatal("withdrawn grants survived", removed.Sheet)
	}
	if !reflect.DeepEqual(removed.Inputs.Play, before.Play) {
		t.Fatal("withdrawal edited play state")
	}
}

func TestPassiveArmorBonusFollowsBodyArmorAndRetainsInactiveEvidence(t *testing.T) {
	for _, scenario := range []struct {
		kind, id, location string
		quantity, ac       int
		applied            bool
	}{
		{"armor", "soft-suit", "carried", 1, 12, false}, {"armor", "soft-suit", "stored", 1, 12, false},
		{"armor", "soft-suit", "equipped", 1, 15, true}, {"armor", "soft-suit", "equipped", 0, 12, false}, {"armor", "linked-suit", "equipped", 1, 18, true},
		{"magic-item", "hard-suit", "equipped", 1, 20, true}, {"armor", "round-guard", "equipped", 1, 14, false},
	} {
		t.Run(scenario.id+"/"+scenario.location, func(t *testing.T) {
			input, records, profile := passiveFixture(t)
			input.Play.Inventory = []character.Item{{ID: "worn", Name: scenario.id, Reference: &character.Reference{Kind: scenario.kind, ID: scenario.id}, Location: scenario.location, Quantity: scenario.quantity}}
			result := EvaluateCharacter(input, records, profile)
			if integer(object(result.Sheet["derived"])["armorClass"], 0) != scenario.ac {
				t.Fatal(result.Sheet["ac"])
			}
			status := "inactive"
			if scenario.applied {
				status = "applied"
			}
			if !passiveTerm(result, "derived.armorClass", "guard", status) {
				t.Fatal("missing condition/source", result.Explanations["derived.armorClass"])
			}
			if !reflect.DeepEqual(result.Inputs.Play, input.Play) {
				t.Fatal("equipment was mutated")
			}
			input.Grants[1].Active = false
			removed := EvaluateCharacter(input, records, profile)
			want := scenario.ac
			if scenario.applied {
				want -= 2
			}
			if integer(object(removed.Sheet["derived"])["armorClass"], 0) != want || passiveTerm(removed, "derived.armorClass", "guard", status) {
				t.Fatal("withdrawn armor bonus survived")
			}
		})
	}
}

func TestSourceBonusesComposeWithLineageAndTypedEffects(t *testing.T) {
	input, records, profile := passiveFixture(t)
	species := recordByID(records, "species", "dwarf")
	species["grants"] = Object{"hpPerLevel": 1, "speedBonus": 3, "senses": Object{"blindsight": 30}}
	species["lineages"] = []any{Object{"id": "swift", "grants": Object{"speedBonus": 5, "hpBonus": 4}}}
	records.byKind["species"]["dwarf"] = mustJSON(species)
	input.Build.Lineage = "swift"
	input.Grants = append(input.Grants, character.Grant{ID: "adjustment", Name: "Adjustment", Reason: "Test", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always",
		Effects: []character.Effect{{Target: "maxHp", Mode: "add", Value: 3}, {Target: "speed", Mode: "minimum", Value: 55}, {Target: "sense", Key: "blindsight", Mode: "add", Value: 5}}})
	result := EvaluateCharacter(input, records, profile)
	if integer(object(result.Sheet["derived"])["maxHp"], 0) != 45 || integer(result.Sheet["speed"], 0) != 55 || integer(object(result.Sheet["senses"])["blindsight"], 0) != 35 {
		t.Fatal("bonuses stacked twice or applied after typed effects", result.Sheet["hp"], result.Sheet["speed"], result.Sheet["senses"])
	}
	input.Grants = input.Grants[:2]
	if speed := integer(EvaluateCharacter(input, records, profile).Sheet["speed"], 0); speed != 50 {
		t.Fatal("lineage counted more than once", speed)
	}
}

func TestUnsupportedPassiveArmorConditionsFailClosed(t *testing.T) {
	for _, requirement := range []any{nil, "armor", Object{}, Object{"armorTypes": []any{}}, Object{"armorTypes": []any{"shield"}}, Object{"armorTypes": []any{"light"}, "unknown": true}} {
		input, records, profile := passiveFixture(t)
		feat := recordByID(records, "feat", "guard")
		objects(object(feat["grants"])["acBonuses"])[0]["requires"] = requirement
		records.byKind["feat"]["guard"] = mustJSON(feat)
		input.Play.Inventory = []character.Item{{ID: "worn", Name: "Suit", Reference: &character.Reference{Kind: "armor", ID: "soft-suit"}, Location: "equipped", Quantity: 1}}
		if ac := integer(object(EvaluateCharacter(input, records, profile).Sheet["derived"])["armorClass"], 0); ac != 13 {
			t.Fatalf("unknown requirement granted AC: %v => %d", requirement, ac)
		}
	}
}

func TestPassiveBonusesFollowSelectedClassPackages(t *testing.T) {
	input, records, profile := classGrantFixture(t)
	feature := recordByID(records, "feature", "martial-training")
	option := object(object(objects(object(feature["grants"])["choicePackages"])[0]["options"])["style"])
	option["hpBonus"], option["speedBonus"] = 7, 4
	records.byKind["feature"]["martial-training"] = mustJSON(feature)
	input.Build.Choices = []character.Choice{{ID: "training-path", Value: mustJSON("style")}, {ID: "training-feat", Value: mustJSON("guard")}}
	result := EvaluateCharacter(input, records, profile)
	if integer(object(result.Sheet["derived"])["maxHp"], 0) != 25 || integer(result.Sheet["speed"], 0) != 34 ||
		!passiveTerm(result, "derived.maxHp", "martial-training", "") {
		t.Fatal("selected class package did not grant or explain its fixed benefit", result.Sheet["hp"])
	}
	input.Build.Choices = []character.Choice{{ID: "training-path", Value: mustJSON("cantrips")}}
	result = EvaluateCharacter(input, records, profile)
	if integer(object(result.Sheet["derived"])["maxHp"], 0) != 18 || integer(result.Sheet["speed"], 0) != 30 ||
		passiveTerm(result, "derived.maxHp", "martial-training", "") {
		t.Fatal("inactive class package retained its passive benefit", result.Sheet["hp"])
	}
}

func TestArmorReferenceDoesNotFallBackToADifferentCatalogKind(t *testing.T) {
	input, records, profile := passiveFixture(t)
	records.byKind["magic-item"]["soft-suit"] = mustJSON(Object{"kind": "magic-item", "id": "soft-suit", "name": "Decorative suit", "characterMechanics": "narrative"})
	input.Play.Inventory = []character.Item{{ID: "decorative", Name: "Soft suit", Reference: &character.Reference{Kind: "magic-item", ID: "soft-suit"}, Location: "equipped", Quantity: 1}}
	result := EvaluateCharacter(input, records, profile)
	if integer(object(result.Sheet["derived"])["armorClass"], 0) != 12 || !passiveTerm(result, "derived.armorClass", "guard", "inactive") {
		t.Fatal("an unrelated armor record with the same ID granted AC", result.Sheet["ac"])
	}
}

func passiveTerm(result character.Result, path, id, status string) bool {
	for _, term := range result.Explanations[path].Terms {
		if term.Source != nil && term.Source.ID == id && (status == "" || term.Status == status) {
			return true
		}
	}
	return false
}
