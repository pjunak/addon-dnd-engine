package rules

import (
	"fmt"
	"regexp"
	"strconv"
)

// attackRule is one attack modifier from a passive grant (grants.attackModifiers)
// or an enabled activation (target "attack"), with class columns resolved.
type attackRule struct {
	rule   Object
	name   string
	source Object
}

// resolveAttackColumns replaces damageBonusColumn and damageDieColumn with the
// granting class's table value, so a multiclass character reads its own level.
func resolveAttackColumns(rule Object, source grantSource, classes []resolvedClass) Object {
	resolved := cloneObject(rule)
	for field, target := range map[string]string{"damageBonusColumn": "damageBonus", "damageDieColumn": "damageDie"} {
		column := text(rule[field])
		if column == "" {
			continue
		}
		delete(resolved, field)
		if value, found := sourceColumnValue(source, classes, column); found {
			resolved[target] = value
		} else {
			resolved["inactive"] = true
		}
	}
	return resolved
}

func collectAttackRules(sources []grantSource, classes []resolvedClass, modifiers []Object) []attackRule {
	rules := make([]attackRule, 0)
	for _, source := range sources {
		for _, rule := range objects(source.Grants["attackModifiers"]) {
			rules = append(rules, attackRule{resolveAttackColumns(rule, source, classes),
				firstText(source.Record["name"], source.Source["id"]), source.Source})
		}
	}
	for _, modifier := range modifiers {
		if text(modifier["target"]) == "attack" {
			rules = append(rules, attackRule{modifier, text(modifier["sourceName"]), object(modifier["source"])})
		}
	}
	return rules
}

