package rules

import (
	"sort"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Keep the full evidence once, and link calculations to their own sources.
// Repeating every selected record on every statistic grows with both the number
// of features and statistics and can prevent an otherwise legal sheet saving.
func scopeCharacterExplanationSources(input character.Inputs, decisions Object, sheet *character.Calculation, evidence []character.Evidence, explanations map[string]character.Explanation) {
	retained := map[character.Reference]bool{}
	for _, entry := range evidence {
		retained[entry.Reference] = true
	}

	set := func(path string, candidates []character.Reference) {
		explanation, exists := explanations[path]
		if !exists {
			return
		}
		for _, term := range explanation.Terms {
			if term.Source != nil {
				candidates = append(candidates, *term.Source)
			}
		}
		explanation.Sources = []character.Reference{}
		for _, ref := range uniqueCharacterReferences(candidates) {
			if retained[ref] {
				explanation.Sources = append(explanation.Sources, ref)
			}
		}
		explanations[path] = explanation
	}
	abilitySources := func(ability string) []character.Reference {
		refs := []character.Reference{}
		for _, grant := range objects(decisions["abilityGrants"]) {
			if integer(object(grant["assign"])[ability], 0) != 0 {
				refs = append(refs, grantSourceReference(sourceFromObject(object(grant["source"]))))
			}
		}
		for _, term := range explanations["abilities."+ability+".score"].Terms {
			if term.Source != nil {
				refs = append(refs, *term.Source)
			}
		}
		return refs
	}
	classSources := func(id string) []character.Reference {
		refs := []character.Reference{{Kind: "class", ID: id}}
		if subclass := input.Build.Subclasses[id]; subclass != "" {
			refs = append(refs, character.Reference{Kind: "subclass", ID: subclass})
		}
		return refs
	}
	for _, ability := range Abilities {
		set("abilities."+ability+".score", abilitySources(ability))
		set("abilities."+ability+".mod", abilitySources(ability))
	}
	trainingSources := []character.Reference{}
	hpSources := abilitySources("CON")
	for _, entry := range evidence {
		facts := factsOf(entry)
		if characterTrainingFacts(facts) {
			trainingSources = append(trainingSources, entry.Reference)
		}
		if object(facts["grants"])["hpPerLevel"] != nil {
			hpSources = append(hpSources, entry.Reference)
		}
	}
	for _, current := range sheet.Classes {
		hpSources = append(hpSources, classSources(current.ClassID)...)
	}
	set("derived.maxHp", hpSources)
	checkSources := func(path, ability string, proficient bool) {
		refs := abilitySources(ability)
		if proficient {
			refs = append(refs, trainingSources...)
		}
		set(path, refs)
	}
	for key, row := range sheet.Skills {
		checkSources("skills."+key+".total", row.Ability, row.Proficient)
	}
	for key, row := range sheet.Saves {
		checkSources("saves."+key+".total", key, row.Proficient)
	}
	set("currencyWorth.value", nil)
	for index := range sheet.Weapons {
		set(fmtIndex("weapons", index)+".reach", nil)
		set(fmtIndex("weapons", index)+".range", nil)
	}
	casters := sheet.Spellcasting.PerClass
	for index, caster := range casters {
		refs := append(classSources(caster.ClassID), abilitySources(caster.Ability)...)
		set(fmtIndex("spellcasting.perClass", index)+".saveDC", refs)
		set(fmtIndex("spellcasting.perClass", index)+".spellAttack", refs)
		set(fmtIndex("spellcasting.perClass", index)+".preparedLimit", classSources(caster.ClassID))
	}
	for _, resource := range sheet.Resources {
		source := resource.Source
		refs := []character.Reference{grantSourceReference(source)}
		switch source.Type {
		case character.SourcePactMagic:
			refs = classSources(source.ID)
		case character.SourceSpellcasting:
			for _, caster := range casters {
				if caster.Pact == nil {
					refs = append(refs, classSources(caster.ClassID)...)
				}
			}
		case character.SourceClass:
			if resource.Kind == character.ResourceHitDice {
				for _, current := range sheet.Classes {
					if current.HitDie == resource.Die {
						refs = append(refs, character.Reference{Kind: "class", ID: current.ClassID})
					}
				}
			}
		case character.SourceSubclass, character.SourceFeature, character.SourceSpecies, character.SourceBackground,
			character.SourceFeat, character.SourceItem, character.SourceArmor, character.SourceCondition,
			character.SourceSpell, character.SourceASI, character.SourceAbility:
		}
		for _, field := range []string{"max", "remaining"} {
			set("resources."+resource.Key+"."+field, refs)
		}
	}
}

// Training can be declared directly, through a choice, or in a source package.
// Only selected records reach this function; unrelated feature prose is not
// evidence for a skill merely because that feature belongs to the same class.
func characterTrainingFacts(record Object) bool {
	for _, key := range []string{"savingThrows", "skillProficiencies", "skills", "expertise"} {
		if record[key] != nil {
			return true
		}
	}
	if contains([]string{"skillProficiency", "expertise", "skillExpertise", "skillProficiencyOrExpertise", "proficiency", "savingThrowProficiency"}, text(record["type"])) {
		return true
	}
	for _, value := range record {
		if nested := object(value); nested != nil && characterTrainingFacts(nested) {
			return true
		}
		for _, nested := range objects(value) {
			if characterTrainingFacts(nested) {
				return true
			}
		}
	}
	return false
}

func uniqueCharacterReferences(refs []character.Reference) []character.Reference {
	seen := map[character.Reference]bool{}
	result := []character.Reference{}
	for _, ref := range refs {
		if ref.Kind != "" && ref.ID != "" && !seen[ref] {
			seen[ref] = true
			result = append(result, ref)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			return result[i].ID < result[j].ID
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}
