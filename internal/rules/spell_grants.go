package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// spellGrantOptions are how a granted spell is prepared and cast.
type spellGrantOptions struct {
	alwaysPrepared   bool
	free             string
	castingAbility   string
	atWill           bool
	ritualOnly       bool
	castResource     string
	castResourceCost int
	castAtLevel      *float64
	resourceKey      string
}

func hydrateSpellGrants(
	decisions Object,
	sheet *character.Calculation,
	classes []resolvedClass,
	feats []Object,
	sources []grantSource,
	records Records,
) {
	granted := make([]character.SpellGrant, 0)
	pendingChoices := make([]character.SpellChoice, 0)
	castingChoices := make([]character.CastingAbilityChoice, 0)
	castingKeys := make(map[string]struct{})
	grantChoices := object(decisions["grantChoices"])
	grantCastingAbilities := object(decisions["grantCastingAbilities"])
	featureChoices := object(decisions["featureChoices"])

	castingAbilityFor := func(source character.Source, sourceGrants Object) string {
		grants := sourceGrants
		if grants == nil {
			for _, candidate := range sources {
				if grantOwner(candidate.Source) == grantOwner(source) &&
					candidate.Grants["castingAbility"] != nil {
					grants = candidate.Grants
					break
				}
			}
		}
		declaration := object(grants["castingAbility"])
		if declaration == nil {
			return ""
		}
		if fixed := text(declaration["fixed"]); fixed != "" {
			return fixed
		}
		if truth(declaration["fromAbilityScoreIncrease"]) {
			for _, grant := range objects(decisions["abilityGrants"]) {
				if text(object(grant["source"])["type"]) != "feat" {
					continue
				}
				choiceID := text(grant["id"])
				if strings.HasSuffix(choiceID, ":featability") {
					choiceID = strings.TrimSuffix(choiceID, ":featability") + ":feat"
				}
				if text(featureChoices[choiceID]) != source.ID || source.Acquisition != nil && source.Acquisition.ID != choiceID {
					continue
				}
				for _, ability := range Abilities {
					if integer(object(grant["assign"])[ability], 0) > 0 {
						return ability
					}
				}
			}
			return ""
		}
		options := stringsOf(declaration["choose"])
		localID := firstText(declaration["id"], "casting-ability")
		key := grantOwner(source) + ":" + localID
		selected := text(grantCastingAbilities[key])
		if !contains(options, selected) {
			selected = ""
		}
		if _, exists := castingKeys[key]; !exists {
			castingKeys[key] = struct{}{}
			castingChoices = append(castingChoices, character.CastingAbilityChoice{
				Key: key, Source: source, Options: options, Selected: nullableText(selected),
			})
		}
		return selected
	}

	// maxSpellLevel "class" is the highest spell level the granting class can
	// cast, such as Magical Discoveries' "a level for which you have slots".
	casterMaxSpellLevel := func(source character.Source) int {
		classID := source.ClassID
		if classID == "" && source.Type == character.SourceClass {
			classID = source.ID
		} else if classID == "" {
			classID = text(recordByID(records, string(source.Type), source.ID)["classId"])
		}
		for _, caster := range sheet.Spellcasting.PerClass {
			if caster.ClassID == classID {
				return caster.MaxSpellLevel
			}
		}
		return 0
	}

	addGrant := func(reference string, source character.Source, options spellGrantOptions) {
		if reference == "" {
			return
		}
		record := recordByID(records, "spell", reference)
		var level *int
		if record != nil {
			value := integer(record["level"], 0)
			level = &value
		}
		entry := character.SpellGrant{
			Ref: reference, Name: firstText(record["name"], reference), Level: level,
			School: text(record["school"]), Source: source,
			AlwaysPrepared: options.alwaysPrepared,
			Free:           nullableText(options.free), CastingAbility: nullableText(options.castingAbility),
			CastAtLevel: options.castAtLevel, ResourceKey: options.resourceKey,
			AtWill: options.atWill, RitualOnly: options.ritualOnly,
			CastResource: options.castResource,
		}
		if options.castResource != "" && options.castResourceCost > 1 {
			entry.CastResourceCost = options.castResourceCost
		}
		granted = append(granted, entry)
	}

	addGrantEntry := func(spell Object, source character.Source, unlocked bool, sourceLevel int, sourceGrants Object) {
		if !unlocked {
			return
		}
		castingAbility := castingAbilityFor(source, sourceGrants)
		var castAtLevel *float64
		for _, row := range objects(spell["castAtLevelByLevel"]) {
			if sourceLevel >= integer(row["level"], 0) {
				value := float64(integer(row["castAtLevel"], 0))
				castAtLevel = &value
			}
		}
		if classID := text(spell["classId"]); classID != "" {
			source.ClassID = classID
		}
		options := spellGrantOptions{
			alwaysPrepared: truth(spell["alwaysPrepared"]), free: text(spell["free"]),
			castingAbility: castingAbility, atWill: truth(spell["atWill"]), ritualOnly: truth(spell["ritualOnly"]),
			castAtLevel: castAtLevel,
		}
		// A spell cast by spending a declared resource (a key, or {key, cost}),
		// such as Misty Step through Steps of the Fey.
		if resource := grantActivationResource(source, spell["resource"]); resource != nil {
			options.castResource, options.castResourceCost = resource.Key, resource.Cost
		}
		if ids := stringsOf(spell["ids"]); len(ids) > 0 {
			for _, reference := range ids {
				addGrant(reference, source, options)
			}
			return
		}
		count := max(0, integer(spell["choose"], 0))
		choiceID := text(spell["id"])
		if count == 0 || choiceID == "" {
			return
		}
		key := grantOwner(source) + ":" + choiceID
		if count == 1 && text(spell["free"]) != "" {
			options.resourceKey = "charge:" + key
		}
		picked := stringsOf(grantChoices[key])
		if len(picked) > count {
			picked = picked[:count]
		}
		_, explicitlySelected := grantChoices[key]
		if len(picked) == 0 && !explicitlySelected && text(spell["default"]) != "" {
			picked = []string{text(spell["default"])}
		}
		for _, reference := range picked {
			addGrant(reference, source, options)
		}
		maxSpellLevel := optionalInteger(spell, "maxSpellLevel")
		if text(spell["maxSpellLevel"]) == "class" {
			level := casterMaxSpellLevel(source)
			maxSpellLevel = &level
		}
		choice := character.SpellChoice{
			Key: key, Source: source, Choose: count,
			SpellLevel:    optionalInteger(spell, "spellLevel"),
			MaxSpellLevel: maxSpellLevel,
			From:          spellFilterOf(spell["from"]), Default: nullableText(spell["default"]),
			AlwaysPrepared: truth(spell["alwaysPrepared"]), Picked: picked,
		}
		if text(spell["free"]) != "" && count == 1 {
			choice.Free = text(spell["free"])
		}
		if spell["levelReplacements"] != nil {
			choice.LevelReplacements = integer(spell["levelReplacements"], 0)
			choice.AcquiredAt = max(integer(spell["atLevel"], integer(spell["level"], 1)), integer(recordByID(records, string(source.Type), source.ID)["level"], 1))
		}
		pendingChoices = append(pendingChoices, choice)
	}

	for _, current := range classes {
		subclass := recordByID(records, "subclass", current.Subclass)
		for _, spell := range objects(subclass["spells"]) {
			unlock := integer(spell["atLevel"], integer(spell["level"], 0))
			addGrantEntry(spell, character.Source{Type: character.SourceSubclass, ID: current.Subclass},
				unlock <= current.Level, current.Level, nil)
		}
	}
	for _, source := range sources {
		for _, spell := range objects(source.Grants["spells"]) {
			unlock := integer(spell["atLevel"], integer(spell["level"], 0))
			addGrantEntry(spell, source.Source, unlock <= source.Level, source.Level, source.Grants)
		}
	}
	for _, feat := range feats {
		target := text(object(feat["grants"])["prepareSpellListOf"])
		if target == "" {
			continue
		}
		for _, sourceFeat := range feats {
			if text(sourceFeat["category"]) != target {
				continue
			}
			source := character.Source{Type: character.SourceFeat, ID: text(sourceFeat["id"])}
			for _, reference := range stringsOf(object(sourceFeat["grants"])["spellList"]) {
				addGrant(reference, source, spellGrantOptions{alwaysPrepared: true, castingAbility: castingAbilityFor(source, nil)})
			}
		}
	}
	sheet.Spellcasting.Granted = granted
	sheet.Spellcasting.PendingChoices = pendingChoices
	sheet.Spellcasting.CastingAbilityChoices = castingChoices
}

// spellFilterOf reads a spell choice's from filter. A single casting time is
// read as a list of one.
func spellFilterOf(value any) character.SpellFilter {
	from := object(value)
	times := stringsOf(from["castingTime"])
	if single := text(from["castingTime"]); single != "" {
		times = []string{single}
	}
	return character.SpellFilter{
		Class: stringsOf(from["class"]), School: stringsOf(from["school"]), IDs: stringsOf(from["ids"]),
		CastingTime: times, Ritual: truth(from["ritual"]),
	}
}

func optionalInteger(source Object, key string) *int {
	if source[key] == nil {
		return nil
	}
	value := integer(source[key], 0)
	return &value
}
