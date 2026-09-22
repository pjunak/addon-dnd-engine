package rules

import (
	"sort"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Conditional repetition is declared by a single explicit, finite choice.
// Unknown policies cannot be interpreted as unrestricted repetition.
func conditionalFeatChoice(record Object) Object {
	policy := object(record["repeatable"])
	if len(policy) != 2 || text(policy["by"]) != "choice" || text(policy["choice"]) == "" {
		return nil
	}
	var match Object
	for _, choice := range objects(object(record["grants"])["choices"]) {
		if text(choice["id"]) != text(policy["choice"]) {
			continue
		}
		if match != nil || text(choice["type"]) != "enumerated" || choice["count"] != nil && (!positivePredicateNumber(choice["count"]) || integer(choice["count"], 0) != 1) {
			return nil
		}
		pool := stringsOf(choice["from"])
		if len(pool) == 0 || len(pool) != len(values(choice["from"])) || contains(pool, "") || len(unique(pool)) != len(pool) {
			return nil
		}
		match = choice
	}
	return match
}

func featRepetitionAllowed(record Object, count int) bool {
	if count <= 1 {
		return true
	}
	if object(record["repeatable"]) != nil {
		choice := conditionalFeatChoice(record)
		return choice != nil && count <= len(stringsOf(choice["from"]))
	}
	return truth(record["repeatable"])
}

// Resolve acquisition order from level prefixes, including nested grants and
// multiclass advancements. Same-level owners use stable IDs, never input order.
// A later invalid pick must not reserve a value or invalidate its earlier owner.
func characterConditionalFeatOptions(input character.Inputs, plan Object, records Records, profile Ruleset) map[string][]any {
	pending := map[string]bool{}
	for _, choice := range objects(plan["creationChoices"]) {
		if truth(choice["distinctAcrossAcquisitions"]) {
			pending[text(choice["id"])] = true
		}
	}
	result, claimed := map[string][]any{}, map[string]map[string]bool{}
	for index := 0; index < max(1, len(input.Build.Levels)) && len(pending) > 0; index++ {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:min(index+1, len(prefix.Build.Levels))]
		detached := character.Result{}
		decisions := characterDecisions(prefix, records, profile, &detached)
		choices := objects(BuilderPlan(decisions, records, profile)["creationChoices"])
		sort.Slice(choices, func(i, j int) bool {
			return text(object(choices[i]["acquisition"])["id"]) < text(object(choices[j]["acquisition"])["id"])
		})
		for _, choice := range choices {
			id := text(choice["id"])
			if !pending[id] {
				continue
			}
			featID := text(object(choice["source"])["id"])
			if claimed[featID] == nil {
				claimed[featID] = map[string]bool{}
			}
			options, eligible := []any{}, map[string]bool{}
			for _, option := range objects(builderChoiceOptions(choice, nil, records)) {
				value := text(option["id"])
				if !claimed[featID][value] {
					options = append(options, option)
					eligible[value] = true
				}
			}
			result[id] = options
			for _, value := range choiceValues(decisions, choice) {
				if eligible[value] {
					claimed[featID][value] = true
				}
			}
			delete(pending, id)
		}
	}
	return result
}
