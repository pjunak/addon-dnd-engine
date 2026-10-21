package rules

import (
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
	"slices"
	"strings"
	"time"
)

func validateCharacterSpells(input character.Inputs, records Records, result *character.Result) {
	block := func(id, message string) {
		addCharacterIssue(result, id, "spells", message, character.SeverityBlocker, nil)
	}
	sheet := result.Sheet.Calculation
	casters := map[string]*character.Caster{}
	for index := range sheet.Spellcasting.PerClass {
		casters[sheet.Spellcasting.PerClass[index].ClassID] = &sheet.Spellcasting.PerClass[index]
	}
	choices := map[string]*character.SpellChoiceOptions{}
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
	for index := range result.SpellOptions.PendingChoices {
		choices[result.SpellOptions.PendingChoices[index].Key] = &result.SpellOptions.PendingChoices[index]
	}
	for _, selection := range []struct {
		name   string
		values map[string][]string
	}{{"cantrips", input.Build.Spells.Cantrips}, {"spellbook", input.Build.Spells.Spellbook}, {"prepared", input.Play.PreparedSpells}} {
		for _, classID := range sortedKeys(selection.values) {
			ids := selection.values[classID]
			caster := casters[classID]
			uniqueIDs := map[string]bool{}
			if caster == nil {
				// The character no longer has this spellcasting class, for
				// example after a level was removed.
				if len(ids) > 0 {
					block("spell-class:"+selection.name+":"+classID, "These spells belong to a spellcasting class the character no longer has.")
				}
				continue
			}
			limit := caster.PreparedLimit
			if selection.name == "cantrips" {
				limit = caster.CantripsKnown
			}
			if selection.name == "spellbook" {
				limit = caster.SpellbookCapacity
			}
			if len(ids) > limit {
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
					valid = valid && caster.Prepares == "spellbook"
				}
				if selection.name == "prepared" && caster.Prepares == "spellbook" {
					valid = valid && contains(input.Build.Spells.Spellbook[classID], id)
				}
				if !valid {
					block("spell-eligibility:"+selection.name+":"+classID+":"+id, "A selected spell is unavailable or ineligible for this class, level or preparation list.")
				}
			}
		}
	}
	for _, classID := range sortedKeys(casters) {
		caster := casters[classID]
		if len(input.Build.Spells.Cantrips[classID]) != caster.CantripsKnown {
			block("cantrip-count:"+classID, "Choose the cantrips granted by this class level.")
		}
		if caster.Prepares == "spellbook" && len(input.Build.Spells.Spellbook[classID]) < caster.SpellbookKnown {
			block("spellbook-count:"+classID, "Choose the spellbook spells granted by the recorded class levels.")
		}
		for _, short := range spellbookSchoolShortfalls(*caster, input.Build.Spells.Spellbook[classID], records) {
			addCharacterIssue(result, "spellbook-school:"+classID+":"+strings.ToLower(short.school), "spells", "Choose the free spellbook spells from the school their feature names.", character.SeverityBlocker, &character.Reference{Kind: string(short.source.Type), ID: short.source.ID})
		}
	}
	// An owned item that is not in effect keeps its picks and toggles for when
	// it is equipped again; they are not checked against today's sources.
	activeItems := activeCharacterItems(input, records)
	inactiveItem := func(key string) bool {
		owner := itemGrantOwner(key, input.Play.Inventory)
		return owner != "" && !activeItems[owner]
	}
	for _, key := range sortedKeys(input.Build.Spells.GrantChoices) {
		if inactiveItem(key) {
			continue
		}
		ids := input.Build.Spells.GrantChoices[key]
		choice := choices[key]
		if choice == nil {
			choice = &character.SpellChoiceOptions{}
			if len(ids) > 0 {
				block("spell-grant:"+key, "This spell choice no longer fits its granting source.")
			}
		}
		seen := map[string]bool{}
		if len(ids) > choice.Choose {
			block("spell-grant:"+key, "This spell choice no longer fits its granting source.")
		}
		for _, id := range ids {
			if seen[id] || !contains(choice.EligibleSpellIDs, id) {
				block("spell-grant:"+key+":"+id, "This spell is not eligible for its granting source.")
			}
			seen[id] = true
		}
	}
	abilityChoices := map[string]character.CastingAbilityChoice{}
	for _, choice := range result.SpellOptions.CastingAbilityChoices {
		abilityChoices[choice.Key] = choice
	}
	for _, key := range sortedKeys(input.Build.Spells.CastingAbilities) {
		if inactiveItem(key) {
			continue
		}
		ability := input.Build.Spells.CastingAbilities[key]
		if !contains(abilityChoices[key].Options, ability) {
			block("casting-ability:"+key, "The casting ability is no longer granted by this source.")
		}
	}
	activations := map[string]character.Activation{}
	for _, activation := range sheet.Activations {
		activations[activation.Key] = activation
	}
	groups := map[string]string{}
	for _, key := range sortedKeys(input.Play.ActiveFeatures) {
		enabled := input.Play.ActiveFeatures[key]
		if !enabled || inactiveItem(key) {
			continue
		}
		activation, exists := activations[key]
		if !exists || !activation.Available {
			block("activation:"+key, "An active feature is unavailable with the current build or equipment.")
		}
		if group := activation.ExclusiveGroup; group != nil && *group != "" {
			if groups[*group] != "" {
				block("activation-group:"+*group, "Choose one active feature in this exclusive group.")
			}
			groups[*group] = key
		}
	}
}

