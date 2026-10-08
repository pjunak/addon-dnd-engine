package rules

import "strings"

var (
	skillNameFolder = strings.NewReplacer(" ", "", "-", "", "_", "")
	foldedSkills    = func() map[string]string {
		result := make(map[string]string, len(SkillAbility))
		for id := range SkillAbility {
			result[foldSkillName(id)] = id
		}
		return result
	}()
)

func foldSkillName(value string) string {
	return strings.ToLower(skillNameFolder.Replace(value))
}

// Provider records and older sheets use spaces, kebab-case and camelCase for
// the same skill. Normalize for computation without rewriting authored keys.
func canonicalSkill(value string) string {
	if id, ok := foldedSkills[foldSkillName(value)]; ok {
		return id
	}
	return value
}
