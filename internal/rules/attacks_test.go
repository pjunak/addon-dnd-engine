package rules

import (
	"reflect"
	"testing"
)

func attackRecords() memoryRecords {
	records := classMechanicsRecords()
	put := func(kind string, record Object) {
		records.byKind[kind][text(record["id"])] = mustJSON(record)
	}
	put("class", Object{"kind": "class", "id": "brute", "name": "Brute", "hitDie": "d12",
		"startingProficiencies": Object{"weapons": []any{"simple", "martial"}},
		"tableColumns":          []any{Object{"key": "rage-damage", "progression": []any{Object{"level": 1, "value": 2}, Object{"level": 9, "value": 3}}}}})
	put("feature", Object{"kind": "feature", "id": "brute-rage", "name": "Rage", "classId": "brute", "level": 1,
		"activations": []any{Object{"id": "rage", "name": "Rage", "modifiers": []any{
			Object{"target": "attack", "damageBonusColumn": "rage-damage", "match": []any{Object{"attackAbility": "STR"}}},
		}}}})
	put("feature", Object{"kind": "feature", "id": "monk-martial-arts", "name": "Martial Arts", "classId": "monk", "level": 1,
		"grants": Object{"attackModifiers": []any{Object{
			"ability": "DEX", "damageDieColumn": "martial-arts", "requires": Object{"noArmor": true, "noShield": true},
			"match": []any{Object{"unarmed": true}, Object{"range": "melee", "category": "simple"}},
		}}}})
	put("feat", Object{"kind": "feat", "id": "archery", "name": "Archery", "grants": Object{
		"attackModifiers": []any{Object{"attackBonus": 2, "match": []any{Object{"range": "ranged"}}}},
	}})
	put("weapon", Object{"kind": "weapon", "id": "club", "name": "Club", "category": "simple", "range": "melee",
		"damage": "1d4", "damageType": "bludgeoning", "properties": []any{"light"}})
	put("weapon", Object{"kind": "weapon", "id": "shortbow", "name": "Shortbow", "category": "simple", "range": "ranged",
		"damage": "1d6", "damageType": "piercing", "properties": []any{"ammunition", "two-handed"}})
	put("weapon", Object{"kind": "weapon", "id": "rapier", "name": "Rapier", "category": "martial", "range": "melee",
		"damage": "1d8", "damageType": "piercing", "properties": []any{"finesse"}})
	return records
}

func attackNamed(sheet Object, name string) Object {
	for _, row := range objects(sheet["weapons"]) {
		if text(row["name"]) == name {
			return row
		}
	}
	return nil
}

func TestAttackModifiersApplyMartialArtsRageAndArchery(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	profile.Constants.UnarmedStrike = &UnarmedStrike{Name: "Unarmed Strike", Damage: "1", DamageType: "bludgeoning"}
	records := attackRecords()
	equipped := func(ids ...string) []any {
		items := make([]any, 0, len(ids))
		for _, id := range ids {
			items = append(items, Object{"ref": id, "location": "equipped"})
		}
		return items
	}
	monk := hydrateView(Object{
		"baseStats": Object{"STR": 10, "DEX": 16},
		"classes":   []any{Object{"classId": "monk", "level": 5}},
		"inventory": equipped("club"),
	}, records, &profile).Sheet
	// Martial Arts swaps in DEX and the larger 1d8 die for unarmed strikes and simple melee weapons.
	if unarmed := attackNamed(monk, "Unarmed Strike"); unarmed["damage"] != "1d8 +3" || integer(unarmed["attackBonus"], 0) != 6 || !truth(unarmed["unarmed"]) {
		t.Fatalf("monk unarmed = %+v", unarmed)
	}
	if club := attackNamed(monk, "Club"); club["damage"] != "1d8 +3" || club["ability"] != "DEX" {
		t.Fatalf("monk club = %+v", club)
	}
	armored := hydrateView(Object{
		"baseStats": Object{"STR": 10, "DEX": 16},
		"classes":   []any{Object{"classId": "monk", "level": 5}},
		"inventory": equipped("leather"),
	}, records, &profile).Sheet
	if unarmed := attackNamed(armored, "Unarmed Strike"); unarmed["damage"] != "1" || unarmed["ability"] != "STR" {
		t.Fatalf("armored monk unarmed = %+v", unarmed)
	}

	rage := func(active bool) Object {
		return hydrateView(Object{
			"baseStats":      Object{"STR": 16, "DEX": 14},
			"classes":        []any{Object{"classId": "brute", "level": 9}},
			"inventory":      equipped("rapier", "shortbow"),
			"feats":          []any{Object{"featId": "archery"}},
			"activeFeatures": Object{"feature:brute-rage:rage": active},
		}, records, &profile).Sheet
	}
	raging := rage(true)
	if rapier := attackNamed(raging, "Rapier"); rapier["damage"] != "1d8 +6" || rapier["ability"] != "STR" || len(objects(rapier["modifiers"])) != 1 {
		t.Fatalf("raging rapier = %+v", rapier)
	}
	// Flat unarmed damage shows its total: 1 + STR 3 + Rage 3.
	if unarmed := attackNamed(raging, "Unarmed Strike"); unarmed["damage"] != "7" {
		t.Fatalf("raging unarmed = %+v", unarmed)
	}
	// Rage adds damage only to Strength attacks; Archery adds +2 to ranged attack rolls.
	if bow := attackNamed(raging, "Shortbow"); bow["damage"] != "1d6 +2" || integer(bow["attackBonus"], 0) != 8 {
		t.Fatalf("raging bow = %+v", bow)
	}
	if rapier := attackNamed(rage(false), "Rapier"); rapier["damage"] != "1d8 +3" || rapier["modifiers"] != nil {
		t.Fatalf("calm rapier = %+v", rapier)
	}
}

