package rules

import (
	"encoding/json"
	"testing"
)

func classMechanicsRecords() memoryRecords {
	records := syntheticRecords()
	records.byKind["class"]["monk"] = mustJSON(Object{"kind": "class", "id": "monk", "name": "Monk", "hitDie": "d8",
		"weaponMastery": Object{"count": 2, "progression": []any{Object{"level": 4, "count": 3}}},
		"tableColumns": []any{
			Object{"key": "martial-arts", "name": "Martial Arts", "progression": []any{
				Object{"level": 1, "value": "1d6"}, Object{"level": 5, "value": "1d8"}}},
			Object{"key": "unarmored-movement", "name": "Unarmored Movement", "progression": []any{
				Object{"level": 2, "value": 10}, Object{"level": 6, "value": 15}}},
		}})
	for _, feature := range []Object{
		{"kind": "feature", "id": "monk-unarmored-movement", "name": "Unarmored Movement", "classId": "monk", "level": 2,
			"grants": Object{"speedBonuses": []any{Object{"column": "unarmored-movement", "requires": Object{"noArmor": true, "noShield": true}}}}},
		{"kind": "feature", "id": "monk-jack", "name": "Jack", "classId": "monk", "level": 2,
			"grants": Object{"skillBonuses": []any{Object{"notProficient": true, "add": "halfProficiency"}}}},
		{"kind": "feature", "id": "monk-aura", "name": "Aura", "classId": "monk", "level": 6,
			"grants": Object{"saveBonuses": []any{Object{"addAbility": "CHA", "min": 1}}}},
		{"kind": "feature", "id": "monk-body", "name": "Body", "classId": "monk", "level": 20,
			"grants": Object{"abilityScoreBonus": Object{"assign": Object{"DEX": 4}, "cap": 25}}},
		{"kind": "feature", "id": "monk-odd", "name": "Odd", "classId": "monk", "level": 1,
			"grants": Object{"speedBonuses": []any{Object{"add": 5, "requires": Object{"mounted": true}}}}},
	} {
		records.byKind["feature"][text(feature["id"])] = mustJSON(feature)
	}
	if records.byKind["armor"] == nil {
		records.byKind["armor"] = map[string]json.RawMessage{}
	}
	records.byKind["armor"]["leather"] = mustJSON(Object{"kind": "armor", "id": "leather", "name": "Leather", "armorType": "light", "baseAc": 11})
	return records
}

func TestClassTableColumnsDriveConditionalSpeedAndMastery(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	records := classMechanicsRecords()
	monk := func(level int, inventory ...any) Object {
		return Hydrate(Object{"classes": []any{Object{"classId": "monk", "level": level}}, "inventory": inventory}, records, &profile).Sheet
	}
	sheet := monk(6)
	if integer(sheet["speed"], 0) != 45 {
		t.Fatalf("speed = %v, bonuses = %+v", sheet["speed"], sheet["speedBonuses"])
	}
	values := objects(sheet["classValues"])
	if len(values) != 2 || values[0]["value"] != "1d8" || integer(values[1]["value"], 0) != 15 {
		t.Fatalf("class values = %+v", values)
	}
	if integer(object(sheet["weaponMastery"])["slots"], 0) != 3 || integer(object(monk(3)["weaponMastery"])["slots"], 0) != 2 {
		t.Fatal("weapon mastery did not follow the class progression")
	}
	if armored := monk(6, Object{"ref": "leather", "location": "equipped"}); integer(armored["speed"], 0) != 30 {
		t.Fatalf("armored speed = %v", armored["speed"])
	}
	if len(objects(monk(1)["classValues"])) != 1 || integer(monk(1)["speed"], 0) != 30 {
		t.Fatal("a column or an unsupported requirement applied before its level")
	}
}

func TestGrantedCheckBonusesUseProficiencyAndAbilityRules(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	sheet := Hydrate(Object{
		"baseStats":          Object{"CHA": 8, "WIS": 14},
		"classes":            []any{Object{"classId": "monk", "level": 6}},
		"skillProficiencies": []any{"insight"},
	}, classMechanicsRecords(), &profile).Sheet
	skills := object(sheet["skills"])
	// Half of +3 rounds down, and only skills without proficiency qualify.
	if integer(object(skills["perception"])["total"], 0) != 3 || integer(object(skills["insight"])["total"], 0) != 5 {
		t.Fatalf("skills = %+v", skills)
	}
	if integer(object(sheet["passives"])["perception"], 0) != 13 {
		t.Fatalf("passive = %+v", sheet["passives"])
	}
	// A negative Charisma modifier still grants the printed minimum of +1.
	if save := object(object(sheet["saves"])["WIS"]); integer(save["total"], 0) != 3 || len(objects(save["bonuses"])) != 1 {
		t.Fatalf("save = %+v", save)
	}
}

func TestFixedFeatureAbilityBonusRaisesItsCap(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	records := classMechanicsRecords()
	decisions := Object{"baseStats": Object{"DEX": 20}, "classes": []any{Object{"classId": "monk", "level": 20}}}
	sheet := Hydrate(NormalizeBuilderDecisions(decisions, records, profile), records, &profile).Sheet
	if number(object(object(sheet["abilities"])["DEX"])["score"], 0) != 24 {
		t.Fatalf("abilities = %+v", sheet["abilities"])
	}
	decisions["classes"] = []any{Object{"classId": "monk", "level": 19}}
	sheet = Hydrate(NormalizeBuilderDecisions(decisions, records, profile), records, &profile).Sheet
	if number(object(object(sheet["abilities"])["DEX"])["score"], 0) != 20 {
		t.Fatal("the increase applied before its feature level")
	}
}
