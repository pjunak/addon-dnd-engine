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
func characterConditionalFeatOptions(input character.Inputs, plan character.Plan, records Records, profile Ruleset) map[string][]character.Option {
	pending := map[string]bool{}
	for _, choice := range plan.CreationChoices {
		if choice.DistinctAcrossAcquisitions {
			pending[choice.ID] = true
		}
	}
	result, claimed := map[string][]character.Option{}, map[string]map[string]bool{}
	for index := 0; index < max(1, len(input.Build.Levels)) && len(pending) > 0; index++ {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:min(index+1, len(prefix.Build.Levels))]
		detached := character.Result{}
		decisions := characterDecisions(prefix, records, profile, &detached)
		choices := BuilderPlan(decisions, records, profile).CreationChoices
		acquisition := func(choice character.PlanChoice) string {
			if choice.Acquisition == nil {
				return ""
			}
			return choice.Acquisition.ID
		}
		sort.Slice(choices, func(i, j int) bool { return acquisition(choices[i]) < acquisition(choices[j]) })
		for _, choice := range choices {
			id := choice.ID
			if !pending[id] {
				continue
			}
			featID := ""
			if choice.Source != nil {
				featID = choice.Source.ID
			}
			if claimed[featID] == nil {
				claimed[featID] = map[string]bool{}
			}
			options, eligible := []character.Option{}, map[string]bool{}
			for _, option := range builderChoiceOptions(choice, nil, records) {
				if !claimed[featID][option.ID] {
					options = append(options, option)
					eligible[option.ID] = true
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
