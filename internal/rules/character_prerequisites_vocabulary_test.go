package rules

import "testing"

func TestFeatPrerequisitePredicates(t *testing.T) {
	sheet := Object{
		"featIdentities": []any{Object{"id": "tyro-of-the-gauntlet", "category": "origin"}, Object{"id": "mark-of-storm", "category": "dragonmark"}},
		"proficiencies":  Object{"armor": []any{"light", "medium", "shields"}, "weapons": []any{"simple"}},
		"spellcasting":   Object{"perClass": []any{Object{"classId": "warlock"}}, "granted": []any{}},
	}
	for _, check := range []struct {
		predicate Object
		want      bool
	}{
		{Object{"feat": "tyro-of-the-gauntlet"}, true},
		{Object{"feat": "zhentarim-ruffian"}, false},
		{Object{"featCategory": "dragonmark"}, true},
		{Object{"withoutFeatCategory": "dragonmark"}, false},
		{Object{"withoutFeatCategory": "darkGift"}, true},
		{Object{"armorTraining": "medium"}, true},
		{Object{"armorTraining": "shield"}, true},
		{Object{"armorTraining": "heavy"}, false},
		{Object{"weaponTraining": "martial"}, false},
		{Object{"weaponTraining": "simple"}, true},
		{Object{"spellcastingFeature": true}, true},
	} {
		matched, known := prerequisiteMatches(check.predicate, sheet)
		if !known || matched != check.want {
			t.Errorf("%v = %v (known %v), want %v", check.predicate, matched, known, check.want)
		}
	}
	// Spells granted by a feat alone are not a Spellcasting feature.
	featCaster := Object{"spellcasting": Object{"perClass": []any{}, "granted": []any{Object{"ref": "light", "level": 0, "source": Object{"type": "feat"}}}}}
	if matched, _ := prerequisiteMatches(Object{"spellcastingFeature": true}, featCaster); matched {
		t.Error("a feat-granted cantrip counted as a Spellcasting feature")
	}
	if _, known := prerequisiteMatches(Object{"armorTraining": "plate"}, sheet); known {
		t.Error("accepted an unknown armor category")
	}
	if !prerequisiteNeedsSheet(Object{"level": 4, "feat": "x"}, 0) || prerequisiteNeedsSheet(Object{"level": 4}, 0) {
		t.Error("sheet need is wrong")
	}
}
