package rules

import (
	"sort"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Mixed proficiency grants must offer useful, distinct training. As with
// Expertise, earlier valid acquisitions reserve their choices without letting
// a later selection invalidate them or qualify an earlier grant.
func characterProficiencyOptions(input character.Inputs, plan Object, records Records, profile Ruleset) map[string][]any {
	pending := map[string]bool{}
	for _, group := range []string{"creationChoices", "classChoices"} {
		for _, choice := range objects(plan[group]) {
			if text(choice["kind"]) == "proficiencies" {
				pending[text(choice["id"])] = true
			}
		}
	}
	result, claimed := map[string][]any{}, map[string]bool{}
	for index := 0; index < max(1, len(input.Build.Levels)) && len(pending) > 0; index++ {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:min(index+1, len(prefix.Build.Levels))]
		detached := character.Result{Issues: []character.Issue{}}
		decisions := characterDecisions(prefix, records, profile, &detached)
		prefixPlan := BuilderPlan(decisions, records, profile)
		choices := []Object{}
		basePlan := cloneObjectDeep(prefixPlan)
		for _, group := range []string{"creationChoices", "classChoices"} {
			ordered := append([]Object{}, objects(prefixPlan[group])...)
			sort.Slice(ordered, func(i, j int) bool { return text(ordered[i]["id"]) < text(ordered[j]["id"]) })
			basePlan[group] = []any{}
			for _, choice := range ordered {
				if text(choice["kind"]) != "proficiencies" {
					basePlan[group] = append(values(basePlan[group]), choice)
					continue
				}
				if pending[text(choice["id"])] {
					choices = append(choices, choice)
				}
			}
		}
		if len(choices) == 0 {
			continue
		}
		normalized := NormalizeBuilderDecisions(decisions, records, profile)
		for key, value := range resolveBuilderChoices(decisions, basePlan, records) {
			normalized[key] = value
		}
		sheet := Hydrate(normalized, records, &profile).Sheet
		applyCharacterEffects(prefix, sheet, &detached, records)
		tools := stringsOf(object(sheet["proficiencies"])["tools"])
		for _, choice := range choices {
			id, options, eligible := text(choice["id"]), []any{}, map[string]bool{}
			for _, option := range objects(builderChoiceOptions(choice, sheet, records)) {
				value := text(option["id"])
				already := claimed[value]
				if skill, ok := strings.CutPrefix(value, "skill:"); ok {
					already = already || truth(object(object(sheet["skills"])[canonicalSkill(skill)])["proficient"])
				}
				if tool, ok := strings.CutPrefix(value, "tool:"); ok {
					already = already || contains(tools, tool)
				}
				if !already {
					options = append(options, option)
					eligible[value] = true
				}
			}
			result[id] = options
			for _, selected := range choiceValues(decisions, choice) {
				if eligible[selected] {
					claimed[selected] = true
				}
			}
			delete(pending, id)
		}
	}
	return result
}
