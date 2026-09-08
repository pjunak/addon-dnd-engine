package rules

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// SpellOptions describes available controls without changing the authored sheet.
// Hydration's existing shape stays stable for consumers that only need stats.
func SpellOptions(decisions, sheet Object, records Records, profile Ruleset) Object {
	casting := object(sheet["spellcasting"])
	classes, choices, grants, slots := []any{}, []any{}, []any{}, []any{}
	spells := recordList(records, "spell")
	sort.Slice(spells, func(i, j int) bool { return text(spells[i]["id"]) < text(spells[j]["id"]) })
	for _, caster := range objects(casting["perClass"]) {
		classID := text(caster["classId"])
		ids, rituals := []string{}, []string{}
		costs, castSlots := Object{}, Object{}
		for _, spell := range spells {
			ref := text(spell["id"])
			if classSpellEligible(spell, caster) {
				ids = append(ids, ref)
			}
			if canCastRitual(decisions, caster, spell) {
				rituals = append(rituals, ref)
			}
			if text(caster["prepares"]) == "spellbook" && classSpellEligible(spell, caster) && integer(spell["level"], 0) > 0 {
				costs[ref] = ScrollCopyCost(number(spell["level"], 0), profile)
			}
			if contains(stringsOf(object(decisions["preparedSpells"])[classID]), ref) || contains(stringsOf(object(decisions["cantrips"])[classID]), ref) {
				castSlots[ref] = anyStrings(spellSlotKeys(decisions, sheet, records, ref, integer(spell["level"], 0)))
			}
		}
		classes = append(classes, Object{"classId": classID, "spellIds": anyStrings(ids), "ritualIds": anyStrings(rituals), "copyCosts": costs, "castSlots": castSlots, "canSwap": text(caster["prepares"]) != "spellbook"})
	}
	for _, choice := range objects(casting["pendingChoices"]) {
		entry := cloneObjectDeep(choice)
		eligible := []string{}
		for _, spell := range spells {
			if grantSpellEligible(spell, choice) {
				eligible = append(eligible, text(spell["id"]))
			}
		}
		entry["eligibleSpellIds"] = anyStrings(eligible)
		choices = append(choices, entry)
	}
	for _, grant := range objects(casting["granted"]) {
		entry := cloneObjectDeep(grant)
		ref := text(grant["ref"])
		entry["key"] = spellGrantKey(grant)
		keys := spellSlotKeys(decisions, sheet, records, ref, integer(grant["level"], 0))
		if text(grant["free"]) != "" && findPlayResource(sheet, "charge-"+ref) != nil {
			keys = append([]string{"charge-" + ref}, keys...)
		}
		entry["slots"] = anyStrings(keys)
		grants = append(grants, entry)
	}
	for _, resource := range objects(sheet["resources"]) {
		if !contains([]string{"slot", "charge"}, text(resource["kind"])) {
			continue
		}
		slots = append(slots, Object{"key": resource["key"], "name": resource["name"], "max": resource["max"], "current": remainingResource(decisions, resource)})
	}
	return Object{"classes": classes, "pendingChoices": choices, "castingAbilityChoices": cloneValue(casting["castingAbilityChoices"]), "granted": grants, "slots": slots}
}

func classSpellEligible(spell, caster Object) bool {
	return spell != nil && caster != nil && integer(spell["level"], 0) <= integer(caster["maxSpellLevel"], 0) &&
		(contains(stringsOf(spell["classes"]), text(caster["spellListClassId"])) || contains(stringsOf(caster["expandedSpellIds"]), text(spell["id"])))
}

