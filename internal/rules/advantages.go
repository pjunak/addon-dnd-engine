package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// hydrateAdvantages lists the rolls passive grants.advantages and active
// "advantage" modifiers give Advantage on, plus Disadvantage on Stealth from
// worn armor that imposes it: on is save, check, attack, initiative,
// concentration or deathSave, optionally narrowed by abilities or skills. An
// entry with a when text applies only in that situation, so it is listed
// without marking rows. Conditions add their own entries later. This is sheet
// guidance; rolls are not resolved.
func hydrateAdvantages(sheet *character.Calculation, sources []grantSource, modifiers []activeModifier, armor armorState) {
	entries := []character.RollEntry{}
	add := func(advantage Object, name string, source character.Source) {
		if entry := rollEntry(advantage, name, source); entry != nil && armorRequirementMet(advantage["requires"], armor) {
			entries = append(entries, *entry)
		}
	}
	for _, source := range sources {
		for _, advantage := range objects(source.Grants["advantages"]) {
			add(advantage, firstText(source.Record["name"], source.Source.ID), source.Source)
		}
	}
	for _, modifier := range modifiers {
		if modifier.target() == "advantage" {
			add(modifier.fields, modifier.name, modifier.source)
		}
	}
	if restrictions := sheet.ArmorRestrictions; restrictions != nil && restrictions.StealthDisadvantage {
		entries = append(entries, character.RollEntry{
			On: character.RollCheck, Mode: character.RollDisadvantage, Skills: []string{"stealth"},
			Name:   firstText(restrictions.ArmorName, restrictions.ArmorID),
			Source: character.Source{Type: character.SourceArmor, ID: restrictions.ArmorID},
		})
	}
	if len(entries) > 0 {
		sheet.Advantages = entries
	}
	applyRollMarks(sheet)
}

// rollEntry validates one declared roll effect; an unknown roll or mode
// declares nothing rather than guessing.
func rollEntry(advantage Object, name string, source character.Source) *character.RollEntry {
	on, mode := character.RollKind(text(advantage["on"])), character.RollMode(text(advantage["mode"]))
	if !on.Valid() || mode != "" && !mode.Valid() {
		return nil
	}
	entry := &character.RollEntry{On: on, Name: name, Source: source, When: text(advantage["when"])}
	if mode == character.RollDisadvantage {
		entry.Mode = mode
	}
	if list := stringsOf(advantage["abilities"]); len(list) > 0 {
		entry.Abilities = list
	}
	if list := stringsOf(advantage["skills"]); len(list) > 0 {
		entry.Skills = list
	}
	return entry
}

// applyRollMarks recomputes every roll mark from sheet.advantages. Advantage
// and Disadvantage on the same roll cancel, so a row is marked only with the
// one that remains: advantage or disadvantage on save and skill rows,
// checkAdvantage or checkDisadvantage on ability rows, and
// <roll>Advantage or <roll>Disadvantage for attack, initiative, concentration
// and death save rolls. Situational entries mark nothing.
func applyRollMarks(sheet *character.Calculation) {
	type marks struct{ advantage, disadvantage bool }
	found := map[string]*marks{}
	mark := func(key string, mode character.RollMode) {
		if found[key] == nil {
			found[key] = &marks{}
		}
		if mode == character.RollDisadvantage {
			found[key].disadvantage = true
		} else {
			found[key].advantage = true
		}
	}
	for _, entry := range sheet.Advantages {
		if entry.When != "" {
			continue
		}
		switch entry.On {
		case character.RollSave:
			abilities := entry.Abilities
			if len(abilities) == 0 {
				abilities = Abilities[:]
			}
			for _, ability := range abilities {
				mark("save:"+ability, entry.Mode)
			}
		case character.RollCheck:
			for id, row := range sheet.Skills {
				if contains(entry.Skills, id) || contains(entry.Abilities, row.Ability) || len(entry.Skills)+len(entry.Abilities) == 0 {
					mark("skill:"+id, entry.Mode)
				}
			}
			for _, ability := range entry.Abilities {
				mark("ability:"+ability, entry.Mode)
			}
		case character.RollAttack, character.RollInitiative, character.RollConcentration, character.RollDeathSave:
			mark(string(entry.On), entry.Mode)
		}
	}
	// Clear earlier marks so a later pass (conditions) can recompute the net result.
	for id, row := range sheet.Saves {
		row.Advantage, row.Disadvantage = false, false
		sheet.Saves[id] = row
	}
	for id, row := range sheet.Skills {
		row.Advantage, row.Disadvantage = false, false
		sheet.Skills[id] = row
	}
	for id, row := range sheet.Abilities {
		row.CheckAdvantage, row.CheckDisadvantage = false, false
		sheet.Abilities[id] = row
	}
	sheet.RollMarks = character.RollMarks{}
	for key, value := range found {
		if value.advantage == value.disadvantage {
			continue
		}
		advantage, disadvantage := !value.disadvantage, value.disadvantage
		switch kind, id, _ := strings.Cut(key, ":"); kind {
		case "save":
			if row, exists := sheet.Saves[id]; exists {
				row.Advantage, row.Disadvantage = advantage, disadvantage
				sheet.Saves[id] = row
			}
		case "skill":
			if row, exists := sheet.Skills[id]; exists {
				row.Advantage, row.Disadvantage = advantage, disadvantage
				sheet.Skills[id] = row
			}
		case "ability":
			if row, exists := sheet.Abilities[id]; exists {
				row.CheckAdvantage, row.CheckDisadvantage = advantage, disadvantage
				sheet.Abilities[id] = row
			}
		default:
			markRoll(&sheet.RollMarks, character.RollKind(kind), advantage)
		}
	}
}

// markRoll sets the net mark of a roll outside the ability, save and skill rows.
func markRoll(marks *character.RollMarks, roll character.RollKind, advantage bool) {
	switch roll {
	case character.RollAttack:
		marks.AttackAdvantage, marks.AttackDisadvantage = advantage, !advantage
	case character.RollInitiative:
		marks.InitiativeAdvantage, marks.InitiativeDisadvantage = advantage, !advantage
	case character.RollConcentration:
		marks.ConcentrationAdvantage, marks.ConcentrationDisadvantage = advantage, !advantage
	case character.RollDeathSave:
		marks.DeathSaveAdvantage, marks.DeathSaveDisadvantage = advantage, !advantage
	case character.RollSave, character.RollCheck:
	}
}
