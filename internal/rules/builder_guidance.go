package rules

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// BuilderGuidance is a read-only companion to the stable Builder plan. It keeps
// completion and option policy out of UI consumers without changing decisions.
func BuilderGuidance(decisions, plan Object, records Records, profile Ruleset) Object {
	normalized := NormalizeBuilderDecisions(decisions, records, profile)
	hydrated := Hydrate(normalized, records, &profile)
	choices, classes := Object{}, []any{}
	sections := []Object{{"id": "foundation", "total": 0, "complete": 0, "issues": []any{}}, {"id": "progression", "total": 0, "complete": 0, "issues": []any{}}, {"id": "spells", "total": 0, "complete": 0, "issues": []any{}}}
	add := func(group int, done bool, id, label, tab string, level int) {
		section := sections[group]
		section["total"] = integer(section["total"], 0) + 1
		if done {
			section["complete"] = integer(section["complete"], 0) + 1
			return
		}
		section["issues"] = append(values(section["issues"]), Object{"id": id, "label": label, "tab": tab, "level": level})
	}
	base := object(plan["baseStats"])
	scoresValid, spent := true, 0
	for _, ability := range Abilities {
		score := integer(base[ability], 0)
		if truth(decisions["manualScores"]) {
			scoresValid = scoresValid && score >= 1 && score <= profile.Constants.AbilityCapHard
		} else {
			cost, exists := profile.Constants.PointBuy.Cost[jsonNumber(score)]
			spent += cost
			scoresValid = scoresValid && exists
		}
	}
	add(0, scoresValid && (truth(decisions["manualScores"]) || spent == profile.Constants.PointBuy.Budget), "abilities", "Finish base ability scores", "character", 0)
	species := selectedRecord(firstText(decisions["species"], decisions["race"]), records, "species")
	add(0, species != nil, "species", "Choose species", "character", 0)
	add(0, selectedRecord(text(decisions["background"]), records, "background") != nil, "background", "Choose background", "character", 0)
	if lineages := objects(species["lineages"]); len(lineages) > 0 {
		valid := false
		for _, lineage := range lineages {
			valid = valid || text(lineage["id"]) == text(decisions["lineage"])
		}
		add(0, valid, "lineage", "Choose lineage", "character", 0)
	}
	for groupIndex, group := range []any{plan["creationChoices"], plan["creationAbilityChoices"], plan["classChoices"]} {
		for _, choice := range objects(group) {
			entry := builderChoiceGuidance(decisions, choice, hydrated.Sheet, records)
			choices[text(choice["id"])] = entry
			section, tab := 0, "character"
			if groupIndex == 2 {
				section, tab = 1, text(choice["classId"])
			}
			add(section, truth(entry["done"]), text(choice["id"]), text(entry["label"]), tab, integer(object(choice["source"])["level"], 1))
		}
	}
	for index, selected := range objects(plan["classes"]) {
		id := text(selected["classId"])
		class := recordByID(records, "class", id)
		add(1, class != nil, fmt.Sprintf("class-%d", index), "Choose class", "character", 0)
		if class == nil {
			continue
		}
		subclasses := []any{}
		for _, sub := range recordList(records, "subclass") {
			if text(sub["classId"]) == id {
				subclasses = append(subclasses, guidanceOption(text(sub["id"]), sub))
			}
		}
		sortGuidanceOptions(subclasses)
		level, subclassLevel := max(1, integer(selected["level"], 1)), max(1, integer(class["subclassLevel"], 3))
		if level >= subclassLevel && len(subclasses) > 0 {
			valid := false
			for _, sub := range objects(subclasses) {
				valid = valid || text(sub["id"]) == text(selected["subclass"])
			}
			add(1, valid, "subclass:"+id, "Choose "+firstText(class["name"], id)+" subclass", id, subclassLevel)
		}
		levels := []any{}
		for at := 1; at <= min(level, 20); at++ {
			features := []any{}
			for _, feature := range objects(hydrated.Sheet["features"]) {
				source := object(feature["source"])
				if integer(source["level"], 0) == at && (text(source["id"]) == id || text(source["id"]) == text(selected["subclass"])) {
					record := recordByID(records, "feature", text(feature["id"]))
					if record == nil {
						record = Object{"name": firstText(feature["name"], feature["id"])}
					}
					features = append(features, guidanceOption(text(feature["id"]), record))
				}
			}
			sortGuidanceOptions(features)
			levels = append(levels, Object{"level": at, "features": features})
		}
		classes = append(classes, Object{"classId": id, "name": firstText(class["name"], id), "level": level, "subclassLevel": subclassLevel, "subclasses": subclasses, "levels": levels})
	}
	casting := object(hydrated.Sheet["spellcasting"])
	for _, choice := range objects(casting["pendingChoices"]) {
		add(2, len(unique(stringsOf(choice["picked"]))) >= max(1, integer(choice["choose"], 1)), text(choice["key"]), "Choose spells: "+guidanceLabel(text(object(choice["source"])["id"])), "spells", 0)
	}
	for _, choice := range objects(casting["castingAbilityChoices"]) {
		add(2, text(choice["selected"]) != "", text(choice["key"]), "Choose casting ability: "+guidanceLabel(text(object(choice["source"])["id"])), "spells", 0)
	}
	total, complete, sectionValues := 0, 0, []any{}
	for _, section := range sections {
		total += integer(section["total"], 0)
		complete += integer(section["complete"], 0)
		sectionValues = append(sectionValues, section)
	}
	return Object{"choices": choices, "classes": classes, "sections": sectionValues, "total": total, "complete": complete, "ready": total == complete, "derived": hydrated.Sheet["derived"], "warnings": anyStrings(hydrated.Warnings)}
}

