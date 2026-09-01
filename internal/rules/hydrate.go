package rules

import (
	"fmt"
	"math"
	"regexp"
	"sort"
)

var SkillAbility = map[string]string{
	"acrobatics": "DEX", "animalHandling": "WIS", "arcana": "INT", "athletics": "STR",
	"deception": "CHA", "history": "INT", "insight": "WIS", "intimidation": "CHA",
	"investigation": "INT", "medicine": "WIS", "nature": "INT", "perception": "WIS",
	"performance": "CHA", "persuasion": "CHA", "religion": "INT", "sleightOfHand": "DEX",
	"stealth": "DEX", "survival": "WIS",
}

type HydrationResult struct {
	Sheet    Object   `json:"sheet"`
	Warnings []string `json:"warnings"`
}

type resolvedClass struct {
	ID       string
	Name     string
	Level    int
	Subclass string
	Record   Object
}

type caster struct {
	class    resolvedClass
	casting  Object
	progress Object
	pact     *PactMagicResult
}

func Hydrate(decisions Object, records Records, ruleset *Ruleset) HydrationResult {
	if decisions == nil {
		decisions = Object{}
	}
	if ruleset == nil {
		return HydrateWithoutRulesData(decisions, "missing")
	}
	warnings := make([]string, 0)
	warn := func(message string) {
		if message != "" {
			warnings = append(warnings, message)
		}
	}

	sheet := Object{
		"abilities": Object{},
		"derived":   Object{},
		"proficiencies": Object{
			"saves": Object{}, "skills": Object{}, "armor": []any{}, "weapons": []any{},
			"tools": []any{}, "languages": []any{},
		},
		"features": []any{},
	}
	mods := make(map[string]int, len(Abilities))
	hydrateAbilities(decisions, sheet, mods, *ruleset)

	classes := resolveClasses(decisions, records, warn)
	totalLevel := integer(decisions["level"], 1)
	if len(classes) > 0 {
		totalLevel = 0
		for _, current := range classes {
			totalLevel += current.Level
		}
	}
	if totalLevel < 1 {
		totalLevel = 1
	}
	pb := ProficiencyBonus(float64(totalLevel))
	classValues := make([]any, 0, len(classes))
	for _, current := range classes {
		classValues = append(classValues, Object{
			"classId": current.ID, "name": current.Name, "level": current.Level,
			"subclass": current.Subclass, "hitDie": text(current.Record["hitDie"]),
		})
	}
	sheet["classes"] = classValues
	sheet["totalLevel"] = totalLevel
	derived := object(sheet["derived"])
	derived["proficiencyBonus"] = pb
	if len(classes) > 0 && classes[0].Record != nil {
		sheet["class"] = classes[0].Record
		derived["hitDie"] = nullableText(classes[0].Record["hitDie"])
	} else {
		derived["hitDie"] = nil
	}

	species, lineage, hpPerLevel := hydrateSpecies(decisions, records, sheet, warn)
	background := selectedRecord(decisions["background"], records, "background")
	if decisions["background"] != nil && background == nil && records != nil {
		warn("Unknown background: " + text(decisions["background"]))
	}
	featRecords := selectedFeats(decisions, records)
	hydrateHitPoints(sheet, classes, mods["CON"], hpPerLevel, featRecords)
	hydrateArmorClass(decisions, sheet, classes, mods, records, species)
	derived["initiative"] = initiative(decisions, records, mods["DEX"], pb)
	hydrateSaves(decisions, sheet, classes, mods, pb)
	hydrateSkills(decisions, sheet, background, mods, pb)
	hydrateProficiencies(decisions, sheet, classes, background, records)
	hydrateSpellcasting(decisions, sheet, classes, featRecords, records, *ruleset, mods, pb)
	hydrateWeaponMastery(decisions, sheet, classes, featRecords, *ruleset)
	hydrateWeaponsAndAttunement(decisions, sheet, classes, records, mods, pb, *ruleset, warn)
	hydrateCoreResources(sheet, classes, *ruleset)

	_ = lineage
	return HydrationResult{Sheet: sheet, Warnings: warnings}
}

