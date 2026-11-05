package rules

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

// focusFixture is a level 1 wizard / level 1 warlock with the magic gear
// records and a few spellcasting foci.
func focusFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, records, profile := magicGearFixture(t)
	for kind, entries := range newMemoryRecords([]Object{
		{"kind": "class", "id": "wizard", "name": "Wizard", "hitDie": "d6", "spellcasting": Object{"type": "full", "ability": "INT", "prepares": "list"}},
		{"kind": "class", "id": "warlock", "name": "Warlock", "hitDie": "d8", "spellcasting": Object{"type": "pact", "ability": "CHA", "prepares": "list"}},
		{"kind": "magic-item", "id": "war-wand", "name": "War wand", "attunement": true, "characterMechanics": "complete",
			"spellcastingFocus": Object{"appliesTo": []any{"spellAttack"}, "bonus": []any{1, 2, 3}}},
		{"kind": "magic-item", "id": "pact-rod", "name": "Pact rod", "attunement": true, "characterMechanics": "complete",
			"spellcastingFocus": Object{"appliesTo": []any{"spellAttack", "spellSaveDC"}, "classes": []any{"warlock"}, "bonus": 2}},
		{"kind": "magic-item", "id": "focus-staff", "name": "Focus staff", "characterMechanics": "complete",
			"baseItem":          Object{"kind": "weapon", "ids": []any{"blade"}, "bonus": []any{1, 2, 3}},
			"spellcastingFocus": Object{"appliesTo": []any{"spellSaveDC", "spellAttack"}}},
	}).byKind {
		for id, record := range entries {
			records.byKind[kind][id] = record
		}
	}
	input.Build.Levels = []character.Level{{ID: "level-one", ClassID: "wizard"}, {ID: "level-two", ClassID: "warlock"}}
	return input, records, profile
}

// focusCasters are the per-class spellcasting rows, wizard first.
func focusCasters(t *testing.T, result character.Result) []character.Caster {
	t.Helper()
	if result.Sheet.Calculation == nil || len(result.Sheet.Calculation.Spellcasting.PerClass) != 2 {
		t.Fatalf("casters: %+v", result.Issues)
	}
	return result.Sheet.Calculation.Spellcasting.PerClass
}

// itemIssues are the result's issues about inventory items; the fixture's
// multiclass prerequisites raise blockers of their own.
func itemIssues(result character.Result) []character.Issue {
	issues := []character.Issue{}
	for _, issue := range result.Issues {
		if strings.HasPrefix(issue.ID, "item-") {
			issues = append(issues, issue)
		}
	}
	return issues
}

// raised is each caster's spell attack and save DC change from the base.
func raised(t *testing.T, base []character.Caster, result character.Result) map[string][2]int {
	t.Helper()
	changes := map[string][2]int{}
	for index, caster := range focusCasters(t, result) {
		changes[caster.ClassID] = [2]int{caster.SpellAttack - base[index].SpellAttack, caster.SaveDC - base[index].SaveDC}
	}
	return changes
}

func TestSpellcastingFocusAddsThePickedBonusToSpells(t *testing.T) {
	input, records, profile := focusFixture(t)
	base := focusCasters(t, EvaluateCharacter(input, records, profile))
	wand := gearItem("wand", "war-wand", "equipped")
	wand.Attuned = true
	input.Play.Inventory = []character.Item{wand}

	// Several bonuses wait for the owner's pick, which the guidance offers.
	result := EvaluateCharacter(input, records, profile)
	if severityOf(result, "item-bonus:wand") != character.SeverityWarning || !reflect.DeepEqual(raised(t, base, result), map[string][2]int{"wizard": {}, "warlock": {}}) {
		t.Fatal("an unpicked focus applied", result.Issues)
	}
	if bonuses := result.Guidance.Equipment["wand"].Bonuses; !reflect.DeepEqual(bonuses, []int{1, 2, 3}) {
		t.Fatal("the guidance does not offer the focus's bonuses", bonuses)
	}

	input.Play.Inventory[0].Bonus = 3
	result = EvaluateCharacter(input, records, profile)
	if len(itemIssues(result)) > 0 || !reflect.DeepEqual(raised(t, base, result), map[string][2]int{"wizard": {3, 0}, "warlock": {3, 0}}) {
		t.Fatal("the picked bonus does not raise every class's spell attacks only", raised(t, base, result), result.Issues)
	}
	for index := range base {
		terms := result.Explanations[fmtIndex("spellcasting.perClass", index)+".spellAttack"].Terms
		last := terms[len(terms)-1]
		if last.Source == nil || *last.Source != (character.Reference{Kind: "magic-item", ID: "war-wand"}) || last.Value != 3 || last.Status != "applied" {
			t.Fatalf("the spell attack explanation does not name the wand: %+v", terms)
		}
	}

	// Without attunement it adds nothing; a bonus it does not offer blocks.
	input.Play.Inventory[0].Attuned = false
	if result = EvaluateCharacter(input, records, profile); !reflect.DeepEqual(focusCasters(t, result), base) {
		t.Fatal("an unattuned focus applied")
	}
	input.Play.Inventory[0].Attuned, input.Play.Inventory[0].Bonus = true, 4
	if result = EvaluateCharacter(input, records, profile); severityOf(result, "item-bonus:wand") != character.SeverityBlocker {
		t.Fatal("a bonus the focus does not offer was accepted", result.Issues)
	}

	// Identical foci do not stack: only the first copy applies.
	second := wand
	second.ID, second.Bonus = "wand-two", 3
	input.Play.Inventory = []character.Item{wand, second}
	input.Play.Inventory[0].Bonus = 1
	if result = EvaluateCharacter(input, records, profile); !reflect.DeepEqual(raised(t, base, result), map[string][2]int{"wizard": {1, 0}, "warlock": {1, 0}}) {
		t.Fatal("two copies of one focus stacked", raised(t, base, result))
	}
}

