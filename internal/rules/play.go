package rules

import (
	"fmt"
	"strconv"
	"strings"
)

// ApplyPlayChange returns a detached decision draft. The caller owns review and
// persistence; neither an invalid choice nor a calculation mutates saved data.
func ApplyPlayChange(decisions, change Object, records Records, profile Ruleset) (Object, error) {
	if err := validatePlayChange(change); err != nil {
		return nil, err
	}
	next := NormalizeBuilderDecisions(decisions, records, profile)
	sheet := Hydrate(next, records, &profile).Sheet
	if integer(object(sheet["derived"])["maxHp"], 0) <= 0 && integer(decisions["maxHp"], 0) > 0 {
		return nil, fmt.Errorf("Choose a valid class in Builder before calculating play changes. Saved values have been kept.")
	}
	var err error
	switch text(change["operation"]) {
	case "rest":
		applyRest(next, sheet, text(change["rest"]))
	case "spend-hit-die":
		resource := findPlayResource(sheet, text(change["key"]))
		if text(resource["kind"]) != "hitdice" {
			return nil, fmt.Errorf("Choose an available hit die.")
		}
		if err = spendResource(next, resource); err == nil {
			healing := max(1, HitDieAverage(text(resource["die"]))+integer(object(object(sheet["abilities"])["CON"])["mod"], 0))
			next["hp"] = min(playMaximumHP(next, sheet), max(0, integer(next["hp"], 0))+healing)
		}
	case "toggle-feature":
		var selected Object
		for _, activation := range objects(sheet["activations"]) {
			if text(activation["key"]) == text(change["key"]) {
				selected = activation
				break
			}
		}
		if selected == nil || truth(change["enabled"]) && !truth(selected["available"]) {
			return nil, fmt.Errorf("This feature is unavailable with the current equipment.")
		}
		active := playMap(next, "activeFeatures")
		delete(active, text(change["key"]))
		if truth(change["enabled"]) {
			if group := text(selected["exclusiveGroup"]); group != "" {
				for _, activation := range objects(sheet["activations"]) {
					if text(activation["exclusiveGroup"]) == group {
						delete(active, text(activation["key"]))
					}
				}
			}
			active[text(change["key"])] = true
		}
	case "select-spell":
		err = selectPlaySpell(next, sheet, change, records)
	case "cast-spell":
		err = castPlaySpell(next, sheet, change, records)
	}
	if err != nil {
		return nil, err
	}
	return next, nil
}

func validatePlayChange(change Object) error {
	fields := map[string][]string{
		"rest": {"rest"}, "spend-hit-die": {"key"}, "toggle-feature": {"key", "enabled"},
		"select-spell": {"classId", "ref", "selection", "selected"}, "cast-spell": {"classId", "ref", "slot"},
	}
	allowed, known := fields[text(change["operation"])]
	if !known || len(change) != len(allowed)+1 {
		return fmt.Errorf("The play change has invalid fields.")
	}
	for _, key := range allowed {
		value, exists := change[key]
		if !exists {
			return fmt.Errorf("The play change is missing %s.", key)
		}
		if key == "enabled" || key == "selected" {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be a boolean.", key)
			}
		} else if value, ok := value.(string); !ok || len(value) > 200 || value == "" && key != "slot" {
			return fmt.Errorf("%s must be a valid string.", key)
		}
	}
	if text(change["operation"]) == "rest" && !contains([]string{"short", "long"}, text(change["rest"])) {
		return fmt.Errorf("Choose a short or long rest.")
	}
	if text(change["operation"]) == "select-spell" && !contains([]string{"cantrips", "spellbook", "preparedSpells"}, text(change["selection"])) {
		return fmt.Errorf("Choose a spell selection list.")
	}
	return nil
}

func playMap(decisions Object, key string) Object {
	value := object(decisions[key])
	if value == nil {
		value = Object{}
		decisions[key] = value
	}
	return value
}

func findPlayResource(sheet Object, key string) Object {
	for _, resource := range objects(sheet["resources"]) {
		if text(resource["key"]) == key {
			return resource
		}
	}
	return nil
}

func remainingResource(decisions, resource Object) int {
	maximum := max(0, integer(resource["max"], 0))
	return min(maximum, max(0, integer(object(decisions["resourceUses"])[text(resource["key"])], maximum)))
}

func spendResource(decisions, resource Object) error {
	remaining := remainingResource(decisions, resource)
	if resource == nil || remaining < 1 {
		return fmt.Errorf("No uses remain for this resource.")
	}
	playMap(decisions, "resourceUses")[text(resource["key"])] = remaining - 1
	return nil
}

func playMaximumHP(decisions, sheet Object) int {
	return max(0, integer(object(decisions["overrides"])["maxHp"], integer(object(sheet["derived"])["maxHp"], integer(decisions["maxHp"], 0))))
}