func grantSpellEligible(spell, choice Object) bool {
	if spell == nil || choice == nil {
		return false
	}
	level := integer(spell["level"], 0)
	if choice["spellLevel"] != nil && level != integer(choice["spellLevel"], 0) || choice["maxSpellLevel"] != nil && level > integer(choice["maxSpellLevel"], 0) {
		return false
	}
	from := object(choice["from"])
	classes, schools, ids := stringsOf(from["class"]), stringsOf(from["school"]), stringsOf(from["ids"])
	eligible := len(classes)+len(schools)+len(ids) == 0 || contains(ids, text(spell["id"]))
	for _, classID := range classes {
		eligible = eligible || contains(stringsOf(spell["classes"]), classID)
	}
	for _, school := range schools {
		eligible = eligible || strings.EqualFold(school, text(spell["school"]))
	}
	if times := stringsOf(from["castingTime"]); len(times) > 0 {
		match := false
		for _, value := range times {
			match = match || strings.EqualFold(value, text(spell["castingTime"]))
		}
		eligible = eligible && match
	}
	if time := text(from["castingTime"]); time != "" {
		eligible = eligible && strings.EqualFold(time, text(spell["castingTime"]))
	}
	return eligible
}

func selectGrantSpell(decisions, sheet, change Object, records Records) error {
	key, ref := text(change["key"]), text(change["ref"])
	var choice Object
	for _, candidate := range objects(object(sheet["spellcasting"])["pendingChoices"]) {
		if text(candidate["key"]) == key {
			choice = candidate
			break
		}
	}
	if choice == nil {
		return fmt.Errorf("This spell choice is no longer available.")
	}
	current := unique(stringsOf(choice["picked"]))
	if truth(change["selected"]) {
		if !grantSpellEligible(recordByID(records, "spell", ref), choice) {
			return fmt.Errorf("This spell does not meet the grant's requirements.")
		}
		if contains(current, ref) {
			return nil
		}
		if len(current) >= integer(choice["choose"], 0) {
			return fmt.Errorf("Remove a selected spell before choosing another.")
		}
		current = append(current, ref)
	} else {
		filtered := []string{}
		for _, item := range current {
			if item != ref {
				filtered = append(filtered, item)
			}
		}
		current = filtered
	}
	playMap(decisions, "grantChoices")[key] = anyStrings(current)
	return nil
}

func selectCastingAbility(decisions, sheet, change Object) error {
	key, ability := text(change["key"]), text(change["ability"])
	for _, choice := range objects(object(sheet["spellcasting"])["castingAbilityChoices"]) {
		if text(choice["key"]) != key {
			continue
		}
		if !contains(stringsOf(choice["options"]), ability) {
			return fmt.Errorf("Choose an available spellcasting ability.")
		}
		playMap(decisions, "grantCastingAbilities")[key] = ability
		return nil
	}
	return fmt.Errorf("This spellcasting ability choice is no longer available.")
}

func spellGrantKey(grant Object) string {
	source := object(grant["source"])
	return text(source["type"]) + ":" + text(source["id"]) + ":" + text(grant["ref"])
}

func castGrantedSpell(decisions, sheet, change Object, records Records) error {
	for _, grant := range objects(object(sheet["spellcasting"])["granted"]) {
		if spellGrantKey(grant) != text(change["key"]) {
			continue
		}
		ref, slot := text(grant["ref"]), text(change["slot"])
		if recordByID(records, "spell", ref) == nil {
			return fmt.Errorf("This granted spell is unavailable in the selected rules data.")
		}
		if slot == "charge-"+ref && text(grant["free"]) != "" {
			return spendResource(decisions, findPlayResource(sheet, slot))
		}
		return spendSpellSlot(decisions, sheet, records, ref, integer(grant["level"], 0), slot)
	}
	return fmt.Errorf("This spell is not granted by the current build.")
}

func spellSlotKeys(decisions, sheet Object, records Records, ref string, level int) []string {
	if level == 0 {
		return []string{""}
	}
	result := []string{}
	for _, resource := range objects(sheet["resources"]) {
		if text(resource["kind"]) != "slot" {
			continue
		}
		key := text(resource["key"])
		slotLevel := integer(resource["level"], 0)
		if strings.HasPrefix(key, "slot-") {
			slotLevel, _ = strconv.Atoi(strings.TrimPrefix(key, "slot-"))
		}
		if key == "pact-slot" {
			for _, caster := range objects(object(sheet["spellcasting"])["perClass"]) {
				slotLevel = max(slotLevel, integer(object(caster["pact"])["level"], 0))
			}
		}
		if slotLevel < level {
			continue
		}
		if restriction := text(resource["restriction"]); restriction != "" {
			allowed := false
			for _, feat := range selectedFeats(decisions, records) {
				if text(feat["category"]) == restriction && contains(stringsOf(object(feat["grants"])["spellList"]), ref) {
					allowed = true
				}
			}
			if !allowed {
				continue
			}
		}
		result = append(result, key)
	}
	return unique(result)
}