func HydrateWithoutRulesData(decisions Object, status string) HydrationResult {
	base := object(decisions["baseStats"])
	if len(base) == 0 {
		base = object(decisions["abilities"])
	}
	abilities := Object{}
	for _, ability := range Abilities {
		score := number(base[ability], 10)
		abilities[ability] = Object{
			"base": score, "score": score, "mod": AbilityModifier(score), "bonus": 0,
		}
	}
	totalLevel := integer(decisions["level"], 1)
	if classes := objects(decisions["classes"]); len(classes) > 0 {
		totalLevel = 0
		for _, current := range classes {
			totalLevel += max(0, integer(current["level"], 0))
		}
	}
	if totalLevel < 1 {
		totalLevel = 1
	}
	return HydrationResult{
		Sheet: Object{
			"abilities": abilities, "totalLevel": totalLevel,
			"derived": Object{
				"proficiencyBonus": ProficiencyBonus(float64(totalLevel)),
				"initiative":       object(abilities["DEX"])["mod"],
			},
			"proficiencies": Object{
				"saves": Object{}, "skills": Object{}, "armor": []any{}, "weapons": []any{},
				"tools": []any{}, "languages": []any{},
			},
			"features": []any{},
		},
		Warnings: []string{fmt.Sprintf(
			"Rules data unavailable (%s); edition-dependent computation was skipped.", status,
		)},
	}
}

func hydrateAbilities(decisions Object, sheet Object, mods map[string]int, ruleset Ruleset) {
	base := object(decisions["baseStats"])
	if len(base) == 0 {
		base = object(decisions["abilities"])
	}
	grants := objects(decisions["abilityGrants"])
	result := object(sheet["abilities"])
	for _, ability := range Abilities {
		bonus := 0
		capValue := ruleset.Constants.AbilityCap
		for _, grant := range grants {
			assign := object(grant["assign"])
			amount := integer(assign[ability], 0)
			if amount == 0 {
				continue
			}
			bonus += amount
			if raised := integer(grant["cap"], 0); raised > capValue {
				capValue = min(ruleset.Constants.AbilityCapHard, raised)
			}
		}
		baseScore := number(base[ability], 10)
		score := math.Min(float64(capValue), baseScore+float64(bonus))
		modifier := AbilityModifier(score)
		mods[ability] = modifier
		result[ability] = Object{
			"base": baseScore, "score": score, "mod": modifier, "bonus": bonus,
		}
	}
}

func resolveClasses(decisions Object, records Records, warn func(string)) []resolvedClass {
	result := make([]resolvedClass, 0)
	for _, current := range objects(decisions["classes"]) {
		id := text(current["classId"])
		if id == "" {
			continue
		}
		record := recordByID(records, "class", id)
		if record == nil {
			record = recordByName(records, "class", id)
		}
		if record == nil && records != nil {
			warn("Unknown class: " + id)
		}
		name := id
		if record != nil {
			name = text(record["name"])
		}
		result = append(result, resolvedClass{
			ID: id, Name: name, Level: max(1, integer(current["level"], 1)),
			Subclass: text(current["subclass"]), Record: record,
		})
	}
	if len(result) == 0 && text(decisions["className"]) != "" {
		name := text(decisions["className"])
		record := recordByName(records, "class", name)
		if record == nil {
			record = recordByID(records, "class", name)
		}
		if record == nil && records != nil {
			warn("Unknown class: " + name)
		}
		id := name
		if record != nil {
			id = text(record["id"])
			name = text(record["name"])
		}
		result = append(result, resolvedClass{
			ID: id, Name: name, Level: max(1, integer(decisions["level"], 1)),
			Subclass: text(decisions["subclass"]), Record: record,
		})
	}
	return result
}