func applyRest(decisions, sheet Object, rest string) {
	uses := playMap(decisions, "resourceUses")
	// Older profiles recover half the total level across all hit-die pools,
	// rather than granting that allowance separately to every multiclass die.
	hitDiceBudget := max(1, integer(sheet["totalLevel"], 1)/2)
	for _, resource := range objects(sheet["resources"]) {
		maximum := max(0, integer(resource["max"], 0))
		current := remainingResource(decisions, resource)
		for _, recharge := range objects(resource["recharge"]) {
			on := text(recharge["on"])
			if on != rest && on != "shortOrLong" && !(rest == "long" && on == "short") {
				continue
			}
			switch amount := recharge["amount"]; text(amount) {
			case "full":
				current = maximum
			case "halfLevel":
				recovered := min(maximum-current, max(1, integer(sheet["totalLevel"], 1)/2))
				if text(resource["kind"]) == "hitdice" {
					recovered = min(recovered, hitDiceBudget)
					hitDiceBudget -= recovered
				}
				current += recovered
			default:
				recovered := max(0, integer(amount, 0))
				if ability := text(object(amount)["abilityMod"]); ability != "" {
					recovered = max(1, integer(object(object(sheet["abilities"])[ability])["mod"], 0))
				}
				current = min(maximum, current+recovered)
			}
		}
		if current >= maximum {
			delete(uses, text(resource["key"]))
		} else {
			uses[text(resource["key"])] = current
		}
	}
	if rest == "long" {
		decisions["hp"] = playMaximumHP(decisions, sheet)
		decisions["tempHp"] = 0
		decisions["activeFeatures"] = Object{}
	}
}

func playCaster(sheet Object, classID string) Object {
	for _, caster := range objects(object(sheet["spellcasting"])["perClass"]) {
		if text(caster["classId"]) == classID {
			return caster
		}
	}
	return nil
}

func selectPlaySpell(decisions, sheet, change Object, records Records) error {
	classID, ref, selection := text(change["classId"]), text(change["ref"]), text(change["selection"])
	lists := playMap(decisions, selection)
	current := unique(stringsOf(lists[classID]))
	// Removing an obsolete reference must remain possible after a provider update.
	if !truth(change["selected"]) {
		filtered := make([]string, 0)
		for _, item := range current {
			if item != ref {
				filtered = append(filtered, item)
			}
		}
		lists[classID] = anyStrings(filtered)
		if selection == "spellbook" {
			removal := cloneObject(change)
			removal["selection"] = "preparedSpells"
			return selectPlaySpell(decisions, sheet, removal, records)
		}
		return nil
	}
	spell, caster := recordByID(records, "spell", ref), playCaster(sheet, classID)
	if spell == nil || caster == nil {
		return fmt.Errorf("Choose a spell and a spellcasting class.")
	}
	level := integer(spell["level"], 0)
	if !contains(stringsOf(spell["classes"]), text(caster["spellListClassId"])) && !contains(stringsOf(caster["expandedSpellIds"]), ref) {
		return fmt.Errorf("This spell is not on the class spell list.")
	}
	if level > integer(caster["maxSpellLevel"], 0) || (selection == "cantrips") != (level == 0) {
		return fmt.Errorf("This spell is not eligible for this selection.")
	}
	limit := integer(caster["preparedLimit"], 0)
	if selection == "cantrips" {
		limit = integer(caster["cantripsKnown"], 0)
	}
	if selection == "spellbook" {
		if text(caster["prepares"]) != "spellbook" {
			return fmt.Errorf("This class does not use a spellbook.")
		}
		// Books can contain copied spells beyond level-up additions.
		limit = 5000
	}
	if selection == "preparedSpells" && text(caster["prepares"]) == "spellbook" && !contains(stringsOf(object(decisions["spellbook"])[classID]), ref) {
		return fmt.Errorf("Learn this spell in the spellbook before preparing it.")
	}
	if contains(current, ref) {
		return nil
	}
	if len(current) >= limit {
		return fmt.Errorf("The class selection limit has been reached (%d).", limit)
	}
	lists[classID] = anyStrings(append(current, ref))
	return nil
}

func castPlaySpell(decisions, sheet, change Object, records Records) error {
	classID, ref, slot := text(change["classId"]), text(change["ref"]), text(change["slot"])
	spell, caster := recordByID(records, "spell", ref), playCaster(sheet, classID)
	if spell == nil || caster == nil {
		return fmt.Errorf("Choose a spellcasting class and a known spell.")
	}
	level := integer(spell["level"], 0)
	selection := "preparedSpells"
	if level == 0 {
		selection = "cantrips"
	}
	if !contains(stringsOf(object(decisions[selection])[classID]), ref) {
		return fmt.Errorf("This spell is not prepared or known by this class.")
	}
	if level == 0 {
		if slot != "" {
			return fmt.Errorf("Cantrips do not spend spell slots.")
		}
		return nil
	}
	resource := findPlayResource(sheet, slot)
	slotLevel, _ := strconv.Atoi(strings.TrimPrefix(slot, "slot-"))
	if slot == "pact-slot" {
		for _, entry := range objects(object(sheet["spellcasting"])["perClass"]) {
			slotLevel = max(slotLevel, integer(object(entry["pact"])["level"], 0))
		}
	}
	if text(resource["kind"]) != "slot" || slotLevel < level {
		return fmt.Errorf("Choose a spell slot of this spell's level or higher.")
	}
	return spendResource(decisions, resource)
}