// spellbookSchoolNeed is how many spellbook spells of one school the class's
// spellbookBonus sources require; source is the first source naming it.
type spellbookSchoolNeed struct {
	school string
	count  int
	source character.Source
}

// spellbookSchoolShortfalls lists the schools the class's spellbook holds too
// few spells of. Any assignment works once enough are present, because the
// remaining level-granted spells have no school.
func spellbookSchoolShortfalls(caster character.Caster, spellbook []string, records Records) []spellbookSchoolNeed {
	needs := []spellbookSchoolNeed{}
	for _, row := range caster.SpellbookBonusSources {
		school := row.School
		if school == "" {
			continue
		}
		index := slices.IndexFunc(needs, func(need spellbookSchoolNeed) bool { return strings.EqualFold(need.school, school) })
		if index < 0 {
			index = len(needs)
			needs = append(needs, spellbookSchoolNeed{school: school, source: row.Source})
		}
		needs[index].count += row.Count
	}
	shortfalls := []spellbookSchoolNeed{}
	for _, need := range needs {
		held := 0
		for _, id := range unique(spellbook) {
			if strings.EqualFold(text(recordByID(records, "spell", id)["school"]), need.school) {
				held++
			}
		}
		if held < need.count {
			shortfalls = append(shortfalls, need)
		}
	}
	return shortfalls
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
	decisions := []character.AcquisitionLevel{}
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
		sheet := Hydrate(normalized, records, profile).Sheet
		for casterIndex := range sheet.Spellcasting.PerClass {
			caster := &sheet.Spellcasting.PerClass[casterIndex]
			if caster.ClassID != level.ClassID || caster.Prepares != "spellbook" {
				continue
			}
			ordinary := []string{}
			for _, id := range selected {
				if !copied[level.ClassID][id] {
					ordinary = append(ordinary, id)
				}
			}
			limit := min(caster.SpellbookKnown, len(ordinary))
			for slot := used[level.ClassID]; slot < limit; slot++ {
				id := ordinary[slot]
				decisions = append(decisions, character.AcquisitionLevel{ID: id, ClassID: level.ClassID, Level: index + 1, ClassLevel: levels[level.ClassID], Source: character.Reference{Kind: "class", ID: level.ClassID}})
				if !classSpellEligible(recordByID(records, "spell", id), caster) {
					addCharacterIssue(result, "spell-acquired-level:"+level.ClassID+":"+id, "spells", fmt.Sprintf("Spell %s is not eligible at its recorded gain at character level %d. Reorder level-granted spellbook choices or replace it.", id, index+1), character.SeverityBlocker, &character.Reference{Kind: "spell", ID: id})
				}
			}
			used[level.ClassID] = limit
		}
	}
	result.SpellOptions.AcquisitionLevels = decisions
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
