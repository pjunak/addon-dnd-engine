package rules

import "math"

// Class tables print per-level values such as Rage Damage or Martial Arts.
// A class record declares them as tableColumns, each with ascending
// {level, value} steps; the last step at or below the class level applies.
func classColumnValue(record Object, key string, level int) (any, bool) {
	for _, column := range objects(record["tableColumns"]) {
		if text(column["key"]) != key {
			continue
		}
		var value any
		found := false
		for _, step := range objects(column["progression"]) {
			if integer(step["level"], 1) <= level {
				value, found = step["value"], true
			}
		}
		return value, found
	}
	return nil, false
}

func hydrateClassValues(sheet Object, classes []resolvedClass) {
	rows := make([]any, 0)
	for _, current := range classes {
		for _, column := range objects(current.Record["tableColumns"]) {
			key := text(column["key"])
			if value, found := classColumnValue(current.Record, key, current.Level); found {
				rows = append(rows, Object{"classId": current.ID, "key": key,
					"name": firstText(column["name"], key), "value": value})
			}
		}
	}
	sheet["classValues"] = rows
}

// The source's own class supplies a column reference, so a multiclass
// character reads each feature at the level of the class that grants it.
func sourceClassColumn(source grantSource, classes []resolvedClass, key string) (int, bool) {
	classID := text(source.Record["classId"])
	if text(source.Source["type"]) == "class" {
		classID = text(source.Source["id"])
	}
	for _, current := range classes {
		if current.ID == classID {
			value, found := classColumnValue(current.Record, key, current.Level)
			return integer(value, 0), found
		}
	}
	return 0, false
}

func weaponMasteryCount(record Object, level int) int {
	mastery := object(record["weaponMastery"])
	count := integer(mastery["count"], 0)
	for _, step := range objects(mastery["progression"]) {
		if integer(step["level"], 1) <= level {
			count = integer(step["count"], count)
		}
	}
	return count
}

type armorState struct {
	bodyType string // "", "light", "medium" or "heavy"
	shield   bool
}

func equippedArmor(decisions Object, records Records) armorState {
	state := armorState{}
	for _, item := range objects(decisions["inventory"]) {
		if text(item["location"]) != "equipped" {
			continue
		}
		switch kind := text(inventoryRecord(item, records, "armor")["armorType"]); kind {
		case "shield":
			state.shield = true
		case "light", "medium", "heavy":
			if state.bodyType == "" {
				state.bodyType = kind
			}
		}
	}
	return state
}

// armorRequirementMet interprets noArmor, noShield and notArmorTypes. Any other
// requirement is unsupported and keeps the bonus inactive.
func armorRequirementMet(raw any, armor armorState) bool {
	if raw == nil {
		return true
	}
	met := true
	for key, value := range object(raw) {
		switch key {
		case "noArmor":
			met = met && (!truth(value) || armor.bodyType == "")
		case "noShield":
			met = met && (!truth(value) || !armor.shield)
		case "notArmorTypes":
			met = met && len(stringsOf(value)) > 0 && !contains(stringsOf(value), armor.bodyType)
		default:
			return false
		}
	}
	return met && object(raw) != nil
}

// Conditional speed bonuses, such as Fast Movement or Unarmored Movement,
// apply after worn-armor penalties. Each row keeps its status for explanations.
func applySpeedBonuses(sheet Object, sources []grantSource, classes []resolvedClass, armor armorState) {
	rows, total := make([]any, 0), 0
	for _, source := range sources {
		for _, bonus := range objects(source.Grants["speedBonuses"]) {
			value, valid := integer(bonus["add"], 0), bonus["add"] != nil
			if column := text(bonus["column"]); column != "" {
				value, valid = sourceClassColumn(source, classes, column)
			}
			status := "inactive"
			if valid && armorRequirementMet(bonus["requires"], armor) {
				total += value
				status = "applied"
			}
			rows = append(rows, Object{"name": firstText(source.Record["name"], source.Source["id"]),
				"source": source.Source, "value": value, "status": status, "requires": bonus["requires"]})
		}
	}
	sheet["speedBonuses"] = rows
	sheet["speed"] = integer(sheet["speed"], 0) + total
	object(sheet["derived"])["speed"] = sheet["speed"]
}

// checkBonus resolves one skill or saving throw bonus: a flat add, half the
// proficiency bonus (rounded down), or an ability modifier, with an optional
// minimum. Unknown fields keep the bonus inactive.
func checkBonus(bonus Object, mods map[string]int, pb int) (int, bool) {
	value := 0
	for key, raw := range bonus {
		switch key {
		case "add":
			if text(raw) == "halfProficiency" {
				value += pb / 2
			} else if amount := number(raw, math.NaN()); !math.IsNaN(amount) && text(raw) == "" {
				value += int(amount)
			} else {
				return 0, false
			}
		case "addAbility":
			modifier, known := mods[text(raw)]
			if !known {
				return 0, false
			}
			value += modifier
		case "skills", "abilities", "notProficient", "min":
		default:
			return 0, false
		}
	}
	if bonus["min"] != nil {
		value = max(value, integer(bonus["min"], value))
	}
	return value, true
}

func grantCheckBonuses(sources []grantSource, field, id string, proficient bool, mods map[string]int, pb int) (int, []any) {
	rows, total := make([]any, 0), 0
	for _, source := range sources {
		for _, bonus := range objects(source.Grants[field]) {
			scope := stringsOf(bonus["skills"])
			if field == "saveBonuses" {
				scope = stringsOf(bonus["abilities"])
			}
			if len(scope) > 0 && !contains(scope, id) || truth(bonus["notProficient"]) && proficient {
				continue
			}
			value, valid := checkBonus(bonus, mods, pb)
			if !valid {
				continue
			}
			total += value
			rows = append(rows, Object{"name": firstText(source.Record["name"], source.Source["id"]),
				"source": source.Source, "value": value})
		}
	}
	return total, rows
}

// withBonuses lists applied bonuses only where a grant contributed one.
func withBonuses(row Object, bonuses []any) Object {
	if len(bonuses) > 0 {
		row["bonuses"] = bonuses
	}
	return row
}
