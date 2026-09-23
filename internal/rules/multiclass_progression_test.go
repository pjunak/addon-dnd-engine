package rules

import (
	"reflect"
	"testing"
)

func TestPactMagicKeepsTheOnlySpellcastingClassTable(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	records.byKind["subclass"] = newMemoryRecords([]Object{{"kind": "subclass", "id": "arcane-guard", "classId": "fighter", "spellcasting": Object{"type": "third", "ability": "INT", "prepares": "list"}, "progression": []any{
		Object{"level": 4, "cantripsKnown": 2, "preparedSpells": 4, "spellSlots": []any{3, 0, 0, 0}},
		Object{"level": 7, "cantripsKnown": 2, "preparedSpells": 5, "spellSlots": []any{4, 2, 0, 0}},
	}}}).byKind["subclass"]
	for _, level := range []int{4, 7} {
		knight := Object{"classId": "fighter", "level": level, "subclass": "arcane-guard"}
		alone := Hydrate(Object{"classes": []any{knight}}, records, &profile).Sheet
		for _, reversed := range []bool{false, true} {
			classes := []any{knight, Object{"classId": "warlock", "level": 2}}
			if reversed {
				classes[0], classes[1] = classes[1], classes[0]
			}
			input := Object{"classes": classes}
			before := mustJSON(input)
			mixed := Hydrate(input, records, &profile).Sheet
			want, got := object(alone["spellcasting"])["slots"], object(mixed["spellcasting"])["slots"]
			if !reflect.DeepEqual(want, got) {
				t.Errorf("level %d reversed=%v: slots %v, want own table %v", level, reversed, got, want)
			}
			if string(before) != string(mustJSON(input)) {
				t.Fatal("mutated caller input")
			}
		}
	}
}

func TestLaterClassMissingProficienciesDoesNotGrantStartingEquipmentTraining(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	fighter := recordByID(records, "class", "fighter")
	fighter["startingProficiencies"] = Object{"armor": []any{"heavy"}, "weapons": []any{"martial"}, "tools": []any{"flute"}}
	delete(fighter, "multiclassProficiencies")
	records.byKind["class"]["fighter"] = mustJSON(fighter)
	sheet := Hydrate(Object{"classes": []any{Object{"classId": "wizard", "level": 1}, Object{"classId": "fighter", "level": 1}}, "inventory": []any{Object{"ref": "longsword", "location": "equipped"}}}, records, &profile).Sheet
	prof := object(sheet["proficiencies"])
	for _, key := range []string{"armor", "weapons", "tools"} {
		if len(values(prof[key])) != 0 {
			t.Errorf("secondary starting %s leaked: %v", key, prof[key])
		}
	}
	if truth(objects(sheet["weapons"])[0]["proficient"]) {
		t.Fatal("weapon attack received starting proficiency")
	}
}

func TestSubclassFeatureProgressionDoesNotReplaceClassSpellProgression(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	records.byKind["subclass"] = newMemoryRecords([]Object{{"kind": "subclass", "id": "sage", "classId": "wizard", "progression": []any{Object{"level": 3, "features": []any{"Insight"}}}}}).byKind["subclass"]
	sheet := Hydrate(Object{"classes": []any{Object{"classId": "wizard", "level": 5, "subclass": "sage"}}}, records, &profile).Sheet
	caster := objects(object(sheet["spellcasting"])["perClass"])[0]
	if integer(caster["cantripsKnown"], 0) != 4 || integer(caster["preparedLimit"], 0) != 9 {
		t.Fatalf("subclass removed class spell capacity: %v", caster)
	}
}

func TestSharedSlotsDoNotRaiseIndividualPreparedSpellLevels(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	sheet := Hydrate(Object{"classes": []any{Object{"classId": "paladin", "level": 4}, Object{"classId": "sorcerer", "level": 3}}}, records, &profile).Sheet
	casting := object(sheet["spellcasting"])
	if !reflect.DeepEqual(integersOf(casting["slots"]), []int{4, 3, 2}) {
		t.Fatal(casting)
	}
	for _, caster := range objects(casting["perClass"]) {
		want := 2
		if text(caster["classId"]) == "paladin" {
			want = 1
		}
		if integer(caster["maxSpellLevel"], 0) != want {
			t.Fatal("shared slots changed per-class preparation", caster)
		}
	}
}
