package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Proficiency choices share one eligibility pool, including typed skills/tools
// and mixed grants. Earlier valid choices reserve their training; within a level,
// class choices and direct origins precede the feats those origins can grant.
func characterProficiencyOptions(input character.Inputs, plan Object, records Records, profile Ruleset) map[string][]any {
	pending := map[string]bool{}
	for _, group := range []string{"creationChoices", "classChoices"} {
		for _, choice := range objects(plan[group]) {
			if isProficiencyChoice(choice) {
				pending[text(choice["id"])] = true
			}
		}
	}
	result, claimed, combinedSeen := map[string][]any{}, map[string]bool{}, map[string]bool{}
	for index := 0; index < max(1, len(input.Build.Levels)) && len(pending) > 0; index++ {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:min(index+1, len(prefix.Build.Levels))]
		detached := character.Result{Issues: []character.Issue{}}
		decisions := characterDecisions(prefix, records, profile, &detached)
		prefixPlan := BuilderPlan(decisions, records, profile)
		choices, replaceable := []Object{}, []Object{}
		combinedNow := []string{}
		basePlan := cloneObjectDeep(prefixPlan)
		for _, group := range []string{"classChoices", "creationChoices"} {
			basePlan[group] = []any{}
			for _, choice := range objects(prefixPlan[group]) {
				if !isProficiencyChoice(choice) {
					// Combined training may add Expertise to an existing skill;
					// it must not disqualify that earlier proficiency choice.
					if text(choice["kind"]) == "skillExpertise" && !combinedSeen[text(choice["id"])] {
						combinedNow = append(combinedNow, text(choice["id"]))
						continue
					}
					basePlan[group] = append(values(basePlan[group]), choice)
					continue
				}
				if !pending[text(choice["id"])] {
					continue
				}
				if text(choice["changeOn"]) != "" {
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
		sheet := Hydrate(normalized, records, &profile).Sheet
		applyCharacterEffects(prefix, sheet, &detached, records)
		tools := stringsOf(object(sheet["proficiencies"])["tools"])
		for _, choice := range choices {
			if text(choice["changeOn"]) != "" {
				for _, group := range []string{"classChoices", "creationChoices"} {
					for _, combined := range objects(prefixPlan[group]) {
						if text(combined["kind"]) == "skillExpertise" {
							for _, skill := range choiceValues(decisions, combined) {
								claimed["skill:"+canonicalSkill(skill)] = true
							}
						}
					}
				}
			}
			id, options, eligible := text(choice["id"]), []any{}, map[string]bool{}
			for _, option := range objects(builderChoiceOptions(choice, sheet, records)) {
				value := text(option["id"])
				key := proficiencyChoiceKey(choice, value)
				already := claimed[key]
				if skill, ok := strings.CutPrefix(key, "skill:"); ok {
					already = already || truth(object(object(sheet["skills"])[canonicalSkill(skill)])["proficient"])
				}
				if tool, ok := strings.CutPrefix(key, "tool:"); ok {
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
					claimed[proficiencyChoiceKey(choice, selected)] = true
				}
			}
			delete(pending, id)
		}
	}
	return result
}

func isProficiencyChoice(choice Object) bool {
	kind := text(choice["kind"])
	return kind == "skills" || kind == "tools" || kind == "proficiencies"
}

func proficiencyChoiceKey(choice Object, value string) string {
	switch text(choice["kind"]) {
	case "skills":
		return "skill:" + canonicalSkill(value)
	case "tools":
		return "tool:" + value
	}
	if skill, ok := strings.CutPrefix(value, "skill:"); ok {
		return "skill:" + canonicalSkill(skill)
	}
	return value
}