func hydrateSpecies(
	decisions Object,
	records Records,
	sheet Object,
	warn func(string),
) (Object, Object, int) {
	selected := text(decisions["race"])
	if selected == "" {
		selected = text(decisions["species"])
	}
	species := selectedRecord(selected, records, "species")
	if selected != "" && species == nil && records != nil {
		warn("Unknown species: " + selected)
	}
	var lineage Object
	if species != nil && text(decisions["lineage"]) != "" {
		for _, candidate := range objects(species["lineages"]) {
			if text(candidate["id"]) == text(decisions["lineage"]) {
				lineage = candidate
				break
			}
		}
	}
	darkvision := 0
	speedBonus := 0
	hpPerLevel := 0
	resistances := make([]string, 0)
	if species != nil {
		sheet["species"] = species
		darkvision = integer(object(species["senses"])["darkvision"], 0)
		resistances = append(resistances, stringsOf(species["resistances"])...)
		hpPerLevel += integer(object(species["grants"])["hpPerLevel"], 0)
	}
	if lineage != nil {
		grants := object(lineage["grants"])
		darkvision = max(darkvision, integer(object(grants["senses"])["darkvision"], 0))
		resistances = append(resistances, stringsOf(grants["resistances"])...)
		hpPerLevel += integer(grants["hpPerLevel"], 0)
		speedBonus += integer(grants["speedBonus"], 0)
	}
	speed := 30
	if species != nil {
		speed = integer(object(species["speeds"])["walk"], 30)
	}
	speed += speedBonus
	sheet["speed"] = speed
	object(sheet["derived"])["speed"] = speed
	if darkvision > 0 {
		sheet["senses"] = Object{"darkvision": darkvision}
	} else {
		sheet["senses"] = Object{}
	}
	sheet["resistances"] = anyStrings(unique(resistances))
	return species, lineage, hpPerLevel
}

func hydrateHitPoints(sheet Object, classes []resolvedClass, conMod, speciesHP int, feats []Object) {
	featHP := 0
	for _, feat := range feats {
		featHP += integer(object(feat["grants"])["hpPerLevel"], 0)
	}
	perLevel := speciesHP + featHP
	dice := 0
	level := 0
	maxAwarded := false
	for _, current := range classes {
		die := hitDieSize(text(current.Record["hitDie"]))
		for index := 0; index < current.Level; index++ {
			level++
			if !maxAwarded {
				dice += die
				maxAwarded = true
			} else {
				dice += die/2 + 1
			}
		}
	}
	maximum := 0
	if level > 0 {
		maximum = dice + conMod*level + perLevel*level
	}
	sheet["hp"] = Object{
		"max": maximum,
		"breakdown": Object{
			"dice": dice, "conMod": conMod, "conTotal": conMod * level,
			"miscPerLevel": perLevel, "miscTotal": perLevel * level, "level": level,
		},
	}
	object(sheet["derived"])["maxHp"] = maximum
}