func TestSpellcastingFocusRaisesOnlyItsClasses(t *testing.T) {
	input, records, profile := focusFixture(t)
	base := focusCasters(t, EvaluateCharacter(input, records, profile))
	rod := gearItem("rod", "pact-rod", "equipped")
	rod.Attuned = true
	input.Play.Inventory = []character.Item{rod}
	result := EvaluateCharacter(input, records, profile)
	if len(itemIssues(result)) > 0 || !reflect.DeepEqual(raised(t, base, result), map[string][2]int{"wizard": {}, "warlock": {2, 2}}) {
		t.Fatal("the rod does not raise only warlock spells", raised(t, base, result), result.Issues)
	}
}

func TestWeaponFocusSharesOnePickedBonus(t *testing.T) {
	input, records, profile := focusFixture(t)
	input.Play.Inventory = []character.Item{plainItem("plain", "weapon", "blade")}
	plain := EvaluateCharacter(input, records, profile)
	base := focusCasters(t, plain)
	plainRow, _ := weaponOf(plain, "plain")
	staff := gearItem("staff", "focus-staff", "equipped")
	staff.Bonus = 2
	input.Play.Inventory = []character.Item{staff}
	result := EvaluateCharacter(input, records, profile)
	row, _ := weaponOf(result, "staff")
	if len(itemIssues(result)) > 0 || row == nil || integer(row["attackBonus"], 0) != integer(plainRow["attackBonus"], 0)+2 {
		t.Fatal("the staff's weapon bonus is missing", row, result.Issues)
	}
	if changes := raised(t, base, result); !reflect.DeepEqual(changes, map[string][2]int{"wizard": {2, 2}, "warlock": {2, 2}}) {
		t.Fatal("the staff's spells do not share its bonus", changes)
	}
}

func TestSpellcastingFocusDeclarationsAreReadAllOrNothing(t *testing.T) {
	input, records, profile := focusFixture(t)
	base := focusCasters(t, EvaluateCharacter(input, records, profile))
	both := []any{"spellAttack", "spellSaveDC"}
	blade := Object{"kind": "weapon", "ids": []any{"blade"}, "bonus": 1}
	for _, declared := range []struct {
		focus any
		base  Object
	}{
		{"focus", nil},
		{Object{"bonus": 1}, nil},
		{Object{"appliesTo": []any{}, "bonus": 1}, nil},
		{Object{"appliesTo": []any{"spellAttack", "spellAttack"}, "bonus": 1}, nil},
		{Object{"appliesTo": []any{"attack"}, "bonus": 1}, nil},
		{Object{"appliesTo": both}, nil},
		{Object{"appliesTo": both, "bonus": 0}, nil},
		{Object{"appliesTo": both, "bonus": []any{1, 1}}, nil},
		{Object{"appliesTo": both, "bonus": 1, "classes": []any{}}, nil},
		{Object{"appliesTo": both, "bonus": 1, "extra": true}, nil},
		{Object{"appliesTo": both, "bonus": 1}, blade},
		{Object{"appliesTo": both}, Object{"kind": "weapon", "ids": []any{"blade"}}},
	} {
		record := Object{"kind": "magic-item", "id": "odd", "name": "Odd", "characterMechanics": "complete", "spellcastingFocus": declared.focus}
		if declared.base != nil {
			record["baseItem"] = declared.base
		}
		records.byKind["magic-item"]["odd"] = mustJSON(record)
		odd := gearItem("odd", "odd", "equipped")
		odd.Bonus = 1
		input.Play.Inventory = []character.Item{odd}
		result := EvaluateCharacter(input, records, profile)
		if severityOf(result, "item-effects:odd") != character.SeverityBlocker || !reflect.DeepEqual(focusCasters(t, result), base) {
			t.Fatal("a malformed spellcastingFocus was read", declared, result.Issues)
		}
	}

	// A text-only item may not be a focus; a base pick on a focus that is no
	// weapon or armor blocks.
	records.byKind["magic-item"]["odd"] = mustJSON(Object{"kind": "magic-item", "id": "odd", "name": "Odd", "characterMechanics": "narrative", "spellcastingFocus": Object{"appliesTo": both, "bonus": 1}})
	input.Play.Inventory = []character.Item{gearItem("odd", "odd", "equipped")}
	if result := EvaluateCharacter(input, records, profile); severityOf(result, "item-effects:odd") != character.SeverityBlocker {
		t.Fatal("a narrative item was a focus", result.Issues)
	}
	rod := gearItem("rod", "pact-rod", "equipped")
	rod.Attuned, rod.BaseItemID = true, "blade"
	input.Play.Inventory = []character.Item{rod}
	if result := EvaluateCharacter(input, records, profile); severityOf(result, "item-base:rod") != character.SeverityBlocker {
		t.Fatal("a base pick on a focus without a base was accepted", result.Issues)
	}
}
