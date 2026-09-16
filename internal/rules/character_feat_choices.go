package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// A historical unscoped selection can be attached only when its owner is
// unambiguous. The detached result carries the canonical ID; a load never
// writes it. Ambiguous input remains visible and blocks saving rather than
// being copied to every acquisition or silently withdrawn.
func normalizeCharacterFeatChoices(input *character.Inputs, records Records, profile Ruleset, result *character.Result) {
	hasUnscoped := false
	for _, choice := range input.Build.Choices {
		if parts := strings.Split(choice.ID, ":"); len(parts) >= 3 && parts[0] == "feat" {
			record := recordByID(records, "feat", parts[1])
			hasUnscoped = hasUnscoped || truth(record["repeatable"]) || object(record["repeatable"]) != nil
		}
	}
	if !hasUnscoped {
		return
	}
	decisions := characterDecisions(*input, records, profile, result)
	plan := BuilderPlan(decisions, records, profile)
	aliases := map[string][]string{}
	for _, descriptor := range objects(plan["creationChoices"]) {
		if alias := text(descriptor["legacyId"]); alias != "" {
			aliases[alias] = append(aliases[alias], text(descriptor["id"]))
		}
	}
	for index, choice := range input.Build.Choices {
		if ids := aliases[choice.ID]; len(ids) == 1 {
			input.Build.Choices[index].ID = ids[0]
		}
	}
}

func ambiguousCharacterFeatChoice(plan Object, id string) bool {
	count := 0
	for _, descriptor := range objects(plan["creationChoices"]) {
		if text(descriptor["legacyId"]) == id {
			count++
		}
	}
	return count > 1
}