func hydrateArmorClass(
	decisions Object,
	sheet Object,
	classes []resolvedClass,
	mods map[string]int,
	records Records,
	species Object,
) {
	var bodyArmor Object
	var shield Object
	for _, item := range objects(decisions["inventory"]) {
		if text(item["location"]) != "equipped" {
			continue
		}
		armor := inventoryRecord(item, records, "armor")
		switch text(armor["armorType"]) {
		case "shield":
			if shield == nil {
				shield = armor
			}
		case "light", "medium", "heavy":
			if bodyArmor == nil {
				bodyArmor = armor
			}
		}
	}
	candidates := make([]any, 0)
	if bodyArmor != nil {
		dexPart := mods["DEX"]
		if _, hasCap := bodyArmor["dexCap"]; hasCap && bodyArmor["dexCap"] != nil {
			capValue := integer(bodyArmor["dexCap"], 0)
			if capValue == 0 {
				dexPart = 0
			} else {
				dexPart = min(dexPart, capValue)
			}
		}
		candidates = append(candidates, Object{
			"id": "armor:" + text(bodyArmor["id"]), "label": text(bodyArmor["name"]),
			"value": integer(bodyArmor["baseAC"], 10) + dexPart,
		})
	} else {
		for _, current := range classes {
			for _, formula := range objects(current.Record["acFormulas"]) {
				if truth(object(formula["requires"])["noShield"]) && shield != nil {
					continue
				}
				addition := 0
				for _, ability := range stringsOf(formula["addAbilities"]) {
					addition += mods[ability]
				}
				candidates = append(candidates, Object{
					"id": text(formula["id"]), "label": firstText(formula["name"], formula["id"]),
					"value": integer(formula["base"], 10) + addition,
				})
			}
		}
	}
	candidates = append(candidates, Object{"id": "unarmored", "label": "Unarmored", "value": 10 + mods["DEX"]})
	best := object(candidates[0])
	for _, candidate := range candidates[1:] {
		if integer(object(candidate)["value"], 0) > integer(best["value"], 0) {
			best = object(candidate)
		}
	}
	shieldBonus := 0
	if shield != nil {
		shieldBonus = integer(shield["acBonus"], 2)
	}
	value := integer(best["value"], 0) + shieldBonus + integer(object(species["grants"])["acBonus"], 0)
	ac := Object{
		"value": value, "base": text(best["label"]), "shield": shieldBonus,
		"candidates": candidates, "restrictions": nil,
	}
	if bodyArmor != nil {
		requirement := strengthRequirement(bodyArmor["strReq"])
		strength := integer(object(object(sheet["abilities"])["STR"])["score"], 10)
		restrictions := Object{
			"armorId": text(bodyArmor["id"]), "strengthRequirement": requirement,
			"meetsStrength":       requirement == 0 || strength >= requirement,
			"speedPenalty":        conditionalInt(requirement > 0 && strength < requirement, 10),
			"stealthDisadvantage": truth(bodyArmor["stealthDisadvantage"]),
		}
		ac["restrictions"] = restrictions
		sheet["armorRestrictions"] = restrictions
		if integer(restrictions["speedPenalty"], 0) > 0 {
			sheet["speed"] = max(0, integer(sheet["speed"], 0)-integer(restrictions["speedPenalty"], 0))
			object(sheet["derived"])["speed"] = sheet["speed"]
		}
	} else {
		sheet["armorRestrictions"] = nil
	}
	sheet["ac"] = ac
	object(sheet["derived"])["armorClass"] = value
}

func hydrateSaves(decisions, sheet Object, classes []resolvedClass, mods map[string]int, pb int) {
	first := []string{}
	if len(classes) > 0 {
		first = stringsOf(classes[0].Record["savingThrows"])
	}
	manual := object(decisions["saveProf"])
	granted := stringsOf(decisions["saveProficiencies"])
	proficiencies := object(object(sheet["proficiencies"])["saves"])
	saves := Object{}
	for _, ability := range Abilities {
		proficient := contains(first, ability) || contains(granted, ability) || truth(manual[ability])
		proficiencies[ability] = proficient
		total := mods[ability]
		if proficient {
			total += pb
		}
		saves[ability] = Object{"mod": mods[ability], "proficient": proficient, "total": total}
	}
	sheet["saves"] = saves
}

func hydrateSkills(decisions, sheet Object, background Object, mods map[string]int, pb int) {
	manual := object(decisions["skillProf"])
	resolved := stringsOf(decisions["skillProficiencies"])
	if decisions["skillProficiencies"] == nil {
		for skill, value := range manual {
			if truth(value) {
				resolved = append(resolved, skill)
			}
		}
	}
	resolved = append(resolved, stringsOf(background["skillProficiencies"])...)
	resolved = append(resolved, stringsOf(decisions["speciesSkillProficiencies"])...)
	expertise := object(decisions["skillExpertise"])
	proficiencies := object(object(sheet["proficiencies"])["skills"])
	skills := Object{}
	ids := make([]string, 0, len(SkillAbility))
	for id := range SkillAbility {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ability := SkillAbility[id]
		proficient := contains(resolved, id)
		expert := truth(expertise[id]) && proficient
		status := "none"
		multiplier := 0
		if expert {
			status, multiplier = "expertise", 2
		} else if proficient {
			status, multiplier = "proficient", 1
		}
		proficiencies[id] = status
		skills[id] = Object{
			"ability": ability, "mod": mods[ability], "proficient": proficient,
			"expertise": expert, "total": mods[ability] + multiplier*pb,
		}
	}
	sheet["skills"] = skills
	passive := 10 + integer(object(skills["perception"])["total"], 0)
	sheet["passives"] = Object{"perception": passive}
	object(sheet["derived"])["passivePerception"] = passive
}

