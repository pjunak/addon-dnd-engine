package rules

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// SpellOptions describes available controls without changing the authored sheet.
// Hydration's existing shape stays stable for consumers that only need stats.
func SpellOptions(decisions Object, sheet *character.Calculation, records Records, profile Ruleset) character.SpellOptions {
	casting := sheet.Spellcasting
	options := character.SpellOptions{
		Classes: []character.CasterOptions{}, PendingChoices: []character.SpellChoiceOptions{}, Granted: []character.SpellGrantOptions{},
		Slots: []character.SlotOption{}, CastingAbilityChoices: append([]character.CastingAbilityChoice{}, casting.CastingAbilityChoices...),
	}
	// The shared catalog is read-only; sort a copy of its entries.
	spells := slices.Clone(recordCatalog(records, "spell"))
	sort.Slice(spells, func(i, j int) bool { return text(spells[i]["id"]) < text(spells[j]["id"]) })
	for index := range casting.PerClass {
		caster := &casting.PerClass[index]
		ids, rituals := []string{}, []string{}
		costs, castSlots := map[string]float64{}, map[string][]string{}
		for _, spell := range spells {
			ref := text(spell["id"])
			if classSpellEligible(spell, caster) {
				ids = append(ids, ref)
			}
			if canCastRitual(decisions, caster, spell) {
				rituals = append(rituals, ref)
			}
			if caster.Prepares == "spellbook" && classSpellEligible(spell, caster) && integer(spell["level"], 0) > 0 {
				costs[ref] = ScrollCopyCost(number(spell["level"], 0), profile)
			}
			if contains(stringsOf(object(decisions["preparedSpells"])[caster.ClassID]), ref) || contains(stringsOf(object(decisions["cantrips"])[caster.ClassID]), ref) {
				castSlots[ref] = spellSlotKeys(decisions, sheet, records, ref, integer(spell["level"], 0))
			}
		}
		options.Classes = append(options.Classes, character.CasterOptions{ClassID: caster.ClassID, SpellIDs: ids, RitualIDs: rituals, CopyCosts: costs, CastSlots: castSlots, CanSwap: caster.Prepares != "spellbook"})
	}
	for _, choice := range casting.PendingChoices {
		eligible := []string{}
		for _, spell := range spells {
			if grantSpellEligible(spell, choice) {
				eligible = append(eligible, text(spell["id"]))
			}
		}
		options.PendingChoices = append(options.PendingChoices, character.SpellChoiceOptions{SpellChoice: choice, EligibleSpellIDs: eligible})
	}
	for _, grant := range casting.Granted {
		entry := character.SpellGrantOptions{SpellGrant: grant, Key: spellGrantKey(grant)}
		if itemSpell(grant) {
			entry.Slots = itemSpellCasts(grant, casting)
			options.Granted = append(options.Granted, entry)
			continue
		}
		level := 0
		if grant.Level != nil {
			level = *grant.Level
		}
		keys := spellSlotKeys(decisions, sheet, records, grant.Ref, level)
		if grant.RitualOnly {
			keys = []string{""}
		} else if grant.AtWill && (len(keys) == 0 || keys[0] != "") {
			keys = append([]string{""}, keys...)
		}
		if key := grant.CastResource; key != "" && findPlayResource(sheet, key) != nil {
			keys = append([]string{key}, keys...)
		}
		if grant.Free != nil && *grant.Free != "" && findPlayResource(sheet, grantFreeResourceKey(grant)) != nil {
			keys = append([]string{grantFreeResourceKey(grant)}, keys...)
		}
		entry.Slots = keys
		options.Granted = append(options.Granted, entry)
	}
	for index := range sheet.Resources {
		resource := &sheet.Resources[index]
		if resource.Kind != character.ResourceSlot && resource.Kind != character.ResourceCharge {
			continue
		}
		options.Slots = append(options.Slots, character.SlotOption{Key: resource.Key, Name: resource.Name, Max: resource.Max, Current: remainingResource(decisions, resource)})
	}
	return options
}

func classSpellEligible(spell Object, caster *character.Caster) bool {
	if spell == nil || caster == nil || integer(spell["level"], 0) > caster.MaxSpellLevel {
		return false
	}
	for _, list := range append([]string{caster.SpellListClassID}, caster.SpellLists...) {
		if contains(stringsOf(spell["classes"]), list) {
			return true
		}
	}
	return contains(caster.ExpandedSpellIDs, text(spell["id"]))
}

