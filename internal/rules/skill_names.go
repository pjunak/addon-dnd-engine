package rules

import "strings"

// Provider records and older sheets use spaces, kebab-case and camelCase for
// the same skill. Normalize for computation without rewriting authored keys.
func canonicalSkill(value string) string {
	fold := func(text string) string {
		return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(text))
	}
	for id := range SkillAbility {
		if fold(id) == fold(value) {
			return id
		}
	}
	return value
}