func hydrateProficiencies(decisions, sheet Object, classes []resolvedClass, background Object, records Records) {
	armor := make([]string, 0)
	tools := make([]string, 0)
	weapons := append([]string(nil), stringsOf(decisions["weaponProficiencies"])...)
	for index, current := range classes {
		source := object(current.Record["startingProficiencies"])
		if index > 0 && current.Record["multiclassProficiencies"] != nil {
			source = object(current.Record["multiclassProficiencies"])
		}
		armor = append(armor, stringsOf(source["armor"])...)
		tools = append(tools, stringsOf(source["tools"])...)
		weapons = append(weapons, stringsOf(source["weapons"])...)
	}
	if declared := text(background["toolProficiency"]); declared != "" && background["toolProficiencyChoice"] == nil {
		tool := selectedRecord(declared, records, "tool")
		if tool != nil {
			tools = append(tools, text(tool["id"]))
		}
	}
	tools = append(tools, text(decisions["backgroundToolProficiency"]))
	tools = append(tools, stringsOf(decisions["toolProficiencies"])...)
	tools = append(tools, stringsOf(decisions["speciesToolProficiencies"])...)
	armor = append(armor, stringsOf(decisions["armorProficiencies"])...)
	proficiencies := object(sheet["proficiencies"])
	proficiencies["armor"] = anyStrings(unique(armor))
	proficiencies["weapons"] = anyStrings(unique(weapons))
	proficiencies["tools"] = anyStrings(unique(tools))
	languages := unique(stringsOf(decisions["languageProficiencies"]))
	proficiencies["languages"] = anyStrings(languages)
	sheet["languages"] = anyStrings(languages)
}

func hydrateSpellcasting(
	_ Object,
	sheet Object,
	classes []resolvedClass,
	feats []Object,
	records Records,
	ruleset Ruleset,
	mods map[string]int,
	pb int,
) {
	casters := make([]caster, 0)
	perClass := make([]any, 0)
	for _, current := range classes {
		casting := object(current.Record["spellcasting"])
		subclass := recordByID(records, "subclass", current.Subclass)
		if casting == nil {
			casting = object(subclass["spellcasting"])
		}
		if casting == nil {
			continue
		}
		progression := progressionAt(objects(current.Record["progression"]), current.Level)
		if subclass != nil && len(objects(subclass["progression"])) > 0 {
			progression = progressionAt(objects(subclass["progression"]), current.Level)
		}
		var pact *PactMagicResult
		if text(casting["type"]) == "pact" {
			pact = PactMagic(current.Level, ruleset)
		}
		ability := text(casting["ability"])
		prepares := firstText(casting["prepares"], "list")
		spellbookKnown := 0
		if prepares == "spellbook" {
			spellbookKnown = ruleset.Constants.Spellbook.BaseKnown +
				ruleset.Constants.Spellbook.KnownPerLevel*(current.Level-1)
		}
		maxSpellLevel := maximumSpellLevel(text(casting["type"]), current.Level, progression, ruleset)
		if pact != nil {
			maxSpellLevel = pact.Level
		}
		var pactValue any
		if pact != nil {
			pactValue = Object{"slots": pact.Slots, "level": pact.Level}
		}
		entry := Object{
			"classId": current.ID, "level": current.Level, "ability": ability,
			"type": text(casting["type"]), "prepares": prepares, "ritual": truth(casting["ritual"]),
			"spellListClassId": firstText(casting["spellListClassId"], current.ID),
			"saveDC":           8 + pb + mods[ability], "spellAttack": pb + mods[ability],
			"preparedLimit":  integer(progression["preparedSpells"], 0),
			"cantripsKnown":  integer(progression["cantripsKnown"], 0),
			"spellbookKnown": spellbookKnown, "maxSpellLevel": maxSpellLevel,
			"pact": pactValue,
		}
		casters = append(casters, caster{class: current, casting: casting, progress: progression, pact: pact})
		perClass = append(perClass, entry)
	}
	expanded := make([]string, 0)
	for _, feat := range feats {
		expanded = append(expanded, stringsOf(object(feat["grants"])["spellList"])...)
	}
	for _, raw := range perClass {
		object(raw)["expandedSpellIds"] = anyStrings(unique(expanded))
	}

	casterLevel := 0
	var slots []int
	if len(casters) == 1 {
		only := casters[0]
		divisor := casterDivisor(text(only.casting["type"]))
		if divisor > 0 {
			casterLevel = int(math.Ceil(float64(only.class.Level) / float64(divisor)))
		}
		if text(only.casting["type"]) != "pact" && only.progress["spellSlots"] != nil {
			slots = integersOf(only.progress["spellSlots"])
		}
	} else {
		for _, current := range casters {
			casterLevel += CasterContribution(text(current.casting["type"]), current.class.Level, ruleset)
		}
	}
	if slots == nil {
		slots = MulticlassSlots(casterLevel, ruleset)
	}
	sheet["spellcasting"] = Object{
		"perClass": perClass, "casterLevel": casterLevel, "slots": anyInts(slots),
		"granted": []any{}, "pendingChoices": []any{}, "castingAbilityChoices": []any{},
	}
}

