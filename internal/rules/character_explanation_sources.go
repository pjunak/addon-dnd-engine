package rules

import (
	"sort"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Keep the full evidence once, and link calculations to their own sources.
// Repeating every selected record on every statistic grows with both the number
// of features and statistics and can prevent an otherwise legal sheet saving.
func scopeCharacterExplanationSources(input character.Inputs, decisions, sheet Object, evidence []character.Evidence, explanations map[string]character.Explanation) {
	retained := map[character.Reference]bool{}
	for _, entry := range evidence {
		retained[entry.Reference] = true
	}
	sourceRef := func(source Object) character.Reference {
		return character.Reference{Kind: text(source["type"]), ID: text(source["id"])}
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
				refs = append(refs, sourceRef(object(grant["source"])))
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
		if characterTrainingFacts(Object(entry.Facts)) {
			trainingSources = append(trainingSources, entry.Reference)
		}
		if object(entry.Facts["grants"])["hpPerLevel"] != nil {
			hpSources = append(hpSources, entry.Reference)
		}
	}
	for _, current := range objects(sheet["classes"]) {
		hpSources = append(hpSources, classSources(text(current["classId"]))...)
	}
	set("derived.maxHp", hpSources)
	for _, group := range []string{"skills", "saves"} {
		for key, raw := range object(sheet[group]) {
			row := object(raw)
			ability := key
			if group == "skills" {
				ability = text(row["ability"])
			}
			refs := abilitySources(ability)
			if truth(row["proficient"]) {
				refs = append(refs, trainingSources...)
			}
			set(group+"."+key+".total", refs)
		}
	}
	casters := objects(object(sheet["spellcasting"])["perClass"])
	for index, caster := range casters {
		refs := append(classSources(text(caster["classId"])), abilitySources(text(caster["ability"]))...)
		set(fmtIndex("spellcasting.perClass", index)+".saveDC", refs)
		set(fmtIndex("spellcasting.perClass", index)+".spellAttack", refs)
	}
	for _, resource := range objects(sheet["resources"]) {
		source := object(resource["source"])
		refs := []character.Reference{sourceRef(source)}
		switch text(source["type"]) {
		case "pactMagic":
			refs = classSources(text(source["id"]))
		case "spellcasting":
			for _, caster := range casters {
				if object(caster["pact"]) == nil {
					refs = append(refs, classSources(text(caster["classId"]))...)
				}
			}
		case "class":
			if text(resource["kind"]) == "hitdice" {
				for _, current := range objects(sheet["classes"]) {
					if text(current["hitDie"]) == text(resource["die"]) {
						refs = append(refs, character.Reference{Kind: "class", ID: text(current["classId"])})
					}
				}
			}
		}
		for _, field := range []string{"max", "remaining"} {
			set("resources."+text(resource["key"])+"."+field, refs)
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
	if contains([]string{"skillProficiency", "expertise", "skillExpertise", "proficiency", "savingThrowProficiency"}, text(record["type"])) {
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