func spendSpellSlot(decisions, sheet Object, records Records, ref string, level int, slot string) error {
	if !contains(spellSlotKeys(decisions, sheet, records, ref, level), slot) {
		return fmt.Errorf("Choose an eligible spell slot of this spell's level or higher.")
	}
	if level == 0 {
		return nil
	}
	return spendResource(decisions, findPlayResource(sheet, slot))
}

func canCastRitual(decisions, caster, spell Object) bool {
	if caster == nil || spell == nil || !truth(caster["ritual"]) || !truth(spell["ritual"]) {
		return false
	}
	key := "preparedSpells"
	if text(caster["prepares"]) == "spellbook" {
		key = "spellbook"
	}
	return contains(stringsOf(object(decisions[key])[text(caster["classId"])]), text(spell["id"]))
}

func copyPlaySpell(decisions, sheet, change Object, records Records, profile Ruleset) error {
	classID, ref, scrollID := text(change["classId"]), text(change["ref"]), text(change["scrollId"])
	if contains(stringsOf(object(decisions["spellbook"])[classID]), ref) {
		return fmt.Errorf("This spell is already in the spellbook.")
	}
	spell := recordByID(records, "spell", ref)
	if spell == nil {
		return fmt.Errorf("Choose a spell to copy.")
	}
	cost := ScrollCopyCost(number(spell["level"], 0), profile)
	currency := playMap(decisions, "currency")
	if number(currency["gp"], 0) < cost {
		return fmt.Errorf("Copying this spell requires %g GP.", cost)
	}
	if scrollID != "" {
		found := false
		for _, item := range objects(decisions["inventory"]) {
			if text(item["id"]) != scrollID {
				continue
			}
			name := strings.ToLower(text(item["name"]))
			matches := text(item["spellRef"]) == ref || strings.Contains(name, "scroll") && text(spell["name"]) != "" && strings.Contains(name, strings.ToLower(text(spell["name"])))
			if !matches || integer(item["qty"], 0) < 1 {
				return fmt.Errorf("Choose a remaining scroll for this spell.")
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("The selected scroll is no longer in the inventory.")
		}
	}
	if err := selectPlaySpell(decisions, sheet, Object{"classId": classID, "ref": ref, "selection": "spellbook", "selected": true}, records); err != nil {
		return err
	}
	currency["gp"] = number(currency["gp"], 0) - cost
	if scrollID != "" {
		inventory := []any{}
		for _, item := range objects(decisions["inventory"]) {
			if text(item["id"]) == scrollID {
				item["qty"] = integer(item["qty"], 0) - 1
				if integer(item["qty"], 0) == 0 {
					continue
				}
			}
			inventory = append(inventory, item)
		}
		decisions["inventory"] = inventory
	}
	return nil
}

func swapPlaySpell(decisions, sheet, change Object, records Records) error {
	classID, previous, next := text(change["classId"]), text(change["out"]), text(change["ref"])
	caster := playCaster(sheet, classID)
	selected := stringsOf(object(decisions["preparedSpells"])[classID])
	if caster == nil || text(caster["prepares"]) == "spellbook" || !contains(selected, previous) || contains(selected, next) {
		return fmt.Errorf("Choose a known spell to replace and a different eligible spell.")
	}
	if err := selectPlaySpell(decisions, sheet, Object{"classId": classID, "ref": previous, "selection": "preparedSpells", "selected": false}, records); err != nil {
		return err
	}
	if err := selectPlaySpell(decisions, sheet, Object{"classId": classID, "ref": next, "selection": "preparedSpells", "selected": true}, records); err != nil {
		return err
	}
	decisions["spellSwaps"] = append(values(decisions["spellSwaps"]), Object{"level": sheet["totalLevel"], "classLevel": caster["level"], "classId": classID, "out": previous, "in": next})
	return nil
}
