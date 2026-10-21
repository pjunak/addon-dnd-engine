package rules

import (
	"sort"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Expertise must use the proficiencies available when its grant is acquired.
// Resolve earlier grants first so a duplicate later pick cannot invalidate its
// prerequisite or the earlier valid selection. No authored choices are changed.
func characterExpertiseOptions(input character.Inputs, plan character.Plan, records Records, profile Ruleset) map[string][]character.Option {
	pending := map[string]bool{}
	for _, group := range [][]character.PlanChoice{plan.CreationChoices, plan.ClassChoices} {
		for _, choice := range group {
			if choice.Kind == character.ChoiceExpertise || choice.Kind == character.ChoiceSkillExpertise {
				pending[choice.ID] = true
			}
		}
	}
	result := map[string][]character.Option{}
	claimed := map[string]bool{}
	for index := 0; index < max(1, len(input.Build.Levels)) && len(pending) > 0; index++ {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:min(index+1, len(prefix.Build.Levels))]
		detached := character.Result{Issues: []character.Issue{}}
		decisions := characterDecisions(prefix, records, profile, &detached)
		prefixPlan := BuilderPlan(decisions, records, profile)
		choices := []character.PlanChoice{}
		for _, group := range [][]character.PlanChoice{prefixPlan.CreationChoices, prefixPlan.ClassChoices} {
			ordered := append([]character.PlanChoice(nil), group...)
			sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
			for _, choice := range ordered {
				if pending[choice.ID] {
					choices = append(choices, choice)
				}
			}
		}
		if len(choices) == 0 {
			continue
		}
		sheet := characterExpertiseBase(prefix, decisions, records, profile)
		for _, choice := range choices {
			context := sheet
			// A source-declared replacement (for example on a rest) uses today's
			// proficiencies; permanently acquired choices use the level prefix above.
			if choice.ChangeOn != "" && index+1 < len(input.Build.Levels) {
				current := characterDecisions(input, records, profile, &detached)
				context = characterExpertiseBase(input, current, records, profile)
			}
			options := []character.Option{}
			eligible := map[string]bool{}
			for _, option := range builderChoiceOptions(choice, context, records) {
				skill := canonicalSkill(option.ID)
				if claimed[skill] || context.Skills[skill].Expertise {
					continue
				}
				options = append(options, option)
				eligible[option.ID] = true
			}
			result[choice.ID] = options
			for _, selected := range choiceValues(decisions, choice) {
				if eligible[selected] {
					claimed[canonicalSkill(selected)] = true
				}
			}
			delete(pending, choice.ID)
		}
	}
	return result
}

func characterExpertiseBase(input character.Inputs, decisions Object, records Records, profile Ruleset) *character.Calculation {
	normalized := NormalizeBuilderDecisions(decisions, records, profile)
	// Keep proficiency from combined skillExpertise choices, but strip the
	// calculated Expertise map. Hydration still applies fixed provider grants.
	normalized["skillExpertise"] = Object{}
	detached := character.Result{Issues: []character.Issue{}}
	applyCharacterAbilityEffects(input, normalized, &detached, profile, records)
	return Hydrate(normalized, records, profile).Sheet
}
