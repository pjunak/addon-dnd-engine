package rules

import (
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
	"sort"
	"time"
)

func characterGrantActive(grant character.Grant, input character.Inputs) bool {
	if !grant.Active || grant.EffectiveLevel > len(input.Build.Levels) {
		return false
	}
	if grant.ExpiresAt != "" {
		expiry, err := time.Parse(time.RFC3339, grant.ExpiresAt)
		at, atErr := time.Parse(time.RFC3339, input.Play.AsOf)
		if err != nil || atErr != nil || !at.Before(expiry) {
			return false
		}
	}
	if grant.Condition == "always" {
		return true
	}
	for _, item := range input.Play.Inventory {
		if item.ID == grant.ItemID && item.Quantity > 0 {
			return grant.Condition == "equipped" && item.Location == "equipped" || grant.Condition == "attuned" && item.Attuned
		}
	}
	return false
}

func activeCharacterGrants(input character.Inputs) []character.Grant {
	grants := []character.Grant{}
	for _, grant := range input.Grants {
		if characterGrantActive(grant, input) {
			grants = append(grants, grant)
		}
	}
	sort.Slice(grants, func(i, j int) bool { return grants[i].ID < grants[j].ID })
	return grants
}

func effectNumber(base int, target, key string, sources []characterEffectSource, result *character.Result) int {
	value, addition, minimum, maximum := base, 0, -1_000_000, 1_000_000
	setBy := ""
	for _, grant := range sources {
		if !grant.Active {
			continue
		}
		for _, effect := range grant.Effects {
			if effect.Target != target || effect.Key != key {
				continue
			}
			switch effect.Mode {
			case "add":
				addition += effect.Value
			case "set":
				if setBy != "" {
					addCharacterIssue(result, "effect-conflict:"+target+":"+key, target, "Conflicting DM replacement values: "+setBy+" and "+grant.Name, "blocker", nil)
				}
				setBy = grant.Name
				value = effect.Value
			case "minimum":
				minimum = max(minimum, effect.Value)
			case "maximum":
				maximum = min(maximum, effect.Value)
			default:
				addCharacterIssue(result, "effect-mode:"+grant.ID, grant.ID, "Unsupported DM effect mode.", "blocker", nil)
			}
		}
	}
	if minimum > maximum {
		addCharacterIssue(result, "effect-bounds:"+target+":"+key, target, "DM effect minimum exceeds maximum.", "blocker", nil)
	}
	return min(maximum, max(minimum, value+addition))
}

func applyCharacterAbilityEffects(input character.Inputs, decisions Object, result *character.Result, profile Ruleset, records Records) {
	sources := characterEffectSources(input, records)
	caps, scores := Object{}, Object{}
	for _, ability := range Abilities {
		base := integer(object(decisions["baseStats"])[ability], 0)
		cap, bonus := profile.Constants.AbilityCap, 0
		for _, grant := range objects(decisions["abilityGrants"]) {
			bonus += integer(object(grant["assign"])[ability], 0)
			if integer(object(grant["assign"])[ability], 0) != 0 {
				cap = max(cap, min(profile.Constants.AbilityCapHard, integer(grant["cap"], cap)))
			}
		}
		caps[ability] = effectNumber(cap, "abilityCap", ability, sources, result)
		score := effectNumber(min(integer(caps[ability], cap), base+bonus), "abilityScore", ability, sources, result)
		if score < 1 || integer(caps[ability], 0) < 1 {
			addCharacterIssue(result, "ability-bound:"+ability, "abilities", ability+" must have a positive score and cap.", "blocker", nil)
		}
		scores[ability] = score - bonus
	}
	decisions["baseStats"] = scores
	decisions["scoreCaps"] = caps
	for _, grant := range sources {
		if !grant.Active {
			continue
		}
		for _, effect := range grant.Effects {
			if effect.Target == "proficiency" {
				if effect.Mode != "set" || effect.Value != 1 {
					addCharacterIssue(result, "proficiency-effect:"+grant.ID, grant.ID, "Proficiency grants must set the selected proficiency to 1.", "blocker", nil)
					continue
				}
				if _, ok := SkillAbility[effect.Key]; ok {
					decisions["skillProficiencies"] = append(values(decisions["skillProficiencies"]), effect.Key)
				} else if contains(Abilities[:], effect.Key) {
					decisions["saveProficiencies"] = append(values(decisions["saveProficiencies"]), effect.Key)
				} else {
					addCharacterIssue(result, "proficiency-kind:"+grant.ID, grant.ID, "Choose a supported skill or saving throw.", "blocker", nil)
				}
			}
		}
	}
}

