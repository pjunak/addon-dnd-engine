package rules

import "testing"

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
	monk := Hydrate(Object{
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
	armored := Hydrate(Object{
		"baseStats": Object{"STR": 10, "DEX": 16},
		"classes":   []any{Object{"classId": "monk", "level": 5}},
		"inventory": equipped("leather"),
	}, records, &profile).Sheet
	if unarmed := attackNamed(armored, "Unarmed Strike"); unarmed["damage"] != "1" || unarmed["ability"] != "STR" {
		t.Fatalf("armored monk unarmed = %+v", unarmed)
	}

	rage := func(active bool) Object {
		return Hydrate(Object{
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