func hydrateWeaponMastery(decisions, sheet Object, classes []resolvedClass, feats []Object, ruleset Ruleset) {
	count := 0
	if ruleset.Capabilities.WeaponMastery != nil && *ruleset.Capabilities.WeaponMastery {
		for _, current := range classes {
			count = max(count, integer(object(current.Record["weaponMastery"])["count"], 0))
		}
		for _, feat := range feats {
			count += max(0, integer(object(feat["grants"])["weaponMasterySlots"], 0))
		}
	}
	sheet["weaponMastery"] = Object{
		"slots": count, "chosen": anyStrings(stringsOf(decisions["weaponMasteryChoices"])),
	}
}

func hydrateWeaponsAndAttunement(
	decisions, sheet Object,
	classes []resolvedClass,
	records Records,
	mods map[string]int,
	pb int,
	ruleset Ruleset,
	warn func(string),
) {
	proficiency := weaponProficiency(classes, stringsOf(object(sheet["proficiencies"])["weapons"]))
	mastery := make(map[string]struct{})
	for _, id := range stringsOf(decisions["weaponMasteryChoices"]) {
		mastery[id] = struct{}{}
	}
	weapons := make([]any, 0)
	attuned := 0
	for _, item := range objects(decisions["inventory"]) {
		if truth(item["attuned"]) {
			attuned++
		}
		location := firstText(item["location"], "pack")
		if location != "equipped" && location != "ready" {
			continue
		}
		record := inventoryRecord(item, records, "weapon")
		if record == nil {
			continue
		}
		weapons = append(weapons, weaponAttack(record, mods, pb, proficiency, mastery))
	}
	sheet["weapons"] = weapons
	limit := ruleset.Constants.AttunementLimit
	for _, current := range classes {
		for _, row := range objects(current.Record["attunementLimit"]) {
			if integer(row["level"], 1) <= current.Level {
				limit = max(limit, integer(row["max"], limit))
			}
		}
	}
	sheet["attunement"] = Object{"count": attuned, "limit": limit, "over": attuned > limit}
	if attuned > limit {
		warn(fmt.Sprintf("Attuned to more than %d magic items (limit %d)", limit, limit))
	}
}

