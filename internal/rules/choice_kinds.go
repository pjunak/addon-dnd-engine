package rules

import "github.com/pjunak/addon-dnd-engine/character"

// choiceKindSpec is how a choice kind is labelled when its data has no
// prompt, which decisions list its picks join, and which options it offers
// when its data lists none.
type choiceKindSpec struct {
	heading  string
	resolved string
	pool     func(Records) []string
}

func skillPool(Records) []string { return sortedKeys(SkillAbility) }

func catalogPool(kind string) func(Records) []string {
	return func(records Records) []string {
		ids := []string{}
		for _, record := range recordCatalog(records, kind) {
			ids = append(ids, text(record["id"]))
		}
		return ids
	}
}

// choiceKindSpecs is the table of choice kinds. Headings are labelKeys that
// clients translate; {0} is the choice's source.
var choiceKindSpecs = map[character.ChoiceKind]choiceKindSpec{
	character.ChoiceSkills:         {"Skills · {0}", "skillProficiencies", skillPool},
	character.ChoiceExpertise:      {"Expertise · {0}", "", skillPool},
	character.ChoiceSkillExpertise: {"Expertise · {0}", "", skillPool},
	character.ChoiceSkillUpgrade:   {"Skill or expertise · {0}", "skillUpgrades", skillPool},
	character.ChoiceTools:          {"Tools · {0}", "toolProficiencies", nil},
	character.ChoiceProficiencies:  {"Proficiencies · {0}", "", nil},
	character.ChoiceWeaponMastery:  {"Weapon Mastery · {0}", "weaponMasteryChoices", catalogPool("weapon")},
	character.ChoiceLanguages:      {"Languages · {0}", "languageProficiencies", nil},
	character.ChoiceSavingThrows:   {"Saving throws · {0}", "saveProficiencies", nil},
	character.ChoiceWeapons:        {"Weapon training · {0}", "weaponProficiencies", nil},
	character.ChoiceArmor:          {"Armor training · {0}", "armorProficiencies", nil},
	character.ChoiceResistances:    {"Damage resistance · {0}", "damageResistances", nil},
	character.ChoiceImmunities:     {"Condition immunity · {0}", "conditionImmunities", nil},
	character.ChoiceFeat:           {"Feat · {0}", "", catalogPool("feat")},
	character.ChoiceEnumerated:     {},
	character.ChoiceASIMode:        {},
	character.ChoiceAbilityBudget:  {},
	character.ChoiceSize:           {},
}

// choiceTypeKinds maps a record choice's type (grants.choices[].type) to
// its kind. Other types are feat choices when they name a category and list
// no options, and enumerated choices otherwise.
var choiceTypeKinds = map[string]character.ChoiceKind{
	"skillProficiency": character.ChoiceSkills, "expertise": character.ChoiceExpertise,
	"skillExpertise": character.ChoiceSkillExpertise, "skillProficiencyOrExpertise": character.ChoiceSkillUpgrade,
	"toolProficiency": character.ChoiceTools, "proficiency": character.ChoiceProficiencies,
	"weaponMastery": character.ChoiceWeaponMastery, "language": character.ChoiceLanguages,
	"savingThrowProficiency": character.ChoiceSavingThrows, "weaponProficiency": character.ChoiceWeapons,
	"armorProficiency": character.ChoiceArmor, "damageResistance": character.ChoiceResistances,
	"conditionImmunity": character.ChoiceImmunities, "feat": character.ChoiceFeat,
}

// isProficiencyChoice says whether a choice draws from the shared skill and
// tool proficiency pool.
func isProficiencyChoice(choice character.PlanChoice) bool {
	return choice.Kind == character.ChoiceSkills || choice.Kind == character.ChoiceTools || choice.Kind == character.ChoiceProficiencies
}
