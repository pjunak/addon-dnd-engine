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
		addCharacterIssue(result, "concentration-spell", "concentration", "The spell being concentrated on is unavailable in the selected rules data.", "warning", nil)
	}
	name := text(spell["name"])
	if name == "" {
		name = saved.Ref
	}
	row := Object{"ref": saved.Ref, "name": name}
	terms := []character.Term{}
	if policy := profile.Constants.Concentration; policy != nil {
		save := object(object(result.Sheet["saves"])[policy.SaveAbility])
		bonus := integer(result.Sheet["concentrationSaveBonus"], 0)
		row["saveAbility"] = policy.SaveAbility
		row["saveBonus"] = integer(save["total"], 0) + bonus
		terms = append(terms, character.Term{Label: policy.SaveAbility + " saving throw", Value: integer(save["total"], 0)})
		if bonus != 0 {
			terms = append(terms, character.Term{Label: "Concentration bonus", Value: bonus})
		}
	}
	if saved.SaveDC > 0 {
		row["saveDC"] = saved.SaveDC
	}
	for _, condition := range objects(result.Sheet["conditions"]) {
		if truth(condition["endsConcentration"]) {
			row["endedBy"] = text(condition["name"])
			break
		}
	}
	result.Sheet["concentration"] = row
	result.Explanations["concentration"] = character.Explanation{Label: "Concentration", Formula: "Authored play state. Casting a Concentration spell replaces it; damage records the save DC; keeping or ending it is an explicit command.", Value: row, Terms: terms, Sources: []character.Reference{}}
}

// castConcentration is the spell a successful cast command starts
// concentrating on, or "" when the spell does not need Concentration.
func castConcentration(change Object, sheet Object, records Records) string {
	ref := text(change["ref"])
	if text(change["operation"]) == "cast-granted-spell" {
		ref = ""
		for _, grant := range objects(object(sheet["spellcasting"])["granted"]) {
			if spellGrantKey(grant) == text(change["key"]) {
				ref = text(grant["ref"])
			}
		}
	}
	if !contains([]string{"cast-spell", "cast-granted-spell", "cast-ritual"}, text(change["operation"])) || !truth(recordByID(records, "spell", ref)["concentration"]) {
		return ""
	}
	return ref
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
func applyConcentrationCommand(input character.Inputs, change Object, records Records, profile Ruleset) (character.Result, error) {
	if len(change) != 1 {
		return EvaluateCharacter(input, records, profile), fmt.Errorf("The play change has invalid fields.")
	}
	input = cloneCharacter(input)
	if input.Play.Concentration == nil {
		return EvaluateCharacter(input, records, profile), fmt.Errorf("The character is not concentrating on a spell.")
	}
	if text(change["operation"]) == "end-concentration" {
		input.Play.Concentration = nil
	} else {
		input.Play.Concentration.SaveDC = 0
	}
	return EvaluateCharacter(input, records, profile), nil
}
