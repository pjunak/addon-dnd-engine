package rules

import (
	"github.com/pjunak/addon-dnd-engine/character"
	"testing"
)

func TestIncrementalBuildCanSaveWithoutAllowingInvalidValues(t *testing.T) {
	_, records, profile := characterFixture(t)
	input := character.Blank()
	check := func(want bool) {
		t.Helper()
		result := EvaluateCharacter(input, records, profile)
		if truth(result.Guidance["canSave"]) != want {
			t.Fatalf("canSave=%v; issues=%+v", result.Guidance["canSave"], result.Issues)
		}
	}
	check(true)
	input.Build.BaseScores["STR"] = profile.Constants.PointBuy.Min
	check(true)
	input.Build.BaseScores["STR"] = profile.Constants.PointBuy.Max + 1
	check(false)
	for _, ability := range Abilities {
		input.Build.BaseScores[ability] = profile.Constants.PointBuy.Max
	}
	check(false)
	input.Build.Method = "array"
	input.Build.BaseScores = map[string]int{"STR": 15}
	check(true)
	input.Build.BaseScores["DEX"] = 15
	check(false)
}

func TestClassOptionsRespectMulticlassPrerequisites(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["class"]["mage"] = mustJSON(Object{"kind": "class", "id": "mage", "name": "Mage", "hitDie": "d6", "multiclassRequirements": Object{"INT": 13}})
	fighter := recordByID(records, "class", "fighter")
	fighter["multiclassRequirements"] = Object{"STR|DEX": 13}
	records.byKind["class"]["fighter"] = mustJSON(fighter)
	result := EvaluateCharacter(input, records, profile)
	for _, option := range objects(result.Guidance["classOptions"]) {
		if text(option["id"]) == "mage" {
			t.Fatal("offered unqualified multiclass")
		}
	}
	input.Build.Method = "point-buy"
	input.Build.BaseScores["INT"] = 13
	result = EvaluateCharacter(input, records, profile)
	found := false
	for _, option := range objects(result.Guidance["classOptions"]) {
		found = found || text(option["id"]) == "mage"
	}
	if !found {
		t.Fatal("qualified class absent", result.Guidance["classOptions"])
	}
}

func TestEquipmentControlsRespectAvailabilityAndAttunement(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	extra := newMemoryRecords([]Object{
		{"kind": "magic-item", "id": "amulet", "name": "Amulet", "attunement": true, "characterMechanics": "complete", "characterEffects": []any{}},
		{"kind": "magic-item", "id": "unknown", "name": "Unstructured item"},
		{"kind": "weapon", "id": "dagger", "name": "Dagger"},
	})
	for kind, rows := range extra.byKind {
		records.byKind[kind] = rows
	}
	ref := &character.Reference{Kind: "magic-item", ID: "amulet"}
	input.Play.Inventory = []character.Item{
		{ID: "first", Reference: ref, Name: "Amulet", Quantity: 1, Location: "carried", Attuned: true},
		{ID: "second", Reference: ref, Name: "Spare amulet", Quantity: 1, Location: "carried"},
		{ID: "empty", Reference: &character.Reference{Kind: "weapon", ID: "dagger"}, Name: "Dagger", Quantity: 0, Location: "carried"},
		{ID: "weapon", Reference: &character.Reference{Kind: "weapon", ID: "dagger"}, Name: "Dagger", Quantity: 1, Location: "carried"},
		{ID: "unknown", Reference: &character.Reference{Kind: "magic-item", ID: "unknown"}, Name: "Unknown", Quantity: 1, Location: "carried"},
	}
	profile.Constants.Character.UniqueAttunement = true
	result := EvaluateCharacter(input, records, profile)
	guidance := object(result.Guidance["equipment"])
	if truth(object(guidance["second"])["canAttune"]) || truth(object(guidance["empty"])["canEquip"]) || truth(object(guidance["unknown"])["canEquip"]) || truth(object(guidance["weapon"])["canAttune"]) || !truth(object(guidance["weapon"])["canEquip"]) {
		t.Fatal("offered invalid equipment action", guidance)
	}
	input.Play.Inventory[0].Attuned = false
	guidance = object(EvaluateCharacter(input, records, profile).Guidance["equipment"])
	if !truth(object(guidance["second"])["canAttune"]) {
		t.Fatal("available attunement option missing", guidance)
	}
	result.Sheet["attunement"] = Object{"count": 3, "limit": 3}
	if truth(object(characterEquipmentOptions(input, records, profile, &result)["second"])["canAttune"]) {
		t.Fatal("offered attunement beyond capacity")
	}
}