func TestPactWeaponActivationGrantsProficiencyAndCharisma(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	profile.Constants.UnarmedStrike = &UnarmedStrike{Name: "Unarmed Strike", Damage: "1", DamageType: "bludgeoning"}
	records := attackRecords()
	put := func(kind string, record Object) { records.byKind[kind][text(record["id"])] = mustJSON(record) }
	put("class", Object{"kind": "class", "id": "hexer", "name": "Hexer", "hitDie": "d8",
		"startingProficiencies": Object{"weapons": []any{"simple"}}})
	put("feature", Object{"kind": "feature", "id": "hexer-pact", "name": "Pact", "classId": "hexer", "level": 1,
		"activations": []any{Object{"id": "pact-weapon", "name": "Pact Weapon", "modifiers": []any{Object{
			"target": "weaponAbility", "ability": "CHA", "proficient": true,
			"match": []any{Object{"range": "melee", "unarmed": false}},
		}}}}})
	sheet := func(active bool) Object {
		return hydrateView(Object{
			"baseStats":      Object{"STR": 12, "DEX": 10, "CHA": 18},
			"classes":        []any{Object{"classId": "hexer", "level": 1}},
			"inventory":      []any{Object{"ref": "rapier", "location": "equipped"}, Object{"ref": "shortbow", "location": "equipped"}},
			"activeFeatures": Object{"feature:hexer-pact:pact-weapon": active},
		}, records, &profile).Sheet
	}
	// The pact weapon is proficient and uses Charisma: +2 proficiency +4 CHA.
	pact := sheet(true)
	if rapier := attackNamed(pact, "Rapier"); rapier["ability"] != "CHA" || !truth(rapier["proficient"]) || integer(rapier["attackBonus"], 0) != 6 {
		t.Fatalf("pact rapier = %+v", rapier)
	}
	// The pact covers melee weapons only, never an unarmed strike or a bow.
	if unarmed := attackNamed(pact, "Unarmed Strike"); unarmed["ability"] != "STR" {
		t.Fatalf("pact unarmed = %+v", unarmed)
	}
	if bow := attackNamed(pact, "Shortbow"); bow["ability"] != "DEX" {
		t.Fatalf("pact bow = %+v", bow)
	}
	if rapier := attackNamed(sheet(false), "Rapier"); rapier["ability"] == "CHA" || truth(rapier["proficient"]) {
		t.Fatalf("rapier without the pact = %+v", rapier)
	}
}

func TestAttackRowsGiveReachAndRange(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	profile.Constants.UnarmedStrike = &UnarmedStrike{Name: "Unarmed Strike", Damage: "1", DamageType: "bludgeoning"}
	records := attackRecords()
	put := func(record Object) { records.byKind["weapon"][text(record["id"])] = mustJSON(record) }
	put(Object{"kind": "weapon", "id": "glaive", "name": "Glaive", "category": "martial", "range": "melee",
		"damage": "1d10", "damageType": "slashing", "properties": []any{"heavy", "reach", "two-handed"}})
	put(Object{"kind": "weapon", "id": "javelin", "name": "Javelin", "category": "simple", "range": "melee",
		"damage": "1d6", "damageType": "piercing", "properties": []any{"thrown"}, "thrownRange": Object{"normal": 30, "long": 120}})
	put(Object{"kind": "weapon", "id": "dart", "name": "Dart", "category": "simple", "range": "ranged",
		"damage": "1d4", "damageType": "piercing", "properties": []any{"finesse", "thrown"}, "thrownRange": Object{"normal": 20, "long": 60}})
	put(Object{"kind": "weapon", "id": "longbow", "name": "Longbow", "category": "martial", "range": "ranged",
		"damage": "1d8", "damageType": "piercing", "properties": []any{"ammunition", "heavy", "two-handed"}, "rangeNormalLong": Object{"normal": 150, "long": 600}})
	decisions := Object{
		"classes": []any{Object{"classId": "brute", "level": 1}},
		"inventory": []any{Object{"ref": "club", "location": "equipped"}, Object{"ref": "glaive", "location": "equipped"},
			Object{"ref": "javelin", "location": "equipped"}, Object{"ref": "dart", "location": "equipped"}, Object{"ref": "longbow", "location": "equipped"}},
	}
	// Without the ruleset's melee reach, rows still carry the record's ranges.
	plain := hydrateView(decisions, records, &profile).Sheet
	if club := attackNamed(plain, "Club"); club["reach"] != nil || club["range"] != nil {
		t.Fatalf("club without reach rules = %+v", club)
	}
	if bow := attackNamed(plain, "Longbow"); !reflect.DeepEqual(bow["range"], Object{"normal": 150, "long": 600}) {
		t.Fatalf("longbow range = %+v", bow["range"])
	}

	profile.Constants.MeleeReach = &MeleeReach{Base: 5, ReachBonus: 5}
	sheet := hydrateView(decisions, records, &profile).Sheet
	for name, want := range map[string]Object{
		"Unarmed Strike": {"reach": 5},
		"Club":           {"reach": 5},
		"Glaive":         {"reach": 10},
		"Javelin":        {"reach": 5, "range": Object{"normal": 30, "long": 120, "thrown": true}},
		"Dart":           {"range": Object{"normal": 20, "long": 60, "thrown": true}},
		"Longbow":        {"range": Object{"normal": 150, "long": 600}},
	} {
		row := attackNamed(sheet, name)
		if !reflect.DeepEqual(row["reach"], want["reach"]) || !reflect.DeepEqual(row["range"], want["range"]) {
			t.Errorf("%s reach/range = %v / %v, want %v", name, row["reach"], row["range"], want)
		}
	}
}
