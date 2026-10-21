package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Proficiency choices share one eligibility pool, including typed skills/tools
// and mixed grants. Earlier valid choices reserve their training; within a level,
// class choices and direct origins precede the feats those origins can grant.
func characterProficiencyOptions(input character.Inputs, plan character.Plan, records Records, profile Ruleset) map[string][]character.Option {
	pending := map[string]bool{}
	for _, group := range [][]character.PlanChoice{plan.CreationChoices, plan.ClassChoices} {
		for _, choice := range group {
			if isProficiencyChoice(choice) {
				pending[choice.ID] = true
			}
		}
	}
	result, claimed, combinedSeen := map[string][]character.Option{}, map[string]bool{}, map[string]bool{}
	for index := 0; index < max(1, len(input.Build.Levels)) && len(pending) > 0; index++ {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:min(index+1, len(prefix.Build.Levels))]
		detached := character.Result{Issues: []character.Issue{}}
		decisions := characterDecisions(prefix, records, profile, &detached)
		prefixPlan := BuilderPlan(decisions, records, profile)
		choices, replaceable := []character.PlanChoice{}, []character.PlanChoice{}
		combinedNow := []string{}
		basePlan := prefixPlan
		basePlan.ClassChoices, basePlan.CreationChoices = []character.PlanChoice{}, []character.PlanChoice{}
		for _, group := range []struct {
			choices []character.PlanChoice
			base    *[]character.PlanChoice
		}{{prefixPlan.ClassChoices, &basePlan.ClassChoices}, {prefixPlan.CreationChoices, &basePlan.CreationChoices}} {
			for _, choice := range group.choices {
				if !isProficiencyChoice(choice) {
					// Combined training may add Expertise to an existing skill;
					// it must not disqualify that earlier proficiency choice.
					if choice.Kind == character.ChoiceSkillExpertise && !combinedSeen[choice.ID] {
						combinedNow = append(combinedNow, choice.ID)
						continue
					}
					*group.base = append(*group.base, choice)
					continue
				}
				if !pending[choice.ID] {
					continue
				}
				if choice.ChangeOn != "" {
					if index+1 >= len(input.Build.Levels) {
						replaceable = append(replaceable, choice)
					}
				} else {
					choices = append(choices, choice)
				}
			}
		}
		for _, id := range combinedNow {
			combinedSeen[id] = true
		}
		// Replaceable training uses current fixed grants and all permanent
		// selections, without retroactively invalidating earlier choices.
		choices = append(choices, replaceable...)
		if len(choices) == 0 {
			continue
		}
		normalized := NormalizeBuilderDecisions(decisions, records, profile)
		for key, value := range resolveBuilderChoices(decisions, basePlan, records) {
			normalized[key] = value
		}
		sheet := Hydrate(normalized, records, profile).Sheet
		applyCharacterEffects(prefix, sheet, &detached, records)
		tools := sheet.Proficiencies.Tools
		for _, choice := range choices {
			if choice.ChangeOn != "" {
				for _, combined := range append(append([]character.PlanChoice{}, prefixPlan.ClassChoices...), prefixPlan.CreationChoices...) {
					if combined.Kind == character.ChoiceSkillExpertise {
						for _, skill := range choiceValues(decisions, combined) {
							claimed["skill:"+canonicalSkill(skill)] = true
						}
					}
				}
			}
			options, eligible := []character.Option{}, map[string]bool{}
			for _, option := range builderChoiceOptions(choice, sheet, records) {
				key := proficiencyChoiceKey(choice, option.ID)
				already := claimed[key]
				if skill, ok := strings.CutPrefix(key, "skill:"); ok {
					already = already || sheet.Skills[canonicalSkill(skill)].Proficient
				}
				if tool, ok := strings.CutPrefix(key, "tool:"); ok {
					already = already || contains(tools, tool)
				}
				if !already {
					options = append(options, option)
					eligible[option.ID] = true
				}
			}
			result[choice.ID] = options
			for _, selected := range choiceValues(decisions, choice) {
				if eligible[selected] {
					claimed[proficiencyChoiceKey(choice, selected)] = true
				}
			}
			delete(pending, choice.ID)
		}
	}
	return result
}

func proficiencyChoiceKey(choice character.PlanChoice, value string) string {
	switch choice.Kind {
	case character.ChoiceSkills:
		return "skill:" + canonicalSkill(value)
	case character.ChoiceTools:
		return "tool:" + value
	case character.ChoiceExpertise, character.ChoiceSkillExpertise, character.ChoiceSkillUpgrade, character.ChoiceProficiencies,
		character.ChoiceWeaponMastery, character.ChoiceLanguages, character.ChoiceSavingThrows, character.ChoiceWeapons,
		character.ChoiceArmor, character.ChoiceResistances, character.ChoiceImmunities, character.ChoiceFeat,
		character.ChoiceEnumerated, character.ChoiceASIMode, character.ChoiceAbilityBudget, character.ChoiceSize:
	}
	if skill, ok := strings.CutPrefix(value, "skill:"); ok {
		return "skill:" + canonicalSkill(skill)
	}
	return value
}