func grantSpellEligible(spell Object, choice character.SpellChoice) bool {
	if spell == nil {
		return false
	}
	level := integer(spell["level"], 0)
	if choice.SpellLevel != nil && level != *choice.SpellLevel || choice.MaxSpellLevel != nil && level > *choice.MaxSpellLevel {
		return false
	}
	from := choice.From
	eligible := len(from.Class)+len(from.School)+len(from.IDs) == 0 || contains(from.IDs, text(spell["id"]))
	for _, classID := range from.Class {
		eligible = eligible || contains(stringsOf(spell["classes"]), classID)
	}
	for _, school := range from.School {
		eligible = eligible || strings.EqualFold(school, text(spell["school"]))
	}
	if len(from.CastingTime) > 0 {
		match := false
		for _, value := range from.CastingTime {
			match = match || strings.EqualFold(value, text(spell["castingTime"]))
		}
		eligible = eligible && match
	}
	if from.Ritual {
		eligible = eligible && truth(spell["ritual"])
	}
	return eligible
}

func selectGrantSpell(decisions Object, sheet *character.Calculation, command character.PlayCommand, records Records) error {
	var choice *character.SpellChoice
	for index := range sheet.Spellcasting.PendingChoices {
		if sheet.Spellcasting.PendingChoices[index].Key == command.Key {
			choice = &sheet.Spellcasting.PendingChoices[index]
			break
		}
	}
	if choice == nil {
		return fmt.Errorf("This spell choice is no longer available.")
	}
	current := unique(choice.Picked)
	if command.Selected {
		if !grantSpellEligible(recordByID(records, "spell", command.Ref), *choice) {
			return fmt.Errorf("This spell does not meet the grant's requirements.")
		}
		if contains(current, command.Ref) {
			return nil
		}
		if len(current) >= choice.Choose {
			return fmt.Errorf("Remove a selected spell before choosing another.")
		}
		current = append(current, command.Ref)
	} else {
		filtered := []string{}
		for _, item := range current {
			if item != command.Ref {
				filtered = append(filtered, item)
			}
		}
		current = filtered
	}
	playMap(decisions, "grantChoices")[command.Key] = anyStrings(current)
	return nil
}

func selectCastingAbility(decisions Object, sheet *character.Calculation, command character.PlayCommand) error {
	for _, choice := range sheet.Spellcasting.CastingAbilityChoices {
		if choice.Key != command.Key {
			continue
		}
		if !contains(choice.Options, command.Ability) {
			return fmt.Errorf("Choose an available spellcasting ability.")
		}
		playMap(decisions, "grantCastingAbilities")[command.Key] = command.Ability
		return nil
	}
	return fmt.Errorf("This spellcasting ability choice is no longer available.")
}

func spellGrantKey(grant character.SpellGrant) string {
	return grantOwner(grant.Source) + ":" + grant.Ref
}

func castGrantedSpell(decisions Object, sheet *character.Calculation, command character.PlayCommand, records Records) error {
	for _, grant := range sheet.Spellcasting.Granted {
		if spellGrantKey(grant) != command.Key {
			continue
		}
		slot := command.Slot
		if recordByID(records, "spell", grant.Ref) == nil {
			return fmt.Errorf("This granted spell is unavailable in the selected rules data.")
		}
		if itemSpell(grant) {
			if itemCastingAbilityPending(grant, sheet.Spellcasting) {
				return fmt.Errorf("Choose the item's spellcasting ability before casting its spells.")
			}
			if !contains(itemSpellCasts(grant, sheet.Spellcasting), slot) {
				return fmt.Errorf("Cast this item spell with the item's own uses; item spells never use spell slots.")
			}
			if slot == "" {
				return nil
			}
			return spendResourceUses(decisions, findPlayResource(sheet, slot), max(1, grant.CastResourceCost))
		}
		if slot == grantFreeResourceKey(grant) && grant.Free != nil && *grant.Free != "" {
			return spendResource(decisions, findPlayResource(sheet, slot))
		}
		if key := grant.CastResource; key != "" && slot == key {
			return spendResourceUses(decisions, findPlayResource(sheet, key), max(1, grant.CastResourceCost))
		}
		// At-will and ritual-only grants cast without a slot or resource.
		if slot == "" && (grant.AtWill || grant.RitualOnly) {
			return nil
		}
		if grant.RitualOnly {
			return fmt.Errorf("This spell can only be cast as a ritual.")
		}
		level := 0
		if grant.Level != nil {
			level = *grant.Level
		}
		return spendSpellSlot(decisions, sheet, records, grant.Ref, level, slot)
	}
	return fmt.Errorf("This spell is not granted by the current build.")
}