func builderChoiceGuidance(decisions, choice, sheet Object, records Records) Object {
	kind, id := text(choice["kind"]), text(choice["id"])
	label := firstText(choice["prompt"], guidanceLabel(id))
	if kind == "asiMode" {
		label = fmt.Sprintf("%s level %d advancement", guidanceLabel(text(choice["classId"])), integer(choice["level"], 1))
	}
	if kind == "abilityBudget" {
		label = "Assign origin ability points"
	}
	options := builderChoiceOptions(choice, sheet, records)
	entry := Object{"id": id, "label": label, "options": options, "picked": 0, "required": max(1, integer(choice["count"], 1)), "done": false}
	if kind == "abilityBudget" {
		picked, required, valid := guidanceAbility(decisions, choice)
		entry["picked"], entry["required"], entry["done"] = picked, required, valid
		return entry
	}
	if kind == "asiMode" {
		mode := text(object(decisions["featureChoices"])[id])
		if mode == "asi" {
			picked, required, valid := guidanceAbility(decisions, object(choice["ability"]))
			entry["picked"], entry["required"], entry["done"] = picked, required, valid
		}
		feat := cloneObjectDeep(object(choice["feat"]))
		feat["kind"] = "feat"
		entry["featOptions"] = builderChoiceOptions(feat, sheet, records)
		if mode == "feat" {
			selected := text(object(decisions["featureChoices"])[text(feat["id"])])
			valid := false
			for _, option := range objects(entry["featOptions"]) {
				valid = valid || text(option["id"]) == selected
			}
			if valid {
				entry["picked"], entry["done"] = 1, true
				if descriptor := object(feat["ability"]); len(stringsOf(descriptor["eligible"])) > 0 {
					picked, required, done := guidanceAbility(decisions, descriptor)
					entry["picked"], entry["required"], entry["done"] = picked, required, done
				}
			}
		}
		return entry
	}
	valid := map[string]bool{}
	for _, option := range objects(options) {
		valid[text(option["id"])] = true
	}
	picked := 0
	for _, value := range choiceValues(decisions, choice) {
		if valid[value] {
			picked++
		}
	}
	entry["picked"], entry["done"] = picked, picked >= integer(entry["required"], 1)
	return entry
}

func guidanceAbility(decisions, descriptor Object) (int, int, bool) {
	budget, total, valid := max(1, integer(descriptor["budget"], 1)), 0, true
	for _, grant := range objects(decisions["abilityGrants"]) {
		if text(grant["id"]) == text(descriptor["id"]) {
			for ability, amount := range object(grant["assign"]) {
				value := integer(amount, 0)
				total += max(0, value)
				valid = valid && value >= 0 && value <= integer(descriptor["perAbilityMax"], budget) && (descriptor["eligible"] == nil || contains(stringsOf(descriptor["eligible"]), ability))
			}
		}
	}
	return total, budget, valid && total == budget
}

func builderChoiceOptions(choice, sheet Object, records Records) []any {
	kind, pool := text(choice["kind"]), stringsOf(choice["from"])
	if pool == nil {
		switch kind {
		case "skills", "expertise", "skillExpertise":
			for skill := range SkillAbility {
				pool = append(pool, skill)
			}
		case "weaponMastery":
			for _, weapon := range recordList(records, "weapon") {
				pool = append(pool, text(weapon["id"]))
			}
		case "feat":
			for _, feat := range recordList(records, "feat") {
				pool = append(pool, text(feat["id"]))
			}
		}
	}
	result := []any{}
	for _, id := range unique(pool) {
		var record Object
		switch kind {
		case "feat":
			record = recordByID(records, "feat", id)
			if record == nil || text(choice["category"]) != "" && text(choice["category"]) != text(record["category"]) || len(stringsOf(choice["categories"])) > 0 && !contains(stringsOf(choice["categories"]), text(record["category"])) {
				continue
			}
		case "weaponMastery":
			record = recordByID(records, "weapon", id)
			if record == nil {
				continue
			}
		case "tools":
			record = recordByID(records, "tool", id)
		case "expertise":
			if !truth(object(object(sheet["skills"])[canonicalSkill(id)])["proficient"]) {
				continue
			}
		case "proficiencies":
			if strings.HasPrefix(id, "tool:") {
				record = recordByID(records, "tool", strings.TrimPrefix(id, "tool:"))
			}
		case "enumerated":
			record = recordByID(records, "feature", id)
		}
		result = append(result, guidanceOption(id, record))
	}
	sortGuidanceOptions(result)
	return result
}

func guidanceOption(id string, record Object) Object {
	result := Object{"id": id, "label": firstText(record["name"], guidanceLabel(id))}
	if description := text(record["text"]); description != "" {
		runes := []rune(description)
		result["description"] = string(runes[:min(1200, len(runes))])
	}
	return result
}

func sortGuidanceOptions(options []any) {
	sort.Slice(options, func(i, j int) bool {
		a, b := object(options[i]), object(options[j])
		if text(a["label"]) == text(b["label"]) {
			return text(a["id"]) < text(b["id"])
		}
		return text(a["label"]) < text(b["label"])
	})
}
func guidanceLabel(value string) string {
	var result strings.Builder
	for index, char := range value {
		if unicode.IsUpper(char) && index > 0 {
			result.WriteByte(' ')
		}
		if char == '-' || char == '_' || char == ':' {
			char = ' '
		}
		result.WriteRune(char)
	}
	parts := strings.Fields(result.String())
	for index, part := range parts {
		chars := []rune(part)
		chars[0] = unicode.ToUpper(chars[0])
		parts[index] = string(chars)
	}
	return strings.Join(parts, " ")
}