func applyCharacterEffects(input character.Inputs, sheet Object, result *character.Result, records Records) {
	sources := characterEffectSources(input, records)
	derived := object(sheet["derived"])
	for _, target := range []string{"armorClass", "initiative", "speed", "maxHp"} {
		derived[target] = effectNumber(integer(derived[target], 0), target, "", sources, result)
	}
	sheet["speed"] = derived["speed"]
	// Movement granted as "speed" depends on the effective walking speed,
	// including item/DM effects applied after the ordinary source calculation.
	flight := 0
	for _, activation := range objects(sheet["activations"]) {
		if !truth(activation["active"]) {
			continue
		}
		source := object(activation["source"])
		record := recordByID(records, text(source["type"]), text(source["id"]))
		for _, definition := range append(objects(record["activations"]), objects(object(record["grants"])["activations"])...) {
			if text(definition["id"]) != text(activation["id"]) {
				continue
			}
			for _, modifier := range objects(definition["modifiers"]) {
				if text(modifier["target"]) == "flySpeed" {
					value := integer(modifier["value"], 0)
					if text(modifier["value"]) == "speed" {
						value = integer(sheet["speed"], 0)
					}
					flight = max(flight, value)
				}
			}
		}
	}
	sheet["flySpeed"] = flight
	object(sheet["ac"])["value"] = derived["armorClass"]
	object(sheet["hp"])["max"] = derived["maxHp"]
	for _, ability := range Abilities {
		row := object(object(sheet["saves"])[ability])
		row["total"] = effectNumber(integer(row["total"], 0), "savingThrow", ability, sources, result)
	}
	attunement := object(sheet["attunement"])
	attunement["limit"] = effectNumber(integer(attunement["limit"], 0), "attunementLimit", "", sources, result)
	attunement["over"] = integer(attunement["count"], 0) > integer(attunement["limit"], 0)
	for _, target := range []string{"armorClass", "speed", "maxHp"} {
		if integer(derived[target], 0) < 0 {
			addCharacterIssue(result, "negative:"+target, target, "The effective value cannot be negative.", "blocker", nil)
		}
	}
	if integer(attunement["limit"], 0) < 0 {
		addCharacterIssue(result, "negative:attunement", "inventory", "Attunement capacity cannot be negative.", "blocker", nil)
	}
	senses := object(sheet["senses"])
	if senses == nil {
		senses = Object{}
		sheet["senses"] = senses
	}
	keys := map[string]bool{}
	for key := range senses {
		keys[key] = true
	}
	for _, grant := range sources {
		if !grant.Active {
			continue
		}
		for _, effect := range grant.Effects {
			if effect.Target == "sense" {
				keys[effect.Key] = true
			}
		}
	}
	for key := range keys {
		senses[key] = effectNumber(integer(senses[key], 0), "sense", key, sources, result)
		if integer(senses[key], 0) < 0 {
			addCharacterIssue(result, "negative:sense:"+key, "senses", "A sense range cannot be negative.", "blocker", nil)
		}
	}
	resourceKeys := map[string]bool{}
	for _, resource := range objects(sheet["resources"]) {
		key := text(resource["key"])
		resourceKeys[key] = true
		resource["max"] = effectNumber(integer(resource["max"], 0), "resourceMax", key, sources, result)
		resource["spent"] = input.Play.ResourceUses[key]
		resource["remaining"] = integer(resource["max"], 0) - input.Play.ResourceUses[key]
		if integer(resource["max"], 0) < 0 {
			addCharacterIssue(result, "negative:resource:"+key, "resources", "Resource capacity cannot be negative.", "blocker", nil)
		}
	}
	for _, source := range sources {
		if !source.Active {
			continue
		}
		for _, effect := range source.Effects {
			if effect.Target == "resourceMax" && !resourceKeys[effect.Key] {
				addCharacterIssue(result, "resource-effect:"+source.ID+":"+effect.Key, "resources", "This capacity adjustment refers to a resource the character does not have.", "blocker", source.Reference)
			}
		}
	}
}

func applyCharacterHitDice(input character.Inputs, sheet Object, records Records, result *character.Result, profile Ruleset) {
	hp := object(sheet["hp"])
	old := object(hp["breakdown"])
	con := integer(object(object(sheet["abilities"])["CON"])["mod"], 0)
	bonus := integer(old["miscPerLevel"], 0)
	rows := []any{}
	maximum := 0
	minimum := profile.Constants.Character.MinimumHPGain
	for index, level := range input.Build.Levels {
		record := recordByID(records, "class", level.ClassID)
		size := hitDieSize(text(record["hitDie"]))
		rolled := size/2 + 1
		method := "average"
		if index == 0 {
			rolled = size
			method = "first level maximum"
		} else if level.HitPoints != nil {
			rolled = *level.HitPoints
			method = "recorded roll"
			if rolled < 1 || rolled > size {
				addCharacterIssue(result, "hit-die:"+level.ID, level.ID, fmt.Sprintf("Recorded hit die must be between 1 and %d.", size), "blocker", &character.Reference{Kind: "class", ID: level.ClassID})
			}
		}
		gained := max(minimum, rolled+con) + bonus
		maximum += gained
		rows = append(rows, Object{"levelId": level.ID, "classId": level.ClassID, "die": size, "result": rolled, "method": method, "constitution": con, "minimum": minimum, "bonus": bonus, "gained": gained})
	}
	hp["levels"] = rows
	hp["max"] = maximum
	object(sheet["derived"])["maxHp"] = maximum
}

func appendEffectExplanations(input character.Inputs, result *character.Result, records Records) {
	for _, grant := range characterEffectSources(input, records) {
		for _, effect := range grant.Effects {
			path := "derived." + effect.Target
			switch effect.Target {
			case "abilityScore", "abilityCap":
				path = "abilities." + effect.Key + ".score"
			case "attunementLimit":
				path = "attunement.limit"
			case "sense":
				path = "senses." + effect.Key
			case "savingThrow":
				path = "saves." + effect.Key + ".total"
			case "resourceMax":
				path = "resources." + effect.Key + ".max"
			case "proficiency":
				if contains(Abilities[:], effect.Key) {
					path = "saves." + effect.Key + ".total"
				} else {
					path = "skills." + effect.Key + ".total"
				}
			}
			explanation, exists := result.Explanations[path]
			if !exists {
				continue
			}
			status := "inactive"
			if grant.Active {
				status = "applied"
			}
			name := grant.Name
			if grant.GrantID != "" {
				name = "DM given: " + name
			}
			explanation.Terms = append(explanation.Terms, character.Term{Label: name + " (" + effect.Mode + ")", Value: effect.Value, GrantID: grant.GrantID, Source: grant.Reference, Status: status})
			result.Explanations[path] = explanation
		}
	}
}
