package rules

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/pjunak/addon-dnd-engine/character"
)

// attackRule is one attack modifier from a passive grant (grants.attackModifiers)
// or an enabled activation (target "attack"), with class columns resolved.
type attackRule struct {
	rule   Object
	name   string
	source character.Source
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

func collectAttackRules(sources []grantSource, classes []resolvedClass, modifiers []activeModifier) []attackRule {
	rules := make([]attackRule, 0)
	for _, source := range sources {
		for _, rule := range objects(source.Grants["attackModifiers"]) {
			rules = append(rules, attackRule{resolveAttackColumns(rule, source, classes),
				firstText(source.Record["name"], source.Source.ID), source.Source})
		}
	}
	for _, modifier := range modifiers {
		if modifier.target() == "attack" {
			rules = append(rules, attackRule{modifier.fields, modifier.name, modifier.source})
		}
	}
	return rules
}

// attackSubject is what an attack is made with: a weapon record, the
// ruleset's unarmed strike, or an attack a feature declares (featureID, with
// the feature's grant source).
type attackSubject struct {
	record    Object
	unarmed   bool
	featureID string
	source    *character.Source
}

// attackFilterMatches tests one match entry; every listed condition must hold.
// An unknown condition never matches, so it cannot widen a rule.
func attackFilterMatches(filter Object, subject attackSubject, ability string) bool {
	weapon, unarmed := subject.record, subject.unarmed
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
			if subject.featureID == "" || subject.featureID != text(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func attackRuleApplies(rule Object, subject attackSubject, ability string, armor armorState) bool {
	if truth(rule["inactive"]) || !armorRequirementMet(rule["requires"], armor) {
		return false
	}
	filters := objects(rule["match"])
	if rule["match"] != nil && len(filters) == 0 {
		return false
	}
	for _, filter := range filters {
		if attackFilterMatches(filter, subject, ability) {
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
func weaponDistance(value any) *character.WeaponRange {
	distance := object(value)
	normal, long := integer(distance["normal"], 0), integer(distance["long"], 0)
	if normal < 1 || long < normal {
		return nil
	}
	return &character.WeaponRange{Normal: normal, Long: long}
}

// attackRow computes one attack.
func attackRow(subject attackSubject, context attackContext) character.Attack {
	weapon, unarmed := subject.record, subject.unarmed
	feature := subject.featureID != ""
	// A feature attack's id names the declaration, not a weapon record.
	id := text(weapon["id"])
	if feature {
		id = ""
	}
	properties := stringsOf(weapon["properties"])
	proficient := unarmed || feature && weapon["proficient"] != false
	if !unarmed && !feature {
		_, explicit := context.proficiency.ids[id]
		prof := context.proficiency
		proficient = explicit || text(weapon["category"]) == "simple" && prof.simple ||
			text(weapon["category"]) == "martial" && (prof.martial ||
				prof.martialLight && contains(properties, "light") ||
				prof.martialFinesseLight && (contains(properties, "finesse") || contains(properties, "light")))
		// A Pact Weapon is proficient while its activation is on.
		for _, option := range context.weaponAbilities {
			proficient = proficient || truth(option["proficient"]) && attackRuleApplies(option, subject, "", context.armor)
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
		if proficient && attackRuleApplies(option, subject, "", context.armor) {
			choose(text(option["ability"]))
		}
	}
	// Ability options (Martial Arts) are chosen before bonuses that depend on
	// the ability used (Rage).
	for _, current := range context.rules {
		if ability := text(current.rule["ability"]); ability != "" && attackRuleApplies(current.rule, subject, "", context.armor) {
			choose(ability)
		}
	}
	die, versatile := text(weapon["damage"]), text(weapon["versatileDamage"])
	attackBonus, damageBonus := 0, 0
	applied := make([]character.AttackApplied, 0)
	for _, current := range context.rules {
		if !attackRuleApplies(current.rule, subject, abilityName, context.armor) {
			continue
		}
		row := character.AttackApplied{Name: current.name, Source: current.source}
		value := integer(current.rule["attackBonus"], 0)
		// An ability-based bonus, such as Sacred Weapon's Charisma modifier,
		// with an optional minimum.
		if ability := text(current.rule["attackBonusAbility"]); ability != "" {
			value += max(integer(current.rule["min"], context.mods[ability]), context.mods[ability])
		}
		if value != 0 {
			attackBonus += value
			row.AttackBonus = value
		}
		if value := integer(current.rule["damageBonus"], 0); value != 0 {
			damageBonus += value
			row.DamageBonus = value
		}
		if candidate := text(current.rule["damageDie"]); candidate != "" {
			die = largerDie(die, candidate)
			if versatile != "" {
				versatile = largerDie(versatile, candidate)
			}
			row.DamageDie = candidate
		}
		row.Ability = text(current.rule["ability"])
		applied = append(applied, row)
	}
	ability := context.mods[abilityName]
	attack := ability + attackBonus
	if proficient {
		attack += context.pb
	}
	var versatileDamage *string
	if versatile != "" {
		damage := withModifier(versatile, ability+damageBonus)
		versatileDamage = &damage
	}
	_, mastered := context.mastery[id]
	row := character.Attack{
		Ref: id, Name: text(weapon["name"]), AttackBonus: attack,
		Ability: abilityName, Damage: withModifier(die, ability+damageBonus), VersatileDamage: versatileDamage,
		DamageType: text(weapon["damageType"]), Properties: properties,
		Mastery: text(weapon["mastery"]), MasteryActive: mastered, Proficient: proficient,
		Unarmed: unarmed, Modifiers: applied,
	}
	if unarmed {
		row.Ref = ""
	}
	if feature {
		row.FeatureAttack, row.Source = subject.featureID, subject.source
	}
	if context.criticalRange > 1 && context.criticalRange < 20 {
		row.CriticalRange = context.criticalRange
	}
	if reach := context.reach; reach != nil && (unarmed || text(weapon["range"]) == "melee") {
		row.Reach = reach.Base
		if contains(properties, "reach") {
			row.Reach = reach.Base + reach.ReachBonus
		}
	}
	// Ranged weapons have a normal/long range; thrown melee weapons keep their
	// reach and add the thrown range.
	if distance := weaponDistance(weapon["rangeNormalLong"]); distance != nil && text(weapon["range"]) == "ranged" {
		row.Range = distance
	} else if distance := weaponDistance(weapon["thrownRange"]); distance != nil && contains(properties, "thrown") {
		distance.Thrown = true
		row.Range = distance
	}
	return row
}
