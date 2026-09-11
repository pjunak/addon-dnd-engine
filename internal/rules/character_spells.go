package rules

import (
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
	"time"
)

func validateCharacterSpells(input character.Inputs, records Records, result *character.Result) {
	block := func(id, message string) { addCharacterIssue(result, id, "spells", message, "blocker", nil) }
	casters := map[string]Object{}
	for _, caster := range objects(object(result.Sheet["spellcasting"])["perClass"]) {
		casters[text(caster["classId"])] = caster
	}
	choices := map[string]Object{}
	copies := map[string]map[string]bool{}
	acquisitionIDs := map[string]bool{}
	for _, swap := range input.Build.Spells.Swaps {
		if swap.Level < 1 || swap.ClassLevel < 2 || swap.ClassID == "" || swap.In == "" || swap.Out == "" || !contains([]string{"", "recorded", "import"}, swap.Origin) {
			block("spell-replacement-ledger", "Spell replacements require a class, acquired level, original spell and replacement.")
		}
	}
	for _, acquisition := range input.Build.Spells.Acquisitions {
		_, timeError := time.Parse(time.RFC3339, acquisition.At)
		if copies[acquisition.ClassID] == nil {
			copies[acquisition.ClassID] = map[string]bool{}
		}
		if acquisition.ID == "" || acquisitionIDs[acquisition.ID] || copies[acquisition.ClassID][acquisition.SpellID] || acquisition.Level < 1 || acquisition.CostGP < 0 || timeError != nil || !contains([]string{"", "recorded", "import"}, acquisition.Origin) {
			block("spell-acquisition:"+acquisition.ID, "Spell acquisitions need a unique ID, source, level, time and recorded non-negative cost.")
		}
		acquisitionIDs[acquisition.ID] = true
		copies[acquisition.ClassID][acquisition.SpellID] = true
	}
	for _, choice := range objects(result.SpellOptions["pendingChoices"]) {
		choices[text(choice["key"])] = choice
	}
	for _, selection := range []struct {
		name   string
		values map[string][]string
	}{{"cantrips", input.Build.Spells.Cantrips}, {"spellbook", input.Build.Spells.Spellbook}, {"prepared", input.Play.PreparedSpells}} {
		for classID, ids := range selection.values {
			caster := casters[classID]
			uniqueIDs := map[string]bool{}
			limit := integer(caster["preparedLimit"], 0)
			if selection.name == "cantrips" {
				limit = integer(caster["cantripsKnown"], 0)
			}
			if selection.name == "spellbook" {
				limit = integer(caster["spellbookKnown"], 0)
				for _, id := range ids {
					if copies[classID][id] {
						limit++
					}
				}
			}
			if len(ids) > limit || caster == nil && len(ids) > 0 {
				block("spell-count:"+selection.name+":"+classID, "The selected spells exceed the currently granted capacity.")
			}
			for _, id := range ids {
				spell := recordByID(records, "spell", id)
				valid := !uniqueIDs[id] && classSpellEligible(spell, caster)
				uniqueIDs[id] = true
				if selection.name == "cantrips" {
					valid = valid && integer(spell["level"], -1) == 0
				} else {
					valid = valid && integer(spell["level"], 0) > 0
				}
				if selection.name == "spellbook" {
					valid = valid && text(caster["prepares"]) == "spellbook"
				}
				if selection.name == "prepared" && text(caster["prepares"]) == "spellbook" {
					valid = valid && contains(input.Build.Spells.Spellbook[classID], id)
				}
				if !valid {
					block("spell-eligibility:"+selection.name+":"+classID+":"+id, "A selected spell is unavailable or ineligible for this class, level or preparation list.")
				}
			}
		}
	}
	for classID, caster := range casters {
		if len(input.Build.Spells.Cantrips[classID]) != integer(caster["cantripsKnown"], 0) {
			block("cantrip-count:"+classID, "Choose the cantrips granted by this class level.")
		}
		if text(caster["prepares"]) == "spellbook" && len(input.Build.Spells.Spellbook[classID]) < integer(caster["spellbookKnown"], 0) {
			block("spellbook-count:"+classID, "Choose the spellbook spells granted by the recorded class levels.")
		}
	}
	for key, ids := range input.Build.Spells.GrantChoices {
		choice := choices[key]
		seen := map[string]bool{}
		if choice == nil && len(ids) > 0 || len(ids) > integer(choice["choose"], 0) {
			block("spell-grant:"+key, "This spell choice no longer fits its granting source.")
		}
		for _, id := range ids {
			if seen[id] || !contains(stringsOf(choice["eligibleSpellIds"]), id) {
				block("spell-grant:"+key+":"+id, "This spell is not eligible for its granting source.")
			}
			seen[id] = true
		}
	}
	abilityChoices := map[string]Object{}
	for _, choice := range objects(result.SpellOptions["castingAbilityChoices"]) {
		abilityChoices[text(choice["key"])] = choice
	}
	for key, ability := range input.Build.Spells.CastingAbilities {
		choice := abilityChoices[key]
		eligible := stringsOf(choice["options"])
		if !contains(eligible, ability) {
			block("casting-ability:"+key, "The casting ability is no longer granted by this source.")
		}
	}
	activations := map[string]Object{}
	for _, activation := range objects(result.Sheet["activations"]) {
		activations[text(activation["key"])] = activation
	}
	groups := map[string]string{}
	for key, enabled := range input.Play.ActiveFeatures {
		if !enabled {
			continue
		}
		activation := activations[key]
		if activation == nil || !truth(activation["available"]) {
			block("activation:"+key, "An active feature is unavailable with the current build or equipment.")
		}
		if group := text(activation["exclusiveGroup"]); group != "" {
			if groups[group] != "" {
				block("activation-group:"+group, "Choose one active feature in this exclusive group.")
			}
			groups[group] = key
		}
	}
}

