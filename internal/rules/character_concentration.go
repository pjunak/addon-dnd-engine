package rules

import (
	"fmt"

	"github.com/pjunak/addon-dnd-engine/character"
)

// characterConcentration projects the saved concentration: the spell, the
// pending save after damage with its bonus, and any active condition that
// ends it. The saved value stays until a command ends it.
func characterConcentration(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	saved := input.Play.Concentration
	if saved == nil {
		return
	}
	spell := recordByID(records, "spell", saved.Ref)
	if spell == nil {
		addCharacterIssue(result, "concentration-spell", "concentration", "The spell being concentrated on is unavailable in the selected rules data.", character.SeverityWarning, nil)
	}
	row := character.SheetConcentration{Ref: saved.Ref, Name: firstText(spell["name"], saved.Ref), SaveDC: max(0, saved.SaveDC)}
	terms := []character.Term{}
	if policy := profile.Constants.Concentration; policy != nil {
		save, bonus := 0, 0
		if sheet := result.Sheet.Calculation; sheet != nil {
			save, bonus = sheet.Saves[policy.SaveAbility].Total, sheet.ConcentrationSaveBonus
		}
		total := save + bonus
		row.SaveAbility, row.SaveBonus = policy.SaveAbility, &total
		terms = append(terms, character.Term{Label: policy.SaveAbility + " saving throw", Value: save})
		if bonus != 0 {
			terms = append(terms, character.Term{Label: "Concentration bonus", Value: bonus})
		}
	}
	for _, condition := range result.Sheet.Conditions {
		if condition.EndsConcentration {
			row.EndedBy = condition.Name
			break
		}
	}
	result.Sheet.Concentration = &row
	result.Explanations["concentration"] = character.Explanation{Label: "Concentration", Formula: "Authored play state. Casting a Concentration spell replaces it; damage records the save DC; keeping or ending it is an explicit command.", Value: row, Terms: terms, Sources: []character.Reference{}}
}

// concentrationAfterDamage records the save DC for damage taken while
// concentrating. Without a ruleset policy no save is recorded.
func concentrationAfterDamage(input *character.Inputs, damage int, profile Ruleset) {
	if input.Play.Concentration == nil || damage < 1 || profile.Constants.Concentration == nil {
		return
	}
	input.Play.Concentration.SaveDC = profile.Constants.Concentration.SaveDC(damage)
}

// applyConcentrationCommand keeps or ends concentration. Both work on a
// character with blocking issues, so an invalid saved value can be ended.
func applyConcentrationCommand(input character.Inputs, records Records, profile Ruleset, change func(*character.Inputs)) (character.Result, error) {
	input = cloneCharacter(input)
	if input.Play.Concentration == nil {
		return EvaluateCharacter(input, records, profile), fmt.Errorf("The character is not concentrating on a spell.")
	}
	change(&input)
	return EvaluateCharacter(input, records, profile), nil
}