// attackFilterMatches tests one match entry; every listed condition must hold.
// An unknown condition never matches, so it cannot widen a rule.
func attackFilterMatches(filter, weapon Object, unarmed bool, ability string) bool {
	for key, value := range filter {
		switch key {
		case "unarmed":
			if truth(value) != unarmed {
				return false
			}
		case "range":
			current := text(weapon["range"])
			if unarmed {
				current = "melee"
			}
			if current != text(value) {
				return false
			}
		case "category":
			if unarmed || text(weapon["category"]) != text(value) {
				return false
			}
		case "property":
			if unarmed || !contains(stringsOf(weapon["properties"]), text(value)) {
				return false
			}
		case "attackAbility":
			if ability == "" || ability != text(value) {
				return false
			}
		case "attack":
			// A feature attack by its declared id, such as an Armorer's weapon.
			if text(weapon["featureAttack"]) == "" || text(weapon["featureAttack"]) != text(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func attackRuleApplies(rule, weapon Object, unarmed bool, ability string, armor armorState) bool {
	if truth(rule["inactive"]) || !armorRequirementMet(rule["requires"], armor) {
		return false
	}
	filters := objects(rule["match"])
	if rule["match"] != nil && len(filters) == 0 {
		return false
	}
	for _, filter := range filters {
		if attackFilterMatches(filter, weapon, unarmed, ability) {
			return true
		}
	}
	return len(filters) == 0
}

var diePattern = regexp.MustCompile(`^(\d+)d(\d+)$`)

// averageDamage compares dice such as "1d8" with flat damage such as "1".
func averageDamage(value string) float64 {
	if match := diePattern.FindStringSubmatch(value); match != nil {
		count, _ := strconv.Atoi(match[1])
		sides, _ := strconv.Atoi(match[2])
		return float64(count) * float64(sides+1) / 2
	}
	flat, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return flat
}

func largerDie(current, candidate string) string {
	if candidate != "" && averageDamage(candidate) > averageDamage(current) {
		return candidate
	}
	return current
}

// withModifier writes dice as "1d8 +3"; flat damage (an unarmed strike's 1)
// is shown as its total.
func withModifier(damage string, modifier int) string {
	if flat, err := strconv.Atoi(damage); err == nil {
		return strconv.Itoa(flat + modifier)
	}
	if modifier == 0 {
		return damage
	}
	return fmt.Sprintf("%s %+d", damage, modifier)
}

type attackContext struct {
	mods        map[string]int
	pb          int
	proficiency weaponProf
	mastery     map[string]struct{}
	// weaponAbilities are active weaponAbility modifiers, such as Bladesong's
	// Intelligence or a Pact Weapon's Charisma.
	weaponAbilities []Object
	rules           []attackRule
	armor           armorState
	reach           *MeleeReach
	// criticalRange is the lowest d20 roll that scores a Critical Hit, when
	// a grant lowers it below 20.
	criticalRange int
}

// weaponDistance reads a record's {normal, long} range in the ruleset's
// distance unit; anything else is no range.
func weaponDistance(value any) (Object, bool) {
	distance := object(value)
	normal, long := integer(distance["normal"], 0), integer(distance["long"], 0)
	if normal < 1 || long < normal {
		return nil, false
	}
	return Object{"normal": normal, "long": long}, true
}

// attackRow computes one attack. weapon is the weapon record, or the ruleset's
// unarmed strike description when unarmed is true.
func attackRow(weapon Object, unarmed bool, context attackContext) Object {
	properties := stringsOf(weapon["properties"])
	proficient := unarmed || text(weapon["featureAttack"]) != "" && weapon["proficient"] != false
	if !unarmed && text(weapon["featureAttack"]) == "" {
		_, explicit := context.proficiency.ids[text(weapon["id"])]
		prof := context.proficiency
		proficient = explicit || text(weapon["category"]) == "simple" && prof.simple ||
			text(weapon["category"]) == "martial" && (prof.martial ||
				prof.martialLight && contains(properties, "light") ||
				prof.martialFinesseLight && (contains(properties, "finesse") || contains(properties, "light")))
		// A Pact Weapon is proficient while its activation is on.
		for _, option := range context.weaponAbilities {
			proficient = proficient || truth(option["proficient"]) && attackRuleApplies(option, weapon, unarmed, "", context.armor)
		}
	}
	abilityName := "STR"
	if !unarmed && text(weapon["range"]) == "ranged" {
		abilityName = "DEX"
	}
	choose := func(candidate string) {
		if _, known := context.mods[candidate]; known && context.mods[candidate] > context.mods[abilityName] {
			abilityName = candidate
		}
	}
	if contains(properties, "finesse") {
		choose("DEX")
	}
	// A feature attack may name an ability it can use instead, such as an
	// Armorer's Intelligence.
	if ability := text(weapon["ability"]); ability != "" {
		choose(ability)
	}
	for _, option := range context.weaponAbilities {
		if proficient && attackRuleApplies(option, weapon, unarmed, "", context.armor) {
			choose(text(option["ability"]))
		}
	}
	// Ability options (Martial Arts) are chosen before bonuses that depend on
	// the ability used (Rage).
	for _, current := range context.rules {
		if ability := text(current.rule["ability"]); ability != "" && attackRuleApplies(current.rule, weapon, unarmed, "", context.armor) {
			choose(ability)
		}
	}
	die, versatile := text(weapon["damage"]), text(weapon["versatileDamage"])
	attackBonus, damageBonus := 0, 0
	applied := make([]any, 0)
	for _, current := range context.rules {
		if !attackRuleApplies(current.rule, weapon, unarmed, abilityName, context.armor) {
			continue
		}
		row := Object{"name": current.name, "source": current.source}
		value := integer(current.rule["attackBonus"], 0)
		// An ability-based bonus, such as Sacred Weapon's Charisma modifier,
		// with an optional minimum.
		if ability := text(current.rule["attackBonusAbility"]); ability != "" {
			value += max(integer(current.rule["min"], context.mods[ability]), context.mods[ability])
		}
		if value != 0 {
			attackBonus += value
			row["attackBonus"] = value
		}
		if value := integer(current.rule["damageBonus"], 0); value != 0 {
			damageBonus += value
			row["damageBonus"] = value
		}
		if candidate := text(current.rule["damageDie"]); candidate != "" {
			die = largerDie(die, candidate)
			if versatile != "" {
				versatile = largerDie(versatile, candidate)
			}
			row["damageDie"] = candidate
		}
		if text(current.rule["ability"]) != "" {
			row["ability"] = current.rule["ability"]
		}
		applied = append(applied, row)
	}
	ability := context.mods[abilityName]
	attack := ability + attackBonus
	if proficient {
		attack += context.pb
	}
	damage := withModifier(die, ability+damageBonus)
	var versatileDamage any
	if versatile != "" {
		versatileDamage = withModifier(versatile, ability+damageBonus)
	}
	_, mastered := context.mastery[text(weapon["id"])]
	row := Object{
		"ref": text(weapon["id"]), "name": text(weapon["name"]), "attackBonus": attack,
		"ability": abilityName, "damage": damage, "versatileDamage": versatileDamage,
		"damageType": text(weapon["damageType"]), "properties": anyStrings(properties),
		"mastery": text(weapon["mastery"]), "masteryActive": mastered, "proficient": proficient,
	}
	if unarmed {
		row["unarmed"] = true
		delete(row, "ref")
	}
	if id := text(weapon["featureAttack"]); id != "" {
		row["featureAttack"], row["source"] = id, weapon["source"]
		delete(row, "ref")
	}
	if context.criticalRange > 1 && context.criticalRange < 20 {
		row["criticalRange"] = context.criticalRange
	}
	if reach := context.reach; reach != nil && (unarmed || text(weapon["range"]) == "melee") {
		row["reach"] = reach.Base
		if contains(properties, "reach") {
			row["reach"] = reach.Base + reach.ReachBonus
		}
	}
	// Ranged weapons have a normal/long range; thrown melee weapons keep their
	// reach and add the thrown range.
	if distance, ok := weaponDistance(weapon["rangeNormalLong"]); ok && text(weapon["range"]) == "ranged" {
		row["range"] = distance
	} else if distance, ok := weaponDistance(weapon["thrownRange"]); ok && contains(properties, "thrown") {
		distance["thrown"] = true
		row["range"] = distance
	}
	if len(applied) > 0 {
		row["modifiers"] = applied
	}
	return row
}
