package rules

import (
	"math"
	"regexp"

	"github.com/pjunak/addon-dnd-engine/character"
)

var SkillAbility = map[string]string{
	"acrobatics": "DEX", "animalHandling": "WIS", "arcana": "INT", "athletics": "STR",
	"deception": "CHA", "history": "INT", "insight": "WIS", "intimidation": "CHA",
	"investigation": "INT", "medicine": "WIS", "nature": "INT", "perception": "WIS",
	"performance": "CHA", "persuasion": "CHA", "religion": "INT", "sleightOfHand": "DEX",
	"stealth": "DEX", "survival": "WIS",
}

// HydrationResult is the calculated statistics for a set of Builder
// decisions, and the warnings the calculation found.
type HydrationResult struct {
	Sheet    *character.Calculation `json:"sheet"`
	Warnings []string               `json:"warnings"`
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

// Hydrate calculates the statistics in a fixed order: each step reads what
// earlier steps wrote.
func Hydrate(decisions Object, records Records, ruleset Ruleset) HydrationResult {
	if decisions == nil {
		decisions = Object{}
	}
	warnings := make([]string, 0)
	warn := func(message string) {
		if message != "" {
			warnings = append(warnings, message)
		}
	}

	sheet := &character.Calculation{
		Abilities: map[string]character.AbilityScore{},
		Saves:     map[string]character.SaveRow{},
		Skills:    map[string]character.SkillRow{},
		Senses:    map[string]int{},
		Proficiencies: character.Proficiencies{
			Saves: map[string]bool{}, Skills: map[string]character.ProficiencyLevel{},
			Armor: []string{}, Weapons: []string{}, Tools: []string{}, Languages: []string{},
		},
	}
	mods := make(map[string]int, len(Abilities))
	hydrateAbilities(decisions, sheet, mods, ruleset)

	classes := resolveClasses(decisions, records, warn)
	totalLevel := 0
	for _, current := range classes {
		totalLevel += current.Level
	}
	totalLevel = max(1, totalLevel)
	pb := ruleset.ProficiencyBonus(totalLevel)
	for _, current := range classes {
		sheet.Classes = append(sheet.Classes, character.SheetClass{
			ClassID: current.ID, Name: current.Name, Level: current.Level,
			Subclass: current.Subclass, HitDie: text(current.Record["hitDie"]),
		})
	}
	hydrateClassValues(sheet, classes, records)
	sheet.TotalLevel = totalLevel
	sheet.Derived.ProficiencyBonus = pb
	if len(classes) > 0 && classes[0].Record != nil {
		sheet.Class = recordIdentity(classes[0].Record)
		sheet.Derived.HitDie = nullableText(classes[0].Record["hitDie"])
	}

	species, lineage := hydrateSpecies(decisions, records, sheet, warn)
	background := recordByID(records, "background", text(decisions["background"]))
	if decisions["background"] != nil && background == nil && records != nil {
		warn("Unknown background: " + text(decisions["background"]))
	}
	featRecords := selectedFeats(decisions, records)
	// Feat prerequisites check earlier feats by identity and category.
	for _, feat := range featRecords {
		sheet.FeatIdentities = append(sheet.FeatIdentities, character.FeatIdentity{ID: text(feat["id"]), Category: text(feat["category"])})
	}
	grantSources := applyChoicePackages(collectGrantSources(
		classes, species, lineage, background, featRecords, objects(decisions["featAcquisitions"]), objects(decisions["itemGrants"]), records, totalLevel,
		object(decisions["featureChoices"]),
	), object(decisions["featureChoices"]))
	activeModifiers := activeGrantModifiers(decisions, sheet, grantSources, classes, records)
	sheet.Speed += grantTotal(grantSources, "speedBonus")
	sheet.Derived.Speed = sheet.Speed
	hydrateHitPoints(sheet, classes, mods["CON"], grantSources, ruleset)
	hydrateArmorClass(decisions, sheet, classes, mods, records, species, grantSources, activeModifiers, ruleset)
	applySpeedBonuses(sheet, grantSources, classes, equippedArmor(decisions, records))
	sheet.Derived.Initiative = initiative(decisions, records, mods["DEX"], pb)
	if bonus, rows := grantCheckBonuses(grantSources, "initiativeBonuses", "", false, mods, pb); len(rows) > 0 {
		sheet.Derived.Initiative += bonus
		sheet.InitiativeBonuses = rows
	}
	hydrateSaves(decisions, sheet, classes, mods, pb, grantSources)
	hydrateSkills(decisions, sheet, background, mods, pb, grantSources, ruleset)
	hydrateProficiencies(decisions, sheet, classes, background, grantSources)
	applyGenericGrants(decisions, sheet, species, grantSources, activeModifiers)
	hydrateAdvantages(sheet, grantSources, activeModifiers, equippedArmor(decisions, records))
	hydrateSpellcasting(decisions, sheet, classes, featRecords, records, ruleset, mods, pb)
	applyCasterGrants(sheet, grantSources, activeModifiers)
	hydrateFeatureSaveDCs(sheet, grantSources, mods, pb, ruleset)
	hydrateSpellGrants(decisions, sheet, classes, featRecords, grantSources, records)
	hydrateWeaponMastery(decisions, sheet, classes, featRecords, ruleset)
	hydrateWeaponsAndAttunement(decisions, sheet, classes, records, mods, pb, ruleset, grantSources, activeModifiers, warn)
	hydrateFeatures(sheet, classes, records, object(decisions["featureChoices"]))
	hydrateResources(sheet, classes, grantSources, records, ruleset, pb)
	hydrateCurrencyWorth(decisions, sheet, ruleset)
	character.FillEmpty(sheet)
	return HydrationResult{Sheet: sheet, Warnings: warnings}
}

// recordIdentity names a record without copying its rules.
func recordIdentity(record Object) *character.RecordIdentity {
	return &character.RecordIdentity{ID: text(record["id"]), Name: text(record["name"]), Kind: text(record["kind"]), Book: text(record["book"])}
}

func hydrateAbilities(decisions Object, sheet *character.Calculation, mods map[string]int, ruleset Ruleset) {
	base := object(decisions["baseStats"])
	grants := objects(decisions["abilityGrants"])
	for _, ability := range Abilities {
		bonus, capValue := abilityGrantTotal(grants, ability, ruleset)
		if cap, exists := object(decisions["scoreCaps"])[ability]; exists {
			capValue = integer(cap, capValue)
		}
		baseScore := integer(base[ability], 10)
		score := min(capValue, baseScore+bonus)
		modifier := AbilityModifier(float64(score))
		mods[ability] = modifier
		sheet.Abilities[ability] = character.AbilityScore{Base: baseScore, Score: score, Mod: modifier, Bonus: bonus}
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
	return result
}

func hydrateSpecies(decisions Object, records Records, sheet *character.Calculation, warn func(string)) (Object, Object) {
	selected := text(decisions["species"])
	species := recordByID(records, "species", selected)
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
	resistances := make([]string, 0)
	if species != nil {
		sheet.Species = recordIdentity(species)
		darkvision = integer(object(species["senses"])["darkvision"], 0)
		resistances = append(resistances, stringsOf(species["resistances"])...)
	}
	if lineage != nil {
		grants := object(lineage["grants"])
		darkvision = max(darkvision, integer(object(grants["senses"])["darkvision"], 0))
		resistances = append(resistances, stringsOf(grants["resistances"])...)
	}
	speed := 30
	if species != nil {
		speed = integer(object(species["speeds"])["walk"], 30)
	}
	sheet.Speed = speed
	sheet.Derived.Speed = speed
	if darkvision > 0 {
		sheet.Senses["darkvision"] = darkvision
	}
	sheet.Resistances = unique(resistances)
	return species, lineage
}

func hydrateHitPoints(sheet *character.Calculation, classes []resolvedClass, conMod int, sources []grantSource, ruleset Ruleset) {
	perLevel := grantTotal(sources, "hpPerLevel")
	fixedBonus := grantTotal(sources, "hpBonus")
	// hpPerClassLevel counts levels in the granting class (Draconic Resilience).
	for _, source := range sources {
		fixedBonus += integer(source.Grants["hpPerClassLevel"], 0) * source.Level
	}
	dice := 0
	level := 0
	maximum := 0
	for _, current := range classes {
		die := hitDieSize(text(current.Record["hitDie"]))
		for index := 0; index < current.Level; index++ {
			level++
			gained, result, _ := ruleset.LevelHitPoints(level, die, nil, conMod, perLevel)
			dice += result
			maximum += gained
		}
	}
	if level > 0 {
		maximum += fixedBonus
	}
	sheet.HP = character.HitPoints{
		Max: maximum,
		Breakdown: character.HPBreakdown{
			Dice: dice, ConMod: conMod, ConTotal: conMod * level,
			MiscPerLevel: perLevel, MiscTotal: perLevel * level, Level: level, FixedBonus: fixedBonus,
		},
	}
	for _, source := range sources {
		value := integer(source.Grants["hpBonus"], 0) + integer(source.Grants["hpPerClassLevel"], 0)*source.Level
		if value != 0 {
			sheet.HP.FixedBonuses = append(sheet.HP.FixedBonuses, character.NamedValue{
				Name: firstText(source.Record["name"], source.Source.ID), Source: source.Source, Value: value,
			})
		}
	}
	sheet.Derived.MaxHP = maximum
}

func hydrateArmorClass(
	decisions Object,
	sheet *character.Calculation,
	classes []resolvedClass,
	mods map[string]int,
	records Records,
	species Object,
	sources []grantSource,
	activeModifiers []activeModifier,
	ruleset Ruleset,
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
	candidates := make([]character.ACCandidate, 0)
	formulaCandidates := func(formulas []Object) {
		for _, formula := range formulas {
			if truth(object(formula["requires"])["noShield"]) && shield != nil {
				continue
			}
			addition := 0
			for _, ability := range stringsOf(formula["addAbilities"]) {
				addition += mods[ability]
			}
			candidates = append(candidates, character.ACCandidate{
				ID: text(formula["id"]), Label: firstText(formula["name"], formula["id"]),
				Value: integer(formula["base"], 10) + addition,
			})
		}
	}
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
		candidates = append(candidates, character.ACCandidate{
			ID: "armor:" + text(bodyArmor["id"]), Label: text(bodyArmor["name"]),
			Value: integer(bodyArmor["baseAC"], 10) + dexPart,
		})
	} else {
		for _, current := range classes {
			formulaCandidates(objects(current.Record["acFormulas"]))
		}
		for _, source := range sources {
			if source.Source.Type == character.SourceClass {
				continue
			}
			formulaCandidates(append(append([]Object(nil), objects(source.Record["acFormulas"])...), objects(source.Grants["acFormulas"])...))
		}
	}
	if bodyArmor == nil {
		candidates = append(candidates, character.ACCandidate{ID: "unarmored", Label: "Unarmored", Value: ruleset.Constants.UnarmoredArmorClassBase + mods["DEX"]})
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Value > best.Value {
			best = candidate
		}
	}
	shieldBonus := 0
	if shield != nil {
		shieldBonus = integer(shield["acBonus"], 2)
	}
	speciesBonus := integer(object(species["grants"])["acBonus"], 0)
	activeBonus := 0
	for _, modifier := range activeModifiers {
		if modifier.target() == "armorClass" {
			activeBonus += integer(modifier.fields["add"], 0) + mods[text(modifier.fields["addAbility"])]
		}
	}
	bonuses, passiveBonus := armorGrantBonuses(sources, bodyArmor)
	value := best.Value + shieldBonus + speciesBonus + activeBonus + passiveBonus
	sheet.AC = character.ArmorClass{
		Value: value, Base: best.Label, Shield: shieldBonus, Candidates: candidates,
		SpeciesBonus: speciesBonus, ActiveBonus: activeBonus, Bonuses: bonuses,
	}
	if bodyArmor != nil {
		requirement := strengthRequirement(bodyArmor["strReq"])
		strength := sheet.Abilities["STR"].Score
		restrictions := character.ArmorRestrictions{
			ArmorID: text(bodyArmor["id"]), ArmorName: text(bodyArmor["name"]), StrengthRequirement: requirement,
			MeetsStrength:       requirement == 0 || strength >= requirement,
			SpeedPenalty:        conditionalInt(requirement > 0 && strength < requirement, 10),
			StealthDisadvantage: truth(bodyArmor["stealthDisadvantage"]),
		}
		forAC, forSheet := restrictions, restrictions
		sheet.AC.Restrictions, sheet.ArmorRestrictions = &forAC, &forSheet
		if restrictions.SpeedPenalty > 0 {
			sheet.Speed = max(0, sheet.Speed-restrictions.SpeedPenalty)
			sheet.Derived.Speed = sheet.Speed
		}
	}
	sheet.Derived.ArmorClass = value
}

func hydrateSaves(
	decisions Object,
	sheet *character.Calculation,
	classes []resolvedClass,
	mods map[string]int,
	pb int,
	sources []grantSource,
) {
	first := []string{}
	if len(classes) > 0 {
		first = stringsOf(classes[0].Record["savingThrows"])
	}
	manual := object(decisions["saveProf"])
	granted := append(grantValues(sources, "savingThrows"), stringsOf(decisions["saveProficiencies"])...)
	for _, ability := range Abilities {
		proficient := contains(first, ability) || contains(granted, ability) || truth(manual[ability])
		sheet.Proficiencies.Saves[ability] = proficient
		bonus, bonuses := grantCheckBonuses(sources, "saveBonuses", ability, proficient, mods, pb)
		total := mods[ability] + bonus
		if proficient {
			total += pb
		}
		sheet.Saves[ability] = character.SaveRow{Mod: mods[ability], Proficient: proficient, Total: total, Bonuses: bonuses}
	}
}

func hydrateSkills(
	decisions Object,
	sheet *character.Calculation,
	background Object,
	mods map[string]int,
	pb int,
	sources []grantSource,
	ruleset Ruleset,
) {
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
	resolved = append(resolved, grantValues(sources, "skills")...)
	for index, id := range resolved {
		resolved[index] = canonicalSkill(id)
	}
	expertise := map[string]bool{}
	for id, selected := range object(decisions["skillExpertise"]) {
		if truth(selected) {
			expertise[canonicalSkill(id)] = true
		}
	}
	for _, id := range grantValues(sources, "expertise") {
		expertise[canonicalSkill(id)] = true
	}
	// Keen Mind and Observant grant proficiency, or Expertise when the skill is
	// already proficient from any other source.
	for _, id := range stringsOf(decisions["skillUpgrades"]) {
		if id = canonicalSkill(id); contains(resolved, id) {
			expertise[id] = true
		} else {
			resolved = append(resolved, id)
		}
	}
	for _, id := range sortedKeys(SkillAbility) {
		ability := SkillAbility[id]
		proficient := contains(resolved, id)
		expert := expertise[id] && proficient
		status := character.TrainingNone
		multiplier := 0
		if expert {
			status, multiplier = character.TrainingExpertise, 2
		} else if proficient {
			status, multiplier = character.TrainingProficient, 1
		}
		sheet.Proficiencies.Skills[id] = status
		bonus, bonuses := grantCheckBonuses(sources, "skillBonuses", id, proficient, mods, pb)
		sheet.Skills[id] = character.SkillRow{
			Ability: ability, Mod: mods[ability], Proficient: proficient,
			Expertise: expert, Total: mods[ability] + multiplier*pb + bonus, Bonuses: bonuses,
		}
	}
	passive := ruleset.Constants.PassiveCheckBase + sheet.Skills["perception"].Total
	sheet.Passives = character.Passives{Perception: passive}
	sheet.Derived.PassivePerception = passive
}

func classProficiencies(record Object, initial bool) Object {
	if initial {
		return object(record["startingProficiencies"])
	}
	return object(record["multiclassProficiencies"])
}

func hydrateProficiencies(
	decisions Object,
	sheet *character.Calculation,
	classes []resolvedClass,
	background Object,
	sources []grantSource,
) {
	armor := make([]string, 0)
	tools := make([]string, 0)
	weapons := append(grantValues(sources, "weapons"), stringsOf(decisions["weaponProficiencies"])...)
	for index, current := range classes {
		source := classProficiencies(current.Record, index == 0)
		armor = append(armor, stringsOf(source["armor"])...)
		tools = append(tools, stringsOf(source["tools"])...)
		weapons = append(weapons, stringsOf(source["weapons"])...)
	}
	if background["toolProficiencyChoice"] == nil {
		tools = append(tools, text(background["toolProficiency"]))
	}
	tools = append(tools, text(decisions["backgroundToolProficiency"]))
	tools = append(tools, stringsOf(decisions["toolProficiencies"])...)
	tools = append(tools, stringsOf(decisions["speciesToolProficiencies"])...)
	armor = append(armor, stringsOf(decisions["armorProficiencies"])...)
	armor = append(armor, grantValues(sources, "armor")...)
	tools = append(tools, grantValues(sources, "tools")...)
	sheet.Proficiencies.Armor = unique(armor)
	sheet.Proficiencies.Weapons = unique(weapons)
	sheet.Proficiencies.Tools = unique(tools)
	languages := unique(stringsOf(decisions["languageProficiencies"]))
	sheet.Proficiencies.Languages = languages
	sheet.Languages = append([]string{}, languages...)
}

func hydrateSpellcasting(
	decisions Object,
	sheet *character.Calculation,
	classes []resolvedClass,
	feats []Object,
	records Records,
	ruleset Ruleset,
	mods map[string]int,
	pb int,
) {
	casters := make([]caster, 0)
	perClass := make([]character.Caster, 0)
	for _, current := range classes {
		castingSource := current.Record
		casting := object(castingSource["spellcasting"])
		if casting == nil {
			castingSource = recordByID(records, "subclass", current.Subclass)
			casting = object(castingSource["spellcasting"])
		}
		if casting == nil {
			continue
		}
		progression := progressionAt(objects(castingSource["progression"]), current.Level)
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
		var pactSlots *character.PactSlots
		if pact != nil {
			maxSpellLevel = pact.Level
			pactSlots = &character.PactSlots{Slots: pact.Slots, Level: pact.Level}
		}
		perClass = append(perClass, character.Caster{
			ClassID: current.ID, Level: current.Level, Ability: ability,
			Type: text(casting["type"]), Prepares: prepares, Ritual: truth(casting["ritual"]),
			SpellListClassID: firstText(casting["spellListClassId"], current.ID),
			SaveDC:           ruleset.Constants.SpellSaveDCBase + pb + mods[ability], SpellAttack: pb + mods[ability],
			PreparedLimit:  integer(progression["preparedSpells"], 0),
			CantripsKnown:  integer(progression["cantripsKnown"], 0),
			SpellbookKnown: spellbookKnown, MaxSpellLevel: maxSpellLevel,
			Pact:          pactSlots,
			PreparedCount: len(unique(stringsOf(object(decisions["preparedSpells"])[current.ID]))),
			Preparation:   spellPreparation(casting, recordByID(records, "subclass", current.Subclass)),
		})
		casters = append(casters, caster{class: current, casting: casting, progress: progression, pact: pact})
	}
	expanded := make([]string, 0)
	for _, feat := range feats {
		expanded = append(expanded, stringsOf(object(feat["grants"])["spellList"])...)
	}
	for index := range perClass {
		perClass[index].ExpandedSpellIDs = unique(expanded)
	}

	// Pact Magic has its own pool and does not turn one Spellcasting class
	// into multiple Spellcasting classes for shared-slot progression.
	spellcasters := make([]caster, 0, len(casters))
	for _, current := range casters {
		if text(current.casting["type"]) != "pact" {
			spellcasters = append(spellcasters, current)
		}
	}
	casterLevel := 0
	var slots []int
	if len(spellcasters) == 1 {
		only := spellcasters[0]
		divisor := casterDivisor(text(only.casting["type"]))
		if divisor > 0 {
			casterLevel = int(math.Ceil(float64(only.class.Level) / float64(divisor)))
		}
		if text(only.casting["type"]) != "pact" && only.progress["spellSlots"] != nil {
			slots = integersOf(only.progress["spellSlots"])
		}
	} else {
		for _, current := range spellcasters {
			casterLevel += CasterContribution(text(current.casting["type"]), current.class.Level, ruleset)
		}
	}
	if slots == nil {
		slots = MulticlassSlots(casterLevel, ruleset)
	}
	sheet.Spellcasting = character.Spellcasting{PerClass: perClass, CasterLevel: casterLevel, Slots: slots}
}

// sourceClassID is the class a grant source belongs to: the class itself or
// the class that owns a subclass or feature record.
func sourceClassID(source grantSource) string {
	if source.Source.Type == character.SourceClass {
		return source.Source.ID
	}
	return text(source.Record["classId"])
}

// applyCasterGrants applies class and subclass grants that change their own
// class's spellcasting: spellLists lets the class prepare spells from other
// classes' lists (Magical Secrets), and spellbookBonus steps
// ([{level, count, school?}] by class level) add spellbook spells (the wizard
// Savants). The reached step's school, when named, limits those spells.
func applyCasterGrants(sheet *character.Calculation, sources []grantSource, modifiers []activeModifier) {
	for index := range sheet.Spellcasting.PerClass {
		caster := &sheet.Spellcasting.PerClass[index]
		lists, bonus, bonusSources := []string{}, 0, []character.SpellbookBonusSource{}
		for _, source := range sources {
			if sourceClassID(source) != caster.ClassID {
				continue
			}
			lists = append(lists, stringsOf(source.Grants["spellLists"])...)
			step, school := 0, ""
			for _, row := range objects(source.Grants["spellbookBonus"]) {
				if integer(row["level"], 1) <= source.Level {
					step, school = integer(row["count"], step), text(row["school"])
				}
			}
			if step > 0 {
				bonus += step
				bonusSources = append(bonusSources, character.SpellbookBonusSource{Source: source.Source, Count: step, School: school})
			}
		}
		if len(lists) > 0 {
			caster.SpellLists = unique(lists)
		}
		if bonus > 0 && caster.Prepares == "spellbook" {
			caster.SpellbookKnown += bonus
			caster.SpellbookBonus = bonus
			caster.SpellbookBonusSources = bonusSources
		}
		// An active spellSaveDC modifier raises its own class's spell save DC
		// (Innate Sorcery).
		for _, modifier := range modifiers {
			if modifier.target() == "spellSaveDC" && modifier.classID == caster.ClassID {
				caster.SaveDC += integer(modifier.fields["add"], 0)
			}
		}
	}
}

// hydrateFeatureSaveDCs lists the save DCs features set for their own effects,
// such as a Monk's Focus or a Battle Master's maneuvers: the ruleset's DC base
// plus the proficiency bonus and the best of the named ability modifiers.
func hydrateFeatureSaveDCs(sheet *character.Calculation, sources []grantSource, mods map[string]int, pb int, ruleset Ruleset) {
	for _, source := range sources {
		for _, dc := range objects(source.Grants["saveDCs"]) {
			abilities := stringsOf(dc["abilities"])
			if len(abilities) == 0 {
				continue
			}
			best := abilities[0]
			for _, ability := range abilities[1:] {
				if mods[ability] > mods[best] {
					best = ability
				}
			}
			sheet.SaveDCs = append(sheet.SaveDCs, character.SaveDC{
				Name: firstText(dc["name"], source.Record["name"], source.Source.ID), Ability: best,
				Value: ruleset.Constants.SpellSaveDCBase + pb + mods[best], Source: source.Source,
			})
		}
	}
}

// hydrateCurrencyWorth totals the authored coins in the ruleset's worth coin.
// Coins the ruleset gives no value are left out of the total.
func hydrateCurrencyWorth(decisions Object, sheet *character.Calculation, ruleset Ruleset) {
	policy := ruleset.Constants.Currency
	if policy == nil {
		return
	}
	total := 0.0
	currency := object(decisions["currency"])
	for _, coin := range sortedKeys(currency) {
		total += number(currency[coin], 0) * float64(policy.CoinValues[coin])
	}
	sheet.CurrencyWorth = &character.CurrencyWorth{Coin: policy.WorthIn, Value: total / float64(policy.CoinValues[policy.WorthIn])}
}

// spellPreparation says when a caster may change its prepared list: after a
// long rest (the source's preparationChange) or on gaining a class level (its
// levelReplacements, which a subclass may override). It is display guidance;
// preparation itself is not restricted by it.
func spellPreparation(casting, subclass Object) *character.Preparation {
	change := object(casting["preparationChange"])
	if text(change["on"]) == "longRest" {
		if text(change["count"]) == "all" {
			return &character.Preparation{On: "longRest", Count: character.PreparationCount{All: true}}
		}
		if count := number(change["count"], 0); count >= 1 && count == math.Floor(count) {
			return &character.Preparation{On: "longRest", Count: character.PreparationCount{Count: int(count)}}
		}
	}
	replacements := casting["levelReplacements"]
	if extra := object(subclass["spellcasting"]); extra["levelReplacements"] != nil {
		replacements = extra["levelReplacements"]
	}
	if count := integer(replacements, 0); count > 0 {
		return &character.Preparation{On: "classLevel", Count: character.PreparationCount{Count: count}}
	}
	return nil
}

func hydrateWeaponMastery(decisions Object, sheet *character.Calculation, classes []resolvedClass, feats []Object, ruleset Ruleset) {
	count := 0
	if ruleset.Capabilities.WeaponMastery != nil && *ruleset.Capabilities.WeaponMastery {
		// Each class's Weapon Mastery feature grants its own kinds of weapons.
		for _, current := range classes {
			count += weaponMasteryCount(current.Record, current.Level)
		}
		for _, feat := range feats {
			count += max(0, integer(object(feat["grants"])["weaponMasterySlots"], 0))
		}
	}
	sheet.WeaponMastery = character.WeaponMastery{Slots: count, Chosen: stringsOf(decisions["weaponMasteryChoices"])}
}

func hydrateWeaponsAndAttunement(
	decisions Object,
	sheet *character.Calculation,
	classes []resolvedClass,
	records Records,
	mods map[string]int,
	pb int,
	ruleset Ruleset,
	sources []grantSource,
	activeModifiers []activeModifier,
	warn func(string),
) {
	proficiency := weaponProficiency(classes, sheet.Proficiencies.Weapons)
	mastery := make(map[string]struct{})
	for _, id := range stringsOf(decisions["weaponMasteryChoices"]) {
		mastery[id] = struct{}{}
	}
	weapons := make([]character.Attack, 0)
	attuned := 0
	weaponAbilities := make([]Object, 0)
	for _, modifier := range activeModifiers {
		if modifier.target() == "weaponAbility" && text(modifier.fields["ability"]) != "" {
			weaponAbilities = append(weaponAbilities, modifier.fields)
		}
	}
	context := attackContext{
		mods: mods, pb: pb, proficiency: proficiency, mastery: mastery, weaponAbilities: weaponAbilities,
		rules: collectAttackRules(sources, classes, activeModifiers), armor: equippedArmor(decisions, records),
		reach: ruleset.Constants.MeleeReach,
	}
	limit := ruleset.Constants.AttunementLimit
	for _, source := range sources {
		if critical := integer(source.Grants["criticalRange"], 0); critical > 1 && (context.criticalRange == 0 || critical < context.criticalRange) {
			context.criticalRange = critical
		}
		limit = max(limit, integer(source.Grants["attunementLimit"], 0))
	}
	if unarmed := ruleset.Constants.UnarmedStrike; unarmed != nil {
		weapons = append(weapons, attackRow(attackSubject{record: Object{
			"name": unarmed.Name, "damage": unarmed.Damage, "damageType": unarmed.DamageType,
		}, unarmed: true}, context))
	}
	// Feature attacks, such as Psychic Blades, are weapons a feature creates.
	for _, source := range sources {
		for _, attack := range objects(source.Grants["attacks"]) {
			id := text(attack["id"])
			if id == "" {
				continue
			}
			featureSource := source.Source
			row := attackRow(attackSubject{record: attack, featureID: id, source: &featureSource}, context)
			row.MasteryActive = text(attack["mastery"]) != "" && truth(attack["masteryActive"])
			weapons = append(weapons, row)
		}
	}
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
		attack := attackRow(attackSubject{record: record}, context)
		if grip := text(item["handGrip"]); grip != "" {
			attack.ItemID, attack.Grip = text(item["id"]), character.Grip(grip)
			if grip == "two" && attack.VersatileDamage != nil {
				attack.Damage = *attack.VersatileDamage
			}
		}
		weapons = append(weapons, attack)
	}
	sheet.Weapons = weapons
	for _, current := range classes {
		for _, row := range objects(current.Record["attunementLimit"]) {
			if integer(row["level"], 1) <= current.Level {
				limit = max(limit, integer(row["max"], limit))
			}
		}
	}
	sheet.Attunement = character.Attunement{Count: attuned, Limit: limit, Over: attuned > limit}
	if attuned > limit {
		warn(attunementCapacityWarning(limit))
	}
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
			result.martialLight = true
		case "martial-finesse-or-light":
			result.martialFinesseLight = true
		case "":
		default:
			result.ids[token] = struct{}{}
		}
	}
	for index, current := range classes {
		source := classProficiencies(current.Record, index == 0)
		for _, token := range stringsOf(source["weapons"]) {
			apply(token)
		}
	}
	for _, token := range extra {
		apply(token)
	}
	return result
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
	if declared := text(item["kind"]); kind == "armor" && declared != "" {
		record := recordByID(records, declared, text(item["ref"]))
		if contains([]string{"light", "medium", "heavy", "shield"}, text(record["armorType"])) {
			return record
		}
		return nil
	}
	return recordByID(records, kind, text(item["ref"]))
}

var (
	hitDiePattern = regexp.MustCompile(`(?i)^d(\d+)$`)
	digitsPattern = regexp.MustCompile(`\d+`)
)

// hitDieSize reads a "dN" hit die, or 0 when the record's value is malformed;
// validation reports that class instead of assuming a die.
func hitDieSize(hitDie string) int {
	match := hitDiePattern.FindStringSubmatch(hitDie)
	if len(match) != 2 {
		return 0
	}
	return integer(match[1], 0)
}

func strengthRequirement(value any) int {
	match := digitsPattern.FindString(text(value))
	if match != "" {
		return integer(match, 0)
	}
	return integer(value, 0)
}

func nullableText(value any) *string {
	if result := text(value); result != "" {
		return &result
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