// Spellbook order records the use of level-granted slots. Copied spells have
// their own paid acquisition ledger and never consume or recreate those slots.
// Checking only today's total would let late spells occupy first-level gains.
func validateCharacterSpellProgression(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	copied := map[string]map[string]bool{}
	for _, acquisition := range input.Build.Spells.Acquisitions {
		if copied[acquisition.ClassID] == nil {
			copied[acquisition.ClassID] = map[string]bool{}
		}
		copied[acquisition.ClassID][acquisition.SpellID] = true
	}
	used := map[string]int{}
	levels := map[string]int{}
	decisions := []any{}
	for index, level := range input.Build.Levels {
		levels[level.ClassID]++
		selected := input.Build.Spells.Spellbook[level.ClassID]
		if len(selected) == 0 {
			continue
		}
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:index+1]
		detached := character.Result{Issues: []character.Issue{}}
		normalized := NormalizeBuilderDecisions(characterDecisions(prefix, records, profile, &detached), records, profile)
		sheet := Hydrate(normalized, records, &profile).Sheet
		for _, caster := range objects(object(sheet["spellcasting"])["perClass"]) {
			if text(caster["classId"]) != level.ClassID || text(caster["prepares"]) != "spellbook" {
				continue
			}
			ordinary := []string{}
			for _, id := range selected {
				if !copied[level.ClassID][id] {
					ordinary = append(ordinary, id)
				}
			}
			limit := min(integer(caster["spellbookKnown"], 0), len(ordinary))
			for slot := used[level.ClassID]; slot < limit; slot++ {
				id := ordinary[slot]
				decisions = append(decisions, Object{"id": id, "classId": level.ClassID, "level": index + 1, "classLevel": levels[level.ClassID], "source": Object{"kind": "class", "id": level.ClassID}})
				if !classSpellEligible(recordByID(records, "spell", id), caster) {
					addCharacterIssue(result, "spell-acquired-level:"+level.ClassID+":"+id, "spells", fmt.Sprintf("Spell %s is not eligible at its recorded gain at character level %d. Reorder level-granted spellbook choices or replace it.", id, index+1), "blocker", &character.Reference{Kind: "spell", ID: id})
				}
			}
			used[level.ClassID] = limit
		}
	}
	result.SpellOptions["acquisitionLevels"] = decisions
}

// Source data declares replacement allowances. The current level's ledger is
// consumed once, independently of rests, provider reloads or repeated clicks.
func characterSpellReplacements(input character.Inputs, records Records, classID string) int {
	classLevel := 0
	for _, level := range input.Build.Levels {
		if level.ClassID == classID {
			classLevel++
		}
	}
	if classLevel < 2 {
		return 0
	}
	casting := object(recordByID(records, "class", classID)["spellcasting"])
	if subclass := input.Build.Subclasses[classID]; subclass != "" {
		if extra := object(recordByID(records, "subclass", subclass)["spellcasting"]); extra["levelReplacements"] != nil {
			casting = extra
		}
	}
	remaining := integer(casting["levelReplacements"], 0)
	for _, swap := range input.Build.Spells.Swaps {
		if swap.ClassID == classID && swap.ClassLevel == classLevel {
			remaining--
		}
	}
	return max(0, remaining)
}