func itemSpell(grant character.SpellGrant) bool {
	return grant.Source.Type == character.SourceItem
}

// itemSpellCasts lists how an item spell can be cast: with the item's own use,
// or with nothing for at-will spells and cantrips, never with spell slots. It
// lists nothing while the item's casting ability is still to be picked.
func itemSpellCasts(grant character.SpellGrant, spellcasting character.Spellcasting) []string {
	casts := []string{}
	if itemCastingAbilityPending(grant, spellcasting) {
		return casts
	}
	if grant.CastResource != "" {
		casts = append(casts, grant.CastResource)
	}
	if grant.AtWill || grant.Level != nil && *grant.Level == 0 {
		casts = append(casts, "")
	}
	return casts
}

func itemCastingAbilityPending(grant character.SpellGrant, spellcasting character.Spellcasting) bool {
	owner := grantOwner(grant.Source)
	for _, choice := range spellcasting.CastingAbilityChoices {
		if grantOwner(choice.Source) == owner && choice.Selected == nil {
			return true
		}
	}
	return false
}

func spellSlotKeys(decisions Object, sheet *character.Calculation, records Records, ref string, level int) []string {
	if level == 0 {
		return []string{""}
	}
	result := []string{}
	for _, resource := range sheet.Resources {
		if resource.Kind != character.ResourceSlot {
			continue
		}
		slotLevel := resource.Level
		if strings.HasPrefix(resource.Key, "slot-") {
			slotLevel, _ = strconv.Atoi(strings.TrimPrefix(resource.Key, "slot-"))
		}
		if resource.Key == "pact-slot" {
			for _, caster := range sheet.Spellcasting.PerClass {
				if caster.Pact != nil {
					slotLevel = max(slotLevel, caster.Pact.Level)
				}
			}
		}
		if slotLevel < level {
			continue
		}
		if restriction := resource.Restriction; restriction != "" {
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
		result = append(result, resource.Key)
	}
	return unique(result)
}

func spendSpellSlot(decisions Object, sheet *character.Calculation, records Records, ref string, level int, slot string) error {
	if !contains(spellSlotKeys(decisions, sheet, records, ref, level), slot) {
		return fmt.Errorf("Choose an eligible spell slot of this spell's level or higher.")
	}
	if level == 0 {
		return nil
	}
	return spendResource(decisions, findPlayResource(sheet, slot))
}

func canCastRitual(decisions Object, caster *character.Caster, spell Object) bool {
	if caster == nil || spell == nil || !caster.Ritual || !truth(spell["ritual"]) {
		return false
	}
	key := "preparedSpells"
	if caster.Prepares == "spellbook" {
		key = "spellbook"
	}
	return contains(stringsOf(object(decisions[key])[caster.ClassID]), text(spell["id"]))
}

func copyPlaySpell(decisions Object, sheet *character.Calculation, command character.PlayCommand, records Records, profile Ruleset) error {
	classID, ref, scrollID := command.ClassID, command.Ref, command.ScrollID
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
			// Only an explicit link counts: item names are free text, in any
			// language, and one spell's name can be part of another's.
			if text(item["spellRef"]) != ref || integer(item["qty"], 0) < 1 {
				return fmt.Errorf("Choose a remaining scroll for this spell.")
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("The selected scroll is no longer in the inventory.")
		}
	}
	if err := selectPlaySpell(decisions, sheet, classID, ref, character.ListSpellbook, true, records); err != nil {
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

func swapPlaySpell(decisions Object, sheet *character.Calculation, command character.PlayCommand, records Records) error {
	classID, previous, next := command.ClassID, command.Out, command.Ref
	caster := playCaster(sheet, classID)
	selected := stringsOf(object(decisions["preparedSpells"])[classID])
	if caster == nil || caster.Prepares == "spellbook" || !contains(selected, previous) || contains(selected, next) {
		return fmt.Errorf("Choose a known spell to replace and a different eligible spell.")
	}
	if err := selectPlaySpell(decisions, sheet, classID, previous, character.ListPrepared, false, records); err != nil {
		return err
	}
	if err := selectPlaySpell(decisions, sheet, classID, next, character.ListPrepared, true, records); err != nil {
		return err
	}
	decisions["spellSwaps"] = append(values(decisions["spellSwaps"]), Object{"level": sheet.TotalLevel, "classLevel": caster.Level, "classId": classID, "out": previous, "in": next})
	return nil
}
