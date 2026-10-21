package rules

import (
	"fmt"
	"sort"
	"time"

	"github.com/pjunak/addon-dnd-engine/character"
)

// effectKey is what an effect's key names.
type effectKey int

const (
	keyNone effectKey = iota
	keyAbility
	keyAny
)

// effectTargetSpec is how one effect target is keyed and which explanation
// path it adjusts.
type effectTargetSpec struct {
	key  effectKey
	path func(key string) string
}

func fixedPath(path string) func(string) string { return func(string) string { return path } }

// effectTargets is the only dispatcher of effect targets.
var effectTargets = map[character.EffectTarget]effectTargetSpec{
	character.TargetAbilityScore:    {keyAbility, func(key string) string { return "abilities." + key + ".score" }},
	character.TargetAbilityCap:      {keyAbility, func(key string) string { return "abilities." + key + ".score" }},
	character.TargetSavingThrow:     {keyAbility, func(key string) string { return "saves." + key + ".total" }},
	character.TargetArmorClass:      {keyNone, fixedPath("derived.armorClass")},
	character.TargetInitiative:      {keyNone, fixedPath("derived.initiative")},
	character.TargetSpeed:           {keyNone, fixedPath("derived.speed")},
	character.TargetMaxHP:           {keyNone, fixedPath("derived.maxHp")},
	character.TargetAttunementLimit: {keyNone, fixedPath("attunement.limit")},
	character.TargetSense:           {keyAny, func(key string) string { return "senses." + key }},
	character.TargetResourceMax:     {keyAny, func(key string) string { return "resources." + key + ".max" }},
	character.TargetProficiency: {keyAny, func(key string) string {
		if contains(Abilities[:], key) {
			return "saves." + key + ".total"
		}
		return "skills." + key + ".total"
	}},
}

func validCharacterEffect(effect character.Effect) bool {
	spec, known := effectTargets[effect.Target]
	if !known || !effect.Mode.Valid() || effect.Value < -100000 || effect.Value > 100000 {
		return false
	}
	switch spec.key {
	case keyAbility:
		return contains(Abilities[:], effect.Key)
	case keyNone:
		return effect.Key == ""
	case keyAny:
		return effect.Key != ""
	}
	return false
}

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
	switch grant.Condition {
	case character.ConditionAlways:
		return true
	case character.ConditionEquipped, character.ConditionAttuned:
		for _, item := range input.Play.Inventory {
			if item.ID == grant.ItemID && item.Quantity > 0 {
				if characterItemSuspended(input, item.ID) {
					return false
				}
				if grant.Condition == character.ConditionEquipped {
					return item.Location == "equipped"
				}
				return item.Attuned
			}
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

func effectNumber(base int, target character.EffectTarget, key string, sources []characterEffectSource, result *character.Result) int {
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
			case character.ModeAdd:
				addition += effect.Value
			case character.ModeSet:
				if setBy != "" {
					addCharacterIssue(result, "effect-conflict:"+string(target)+":"+key, string(target), "Conflicting DM replacement values: "+setBy+" and "+grant.Name, character.SeverityBlocker, nil)
				}
				setBy = grant.Name
				value = effect.Value
			case character.ModeMinimum:
				minimum = max(minimum, effect.Value)
			case character.ModeMaximum:
				maximum = min(maximum, effect.Value)
			default:
				addCharacterIssue(result, "effect-mode:"+grant.ID, grant.ID, "Unsupported DM effect mode.", character.SeverityBlocker, nil)
			}
		}
	}
	if minimum > maximum {
		addCharacterIssue(result, "effect-bounds:"+string(target)+":"+key, string(target), "DM effect minimum exceeds maximum.", character.SeverityBlocker, nil)
	}
	return min(maximum, max(minimum, value+addition))
}

func abilityScoreFloor(sources []characterEffectSource, ability string) int {
	floor := 0
	for _, grant := range sources {
		if !grant.Active {
			continue
		}
		for _, effect := range grant.Effects {
			if effect.Target == character.TargetAbilityScore && effect.Key == ability && (effect.Mode == character.ModeSet || effect.Mode == character.ModeMinimum) {
				floor = max(floor, effect.Value)
			}
		}
	}
	return floor
}