func hydrateCoreResources(sheet Object, classes []resolvedClass, ruleset Ruleset) {
	resources := make([]any, 0)
	byDie := make(map[string]int)
	for _, current := range classes {
		die := text(current.Record["hitDie"])
		if die != "" {
			byDie[die] += current.Level
		}
	}
	dice := make([]string, 0, len(byDie))
	for die := range byDie {
		dice = append(dice, die)
	}
	sort.Strings(dice)
	amount := "full"
	if ruleset.Constants.Rest.LongRestHitDice == "half" {
		amount = "halfLevel"
	}
	for _, die := range dice {
		resources = append(resources, Object{
			"key": "hit-dice-" + die, "name": "Hit Dice (" + die + ")", "max": byDie[die],
			"kind": "hitdice", "die": die,
			"recharge": []any{Object{"on": "long", "amount": amount}},
			"source":   Object{"type": "class"},
		})
	}
	for index, count := range integersOf(object(sheet["spellcasting"])["slots"]) {
		if count < 1 {
			continue
		}
		resources = append(resources, Object{
			"key": fmt.Sprintf("slot-%d", index+1), "name": fmt.Sprintf("Spell Slots (%d)", index+1),
			"max": count, "kind": "slot",
			"recharge": []any{Object{"on": "long", "amount": "full"}},
			"source":   Object{"type": "spellcasting"},
		})
	}
	for _, raw := range values(object(sheet["spellcasting"])["perClass"]) {
		entry := object(raw)
		pact := object(entry["pact"])
		if pact == nil || integer(pact["slots"], 0) < 1 {
			continue
		}
		resources = append(resources, Object{
			"key": "pact-slot", "name": fmt.Sprintf("Pact Slots (%d)", integer(pact["level"], 0)),
			"max": integer(pact["slots"], 0), "kind": "slot",
			"recharge": []any{Object{"on": "short", "amount": "full"}},
			"source":   Object{"type": "pactMagic", "id": text(entry["classId"])},
		})
	}
	sheet["resources"] = resources
}

type weaponProf struct {
	simple, martial, martialLight, martialFinesseLight bool
	ids                                                map[string]struct{}
}

func weaponProficiency(classes []resolvedClass, extra []string) weaponProf {
	result := weaponProf{ids: make(map[string]struct{})}
	apply := func(token string) {
		switch token {
		case "simple":
			result.simple = true
		case "martial":
			result.martial = true
		case "martial-light":
			result.simple, result.martialLight = true, true
		case "martial-finesse-or-light":
			result.simple, result.martialFinesseLight = true, true
		case "":
		default:
			result.ids[token] = struct{}{}
		}
	}
	for index, current := range classes {
		source := object(current.Record["startingProficiencies"])
		if index > 0 && current.Record["multiclassProficiencies"] != nil {
			source = object(current.Record["multiclassProficiencies"])
		}
		for _, token := range stringsOf(source["weapons"]) {
			apply(token)
		}
	}
	for _, token := range extra {
		apply(token)
	}
	return result
}

func weaponAttack(record Object, mods map[string]int, pb int, prof weaponProf, mastery map[string]struct{}) Object {
	properties := stringsOf(record["properties"])
	_, explicit := prof.ids[text(record["id"])]
	proficient := explicit || text(record["category"]) == "simple" && prof.simple ||
		text(record["category"]) == "martial" && (prof.martial ||
			prof.martialLight && contains(properties, "light") ||
			prof.martialFinesseLight && (contains(properties, "finesse") || contains(properties, "light")))
	ability := mods["STR"]
	if text(record["range"]) == "ranged" {
		ability = mods["DEX"]
	}
	if contains(properties, "finesse") {
		ability = max(mods["STR"], mods["DEX"])
	}
	attack := ability
	if proficient {
		attack += pb
	}
	suffix := ""
	if ability != 0 {
		suffix = fmt.Sprintf(" %+d", ability)
	}
	_, mastered := mastery[text(record["id"])]
	var versatile any
	if text(record["versatileDamage"]) != "" {
		versatile = text(record["versatileDamage"]) + suffix
	}
	return Object{
		"ref": text(record["id"]), "name": text(record["name"]), "attackBonus": attack,
		"damage": text(record["damage"]) + suffix, "versatileDamage": versatile,
		"damageType": text(record["damageType"]), "properties": anyStrings(properties),
		"mastery": text(record["mastery"]), "masteryActive": mastered, "proficient": proficient,
	}
}

