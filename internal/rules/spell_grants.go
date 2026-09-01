package rules

import (
	"fmt"
	"strings"
)

func hydrateSpellGrants(
	decisions Object,
	sheet Object,
	classes []resolvedClass,
	feats []Object,
	sources []grantSource,
	records Records,
) {
	spellcasting := object(sheet["spellcasting"])
	granted := make([]any, 0)
	pendingChoices := make([]any, 0)
	castingChoices := make([]any, 0)
	castingKeys := make(map[string]struct{})
	grantChoices := object(decisions["grantChoices"])
	grantCastingAbilities := object(decisions["grantCastingAbilities"])
	featureChoices := object(decisions["featureChoices"])

	castingAbilityFor := func(source, sourceGrants Object) string {
		grants := sourceGrants
		if grants == nil {
			for _, candidate := range sources {
				if text(candidate.Source["type"]) == text(source["type"]) &&
					text(candidate.Source["id"]) == text(source["id"]) &&
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
				grantSource := object(grant["source"])
				if text(grantSource["type"]) != "feat" {
					continue
				}
				choiceID := text(grant["id"])
				if strings.HasSuffix(choiceID, ":featability") {
					choiceID = strings.TrimSuffix(choiceID, ":featability") + ":feat"
				}
				if text(featureChoices[choiceID]) != text(source["id"]) {
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
		key := fmt.Sprintf("%s:%s:%s", text(source["type"]), text(source["id"]),
			firstText(declaration["id"], "casting-ability"))
		selected := text(grantCastingAbilities[key])
		if !contains(options, selected) {
			selected = ""
		}
		if _, exists := castingKeys[key]; !exists {
			castingKeys[key] = struct{}{}
			castingChoices = append(castingChoices, Object{
				"key": key, "source": source, "options": anyStrings(options), "selected": nullableString(selected),
			})
		}
		return selected
	}

	addGrant := func(reference string, source, options Object) {
		if reference == "" {
			return
		}
		record := recordByID(records, "spell", reference)
		var level any
		if record != nil {
			level = integer(record["level"], 0)
		}
		granted = append(granted, Object{
			"ref": reference, "name": firstText(record["name"], reference), "level": level,
			"school": text(record["school"]), "source": source,
			"alwaysPrepared": truth(options["alwaysPrepared"]),
			"free":           nullableText(options["free"]), "castingAbility": nullableText(options["castingAbility"]),
			"castAtLevel": nullableNumber(options["castAtLevel"]),
		})
	}

	addGrantEntry := func(spell, source Object, unlocked bool, sourceLevel int, sourceGrants Object) {
		if !unlocked {
			return
		}
		castingAbility := castingAbilityFor(source, sourceGrants)
		castAtLevel := 0
		hasCastAtLevel := false
		for _, row := range objects(spell["castAtLevelByLevel"]) {
			if sourceLevel >= integer(row["level"], 0) {
				castAtLevel = integer(row["castAtLevel"], 0)
				hasCastAtLevel = true
			}
		}
		options := Object{
			"alwaysPrepared": spell["alwaysPrepared"], "free": spell["free"],
			"castingAbility": castingAbility,
		}
		if hasCastAtLevel {
			options["castAtLevel"] = castAtLevel
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
		key := fmt.Sprintf("%s:%s:%s", text(source["type"]), text(source["id"]), choiceID)
		picked := stringsOf(grantChoices[key])
		if len(picked) > count {
			picked = picked[:count]
		}
		if len(picked) == 0 && text(spell["default"]) != "" {
			picked = []string{text(spell["default"])}
		}
		for _, reference := range picked {
			addGrant(reference, source, options)
		}
		pendingChoices = append(pendingChoices, Object{
			"key": key, "source": source, "choose": count,
			"spellLevel":    optionalInteger(spell, "spellLevel"),
			"maxSpellLevel": optionalInteger(spell, "maxSpellLevel"),
			"from":          objectOrEmpty(spell["from"]), "default": nullableText(spell["default"]),
			"alwaysPrepared": truth(spell["alwaysPrepared"]), "picked": anyStrings(picked),
		})
	}

	for _, current := range classes {
		subclass := recordByID(records, "subclass", current.Subclass)
		for _, spell := range objects(subclass["spells"]) {
			unlock := integer(spell["atLevel"], integer(spell["level"], 0))
			addGrantEntry(spell, Object{"type": "subclass", "id": current.Subclass},
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
			source := Object{"type": "feat", "id": text(sourceFeat["id"])}
			for _, reference := range stringsOf(object(sourceFeat["grants"])["spellList"]) {
				addGrant(reference, source, Object{
					"alwaysPrepared": true, "castingAbility": castingAbilityFor(source, nil),
				})
			}
		}
	}

	spellcasting["granted"] = granted
	spellcasting["pendingChoices"] = pendingChoices
	spellcasting["castingAbilityChoices"] = castingChoices
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableNumber(value any) any {
	if value == nil {
		return nil
	}
	return number(value, 0)
}

func optionalInteger(source Object, key string) any {
	if source[key] == nil {
		return nil
	}
	return integer(source[key], 0)
}

func objectOrEmpty(value any) Object {
	if result := object(value); result != nil {
		return result
	}
	return Object{}
}