// abilityGrantTotal sums one ability's score grants and the cap they allow:
// the normal cap, raised by any contributing grant that names a higher one, up
// to the hard limit.
func abilityGrantTotal(grants []Object, ability string, ruleset Ruleset) (bonus, cap int) {
	cap = ruleset.Constants.AbilityCap
	for _, grant := range grants {
		amount := integer(object(grant["assign"])[ability], 0)
		if amount == 0 {
			continue
		}
		bonus += amount
		cap = max(cap, min(ruleset.Constants.AbilityCapHard, integer(grant["cap"], cap)))
	}
	return bonus, cap
}

func applyCharacterAbilityEffects(input character.Inputs, decisions Object, result *character.Result, profile Ruleset, records Records) {
	sources := characterEffectSources(input, records)
	caps, scores := Object{}, Object{}
	grants := objects(decisions["abilityGrants"])
	for _, ability := range Abilities {
		base := integer(object(decisions["baseStats"])[ability], 0)
		bonus, cap := abilityGrantTotal(grants, ability, profile)
		caps[ability] = effectNumber(cap, character.TargetAbilityCap, ability, sources, result)
		score := effectNumber(min(integer(caps[ability], cap), base+bonus), character.TargetAbilityScore, ability, sources, result)
		// A "set" or "minimum" score (a Giant Strength belt) replaces the score
		// rather than adding to it, so it lifts the cap up to the hard limit.
		if floor := abilityScoreFloor(sources, ability); floor > integer(caps[ability], 0) {
			caps[ability] = min(profile.Constants.AbilityCapHard, floor)
		}
		if score < 1 || integer(caps[ability], 0) < 1 {
			addCharacterIssue(result, "ability-bound:"+ability, "abilities", ability+" must have a positive score and cap.", character.SeverityBlocker, nil)
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
			if effect.Target != character.TargetProficiency {
				continue
			}
			if effect.Mode != character.ModeSet || effect.Value != 1 {
				addCharacterIssue(result, "proficiency-effect:"+grant.ID, grant.ID, "Proficiency grants must set the selected proficiency to 1.", character.SeverityBlocker, nil)
				continue
			}
			if _, ok := SkillAbility[effect.Key]; ok {
				decisions["skillProficiencies"] = append(values(decisions["skillProficiencies"]), effect.Key)
			} else if contains(Abilities[:], effect.Key) {
				decisions["saveProficiencies"] = append(values(decisions["saveProficiencies"]), effect.Key)
			} else {
				addCharacterIssue(result, "proficiency-kind:"+grant.ID, grant.ID, "Choose a supported skill or saving throw.", character.SeverityBlocker, nil)
			}
		}
	}
}

