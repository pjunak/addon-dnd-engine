package rules

import (
	"sort"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Expertise must use the proficiencies available when its grant is acquired.
// Resolve earlier grants first so a duplicate later pick cannot invalidate its
// prerequisite or the earlier valid selection. No authored choices are changed.
func characterExpertiseOptions(input character.Inputs, plan Object, records Records, profile Ruleset) map[string][]any {
	pending := map[string]bool{}
	for _, group := range []string{"creationChoices", "classChoices"} {
		for _, choice := range objects(plan[group]) {
			if kind := text(choice["kind"]); kind == "expertise" || kind == "skillExpertise" {
				pending[text(choice["id"])] = true
			}
		}
	}
	result := map[string][]any{}
	claimed := map[string]bool{}
	for index := 0; index < max(1, len(input.Build.Levels)) && len(pending) > 0; index++ {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:min(index+1, len(prefix.Build.Levels))]
		detached := character.Result{Issues: []character.Issue{}}
		decisions := characterDecisions(prefix, records, profile, &detached)
		prefixPlan := BuilderPlan(decisions, records, profile)
		choices := []Object{}
		for _, group := range []string{"creationChoices", "classChoices"} {
			ordered := append([]Object(nil), objects(prefixPlan[group])...)
			sort.Slice(ordered, func(i, j int) bool { return text(ordered[i]["id"]) < text(ordered[j]["id"]) })
			for _, choice := range ordered {
				if pending[text(choice["id"])] {
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
			if text(choice["changeOn"]) != "" && index+1 < len(input.Build.Levels) {
				current := characterDecisions(input, records, profile, &detached)
				context = characterExpertiseBase(input, current, records, profile)
			}
			id := text(choice["id"])
			options := []any{}
			eligible := map[string]bool{}
			for _, option := range objects(builderChoiceOptions(choice, context, records)) {
				skill := canonicalSkill(text(option["id"]))
				if claimed[skill] || truth(object(object(context["skills"])[skill])["expertise"]) {
					continue
				}
				options = append(options, option)
				eligible[text(option["id"])] = true
			}
			result[id] = options
			for _, selected := range choiceValues(decisions, choice) {
				skill := canonicalSkill(selected)
				if eligible[selected] {
					claimed[skill] = true
				}
			}
			delete(pending, id)
		}
	}
	return result
}

func characterExpertiseBase(input character.Inputs, decisions Object, records Records, profile Ruleset) Object {
	normalized := NormalizeBuilderDecisions(decisions, records, profile)
	// Keep proficiency from combined skillExpertise choices, but strip the
	// calculated Expertise map. Hydration still applies fixed provider grants.
	normalized["skillExpertise"] = Object{}
	detached := character.Result{Issues: []character.Issue{}}
	applyCharacterAbilityEffects(input, normalized, &detached, profile, records)
	return Hydrate(normalized, records, &profile).Sheet
}