func progressionAt(progression []Object, level int) Object {
	var best Object
	bestLevel := -1
	for _, row := range progression {
		current := integer(row["level"], 0)
		if current <= level && current > bestLevel {
			best, bestLevel = row, current
		}
	}
	if best == nil && len(progression) > 0 {
		return progression[0]
	}
	return best
}

func maximumSpellLevel(kind string, level int, progression Object, ruleset Ruleset) int {
	if progression["spellSlots"] != nil {
		maximum := 0
		for index, count := range integersOf(progression["spellSlots"]) {
			if count > 0 {
				maximum = index + 1
			}
		}
		return maximum
	}
	divisor := casterDivisor(kind)
	if divisor == 0 {
		return 0
	}
	return len(MulticlassSlots(int(math.Ceil(float64(level)/float64(divisor))), ruleset))
}

func casterDivisor(kind string) int {
	switch kind {
	case "full":
		return 1
	case "half":
		return 2
	case "third":
		return 3
	default:
		return 0
	}
}

func CasterContribution(kind string, level int, ruleset Ruleset) int {
	if kind == "full" {
		return level
	}
	divisor := casterDivisor(kind)
	if divisor == 0 {
		return 0
	}
	ratio := float64(level) / float64(divisor)
	direction := ruleset.Constants.CasterFractions.Half
	if kind == "third" {
		direction = ruleset.Constants.CasterFractions.Third
	}
	if direction == "up" {
		return int(math.Ceil(ratio))
	}
	return int(math.Floor(ratio))
}

func selectedRecord(value any, records Records, kind string) Object {
	id := text(value)
	if id == "" {
		return nil
	}
	if record := recordByName(records, kind, id); record != nil {
		return record
	}
	return recordByID(records, kind, id)
}

func selectedFeats(decisions Object, records Records) []Object {
	result := make([]Object, 0)
	for _, value := range values(decisions["feats"]) {
		id, ok := value.(string)
		if !ok {
			current := object(value)
			id = firstText(current["featId"], current["id"])
		}
		if record := recordByID(records, "feat", id); record != nil {
			result = append(result, record)
		}
	}
	return result
}

func initiative(decisions Object, records Records, dexterity, proficiency int) int {
	result := dexterity
	for _, value := range values(decisions["feats"]) {
		id, _ := value.(string)
		if current := object(value); current != nil {
			id = firstText(current["featId"], current["id"])
		}
		feat := recordByID(records, "feat", id)
		for _, modifier := range objects(feat["modifiers"]) {
			if text(modifier["target"]) == "initiative" {
				if text(modifier["add"]) == "PB" {
					result += proficiency
				} else {
					result += integer(modifier["add"], 0)
				}
			}
		}
	}
	return result
}

func inventoryRecord(item Object, records Records, kind string) Object {
	if record := recordByID(records, kind, text(item["ref"])); record != nil {
		return record
	}
	return recordByName(records, kind, text(item["name"]))
}

func hitDieSize(hitDie string) int {
	match := regexp.MustCompile(`(?i)^d(\d+)$`).FindStringSubmatch(hitDie)
	if len(match) != 2 {
		return 8
	}
	return integer(match[1], 8)
}

func strengthRequirement(value any) int {
	match := regexp.MustCompile(`\d+`).FindString(text(value))
	if match != "" {
		return integer(match, 0)
	}
	return integer(value, 0)
}

func nullableText(value any) any {
	if result := text(value); result != "" {
		return result
	}
	return nil
}

func firstText(values ...any) string {
	for _, value := range values {
		if current := text(value); current != "" {
			return current
		}
	}
	return ""
}

func anyStrings(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func anyInts(values []int) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func integersOf(value any) []int {
	items := values(value)
	result := make([]int, 0, len(items))
	for _, item := range items {
		result = append(result, integer(item, 0))
	}
	return result
}

func conditionalInt(condition bool, value int) int {
	if condition {
		return value
	}
	return 0
}