func applyCharacterEffects(input character.Inputs, sheet *character.Calculation, result *character.Result, records Records) {
	sources := characterEffectSources(input, records)
	derived := &sheet.Derived
	derived.ArmorClass = effectNumber(derived.ArmorClass, character.TargetArmorClass, "", sources, result)
	derived.Initiative = effectNumber(derived.Initiative, character.TargetInitiative, "", sources, result)
	derived.Speed = effectNumber(derived.Speed, character.TargetSpeed, "", sources, result)
	derived.MaxHP = effectNumber(derived.MaxHP, character.TargetMaxHP, "", sources, result)
	sheet.Speed = derived.Speed
	// Movement granted as "speed" depends on the effective walking speed,
	// including item/DM effects applied after the ordinary source calculation.
	applyMovement(sheet)
	sheet.AC.Value = derived.ArmorClass
	sheet.HP.Max = derived.MaxHP
	for _, ability := range Abilities {
		row := sheet.Saves[ability]
		row.Total = effectNumber(row.Total, character.TargetSavingThrow, ability, sources, result)
		sheet.Saves[ability] = row
	}
	attunement := &sheet.Attunement
	attunement.Limit = effectNumber(attunement.Limit, character.TargetAttunementLimit, "", sources, result)
	attunement.Over = attunement.Count > attunement.Limit
	for _, effective := range []struct {
		target string
		value  int
	}{{"armorClass", derived.ArmorClass}, {"speed", derived.Speed}, {"maxHp", derived.MaxHP}} {
		if effective.value < 0 {
			addCharacterIssue(result, "negative:"+effective.target, effective.target, "The effective value cannot be negative.", character.SeverityBlocker, nil)
		}
	}
	if attunement.Limit < 0 {
		addCharacterIssue(result, "negative:attunement", "inventory", "Attunement capacity cannot be negative.", character.SeverityBlocker, nil)
	}
	if sheet.Senses == nil {
		sheet.Senses = map[string]int{}
	}
	keys := map[string]bool{}
	for key := range sheet.Senses {
		keys[key] = true
	}
	for _, grant := range sources {
		if !grant.Active {
			continue
		}
		for _, effect := range grant.Effects {
			if effect.Target == character.TargetSense {
				keys[effect.Key] = true
			}
		}
	}
	for key := range keys {
		sheet.Senses[key] = effectNumber(sheet.Senses[key], character.TargetSense, key, sources, result)
		if sheet.Senses[key] < 0 {
			addCharacterIssue(result, "negative:sense:"+key, "senses", "A sense range cannot be negative.", character.SeverityBlocker, nil)
		}
	}
	resourceKeys := map[string]bool{}
	for index := range sheet.Resources {
		resource := &sheet.Resources[index]
		resourceKeys[resource.Key] = true
		resource.Max = effectNumber(resource.Max, character.TargetResourceMax, resource.Key, sources, result)
		resource.Spent = input.Play.ResourceUses[resource.Key]
		resource.Remaining = resource.Max - input.Play.ResourceUses[resource.Key]
		if resource.Max < 0 {
			addCharacterIssue(result, "negative:resource:"+resource.Key, "resources", "Resource capacity cannot be negative.", character.SeverityBlocker, nil)
		}
	}
	for _, source := range sources {
		if !source.Active {
			continue
		}
		for _, effect := range source.Effects {
			if effect.Target == character.TargetResourceMax && !resourceKeys[effect.Key] {
				addCharacterIssue(result, "resource-effect:"+source.ID+":"+effect.Key, "resources", "This capacity adjustment refers to a resource the character does not have.", character.SeverityBlocker, source.Reference)
			}
		}
	}
}

func applyCharacterHitDice(input character.Inputs, sheet *character.Calculation, records Records, result *character.Result, profile Ruleset) {
	con := sheet.Abilities["CON"].Mod
	bonus := sheet.HP.Breakdown.MiscPerLevel
	rows := []character.HPLevel{}
	maximum := sheet.HP.Breakdown.FixedBonus
	minimum := profile.Constants.Character.MinimumHPGain
	for index, level := range input.Build.Levels {
		record := recordByID(records, "class", level.ClassID)
		size := hitDieSize(text(record["hitDie"]))
		gained, rolled, method := profile.LevelHitPoints(index+1, size, level.HitPoints, con, bonus)
		if method == "recorded roll" && (rolled < 1 || rolled > size) {
			addCharacterIssue(result, "hit-die:"+level.ID, level.ID, fmt.Sprintf("Recorded hit die must be between 1 and %d.", size), character.SeverityBlocker, &character.Reference{Kind: "class", ID: level.ClassID})
		}
		maximum += gained
		rows = append(rows, character.HPLevel{LevelID: level.ID, ClassID: level.ClassID, Die: size, Result: rolled, Method: method, Constitution: con, Minimum: minimum, Bonus: bonus, Gained: gained})
	}
	sheet.HP.Levels = rows
	sheet.HP.Max = maximum
	sheet.Derived.MaxHP = maximum
}

func appendEffectExplanations(input character.Inputs, result *character.Result, records Records) {
	for _, grant := range characterEffectSources(input, records) {
		for _, effect := range grant.Effects {
			spec, known := effectTargets[effect.Target]
			if !known {
				continue
			}
			path := spec.path(effect.Key)
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
			explanation.Terms = append(explanation.Terms, character.Term{Label: name + " (" + string(effect.Mode) + ")", Value: effect.Value, GrantID: grant.GrantID, Source: grant.Reference, Status: status})
			if grant.Reference != nil {
				explanation.Sources = uniqueCharacterReferences(append(explanation.Sources, *grant.Reference))
			}
			result.Explanations[path] = explanation
		}
	}
}
