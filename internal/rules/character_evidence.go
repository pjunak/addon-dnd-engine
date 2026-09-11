package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"sort"
	"strings"
)

type characterRecords struct {
	source   Records
	selected map[string]character.Evidence
}

type SourceIdentity struct{ PackageID, PackageGeneration, ContentRevision string }

func newCharacterRecords(source Records) *characterRecords {
	return &characterRecords{source: source, selected: map[string]character.Evidence{}}
}
func (records *characterRecords) Value(kind, id string) (json.RawMessage, bool) {
	body, ok := records.source.Value(kind, id)
	if !ok {
		return nil, false
	}
	record, valid := DecodeObject(body)
	if !valid {
		return nil, false
	}
	sum := sha256.Sum256(body)
	summary := firstText(record["description"], record["text"], record["summary"])
	if runes := []rune(summary); len(runes) > 800 {
		summary = string(runes[:800]) + "…"
	}
	facts := cloneObjectDeep(record)
	for _, key := range []string{"text", "description", "lore", "body", "entries", "fluff", "art", "images"} {
		delete(facts, key)
	}
	identity := SourceIdentity{}
	if source, ok := records.source.(interface {
		Provenance(string, string) SourceIdentity
	}); ok {
		identity = source.Provenance(kind, id)
	}
	records.selected[kind+":"+id] = character.Evidence{Reference: character.Reference{Kind: kind, ID: id}, Name: firstText(record["name"], id), Book: text(record["book"]), Hash: hex.EncodeToString(sum[:]), Summary: summary, Facts: facts, PackageID: identity.PackageID, PackageGeneration: identity.PackageGeneration, ContentRevision: identity.ContentRevision}
	return body, true
}
func (records *characterRecords) ValueByName(kind, name string) (json.RawMessage, bool) {
	return nil, false
}
func (records *characterRecords) Values(kind string) []json.RawMessage {
	return records.source.Values(kind)
}
func (records *characterRecords) evidence() []character.Evidence {
	keys := make([]string, 0, len(records.selected))
	for key := range records.selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]character.Evidence, 0, len(keys))
	for _, key := range keys {
		result = append(result, records.selected[key])
	}
	return result
}

func captureCharacterSources(sheet Object, records *characterRecords) {
	for _, feature := range objects(sheet["features"]) {
		_, _ = records.Value("feature", text(feature["id"]))
	}
	for _, resource := range objects(sheet["resources"]) {
		source := object(resource["source"])
		_, _ = records.Value(text(source["type"]), text(source["id"]))
	}

}

// Source tables belong in retained evidence. The playable projection contains
// identities and selected results, not a second copy of every progression row.
func compactCharacterProjection(sheet Object) {
	for _, key := range []string{"class", "species", "background"} {
		if record := object(sheet[key]); record != nil {
			identity := Object{}
			for _, field := range []string{"id", "name", "kind", "book"} {
				if value, exists := record[field]; exists {
					identity[field] = value
				}
			}
			sheet[key] = identity
		}
	}
}

func characterExplanations(input character.Inputs, sheet Object, evidence []character.Evidence) map[string]character.Explanation {
	result := map[string]character.Explanation{}
	sources := []character.Reference{}
	for _, entry := range evidence {
		sources = append(sources, entry.Reference)
	}
	// Numeric leaves retain their exact containing rule/table row. More specific
	// calculations below replace the generic row explanation for visible stats.
	var visit func(any, string, map[string]any)
	visit = func(value any, path string, parent map[string]any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				next := key
				if path != "" {
					next = path + "." + key
				}
				visit(child, next, value)
			}
		case Object:
			visit(map[string]any(value), path, parent)
		case []any:
			for index, child := range value {
				visit(child, fmtIndex(path, index), parent)
			}
		case float64, int:
			intValue := number(value, 0)
			terms := []character.Term{}
			for _, key := range []string{"level", "classLevel", "die", "ability", "mod", "proficient", "expertise", "formula", "scaling", "progression", "source", "base", "bonus"} {
				if val, ok := parent[key]; ok {
					terms = append(terms, character.Term{Label: key, Value: val})
				}
			}
			result[path] = character.Explanation{Label: path, Formula: "Result of the selected rule and its applicable progression row.", Value: intValue, Terms: terms, Sources: []character.Reference{}}
		}
	}
	visit(sheet, "", nil)
	makeExplanation := func(path, label, formula string, value any, terms ...character.Term) {
		result[path] = character.Explanation{Label: label, Formula: formula, Value: value, Terms: terms, Sources: sources}
	}
	derived := object(sheet["derived"])
	pb := integer(derived["proficiencyBonus"], 0)
	makeExplanation("derived.proficiencyBonus", "Proficiency bonus", "2 + floor((character level − 1) / 4)", pb, character.Term{Label: "Character level", Value: len(input.Build.Levels)})
	for _, ability := range Abilities {
		value := object(object(sheet["abilities"])[ability])
		score := value["score"]
		makeExplanation("abilities."+ability+".score", ability+" score", "min(effective ability cap, base score + granted increases)", score, character.Term{Label: "Recorded base", Value: input.Build.BaseScores[ability]}, character.Term{Label: "Granted increases", Value: value["bonus"]}, character.Term{Label: "Effective cap", Value: value["cap"]})
		makeExplanation("abilities."+ability+".mod", ability+" modifier", "floor((effective score − 10) / 2)", value["mod"], character.Term{Label: "Effective score", Value: score})
	}
	for _, group := range []string{"saves", "skills"} {
		for key, raw := range object(sheet[group]) {
			row := object(raw)
			multiplier := 0
			if truth(row["proficient"]) {
				multiplier = 1
			}
			if truth(row["expertise"]) {
				multiplier = 2
			}
			makeExplanation(group+"."+key+".total", guidanceLabel(key), "ability modifier + proficiency multiplier × proficiency bonus", row["total"], character.Term{Label: "Ability modifier", Value: row["mod"]}, character.Term{Label: "Proficiency multiplier", Value: multiplier}, character.Term{Label: "Proficiency bonus", Value: pb})
		}
	}
	hp := object(sheet["hp"])
	makeExplanation("derived.maxHp", "Maximum HP", "sum of each level's max(minimum gain, hit die result + Constitution modifier) + per-level grants", derived["maxHp"], character.Term{Label: "Level gains", Value: hp["levels"]})
	ac := object(sheet["ac"])
	terms := []character.Term{{Label: "Available formulas (highest applies)", Value: ac["candidates"]}, {Label: "Chosen formula", Value: ac["base"]}, {Label: "Shield", Value: ac["shield"]}}
	for _, key := range []string{"speciesBonus", "activeBonus", "restrictions"} {
		if value, ok := ac[key]; ok {
			terms = append(terms, character.Term{Label: key, Value: value})
		}
	}
	makeExplanation("derived.armorClass", "Armor Class", "highest eligible AC formula + shield + applicable bonuses", derived["armorClass"], terms...)
	dex := object(object(sheet["abilities"])["DEX"])["mod"]
	makeExplanation("derived.initiative", "Initiative", "Dexterity modifier + applicable feature bonuses", derived["initiative"], character.Term{Label: "Dexterity modifier", Value: dex}, character.Term{Label: "Feature bonuses", Value: integer(derived["initiative"], 0) - integer(dex, 0)})
	speedTerms := []character.Term{}
	for _, entry := range evidence {
		for _, key := range []string{"speeds", "speed", "grants", "modifiers"} {
			if fact, ok := entry.Facts[key]; ok && key != "grants" || key == "grants" && object(entry.Facts[key])["speedBonus"] != nil {
				ref := entry.Reference
				speedTerms = append(speedTerms, character.Term{Label: entry.Name + ": " + key, Value: fact, Source: &ref})
			}
		}
	}
	makeExplanation("derived.speed", "Speed", "species movement + applicable grants and active effects", derived["speed"], speedTerms...)
	attunement := object(sheet["attunement"])
	makeExplanation("attunement.limit", "Attunement capacity", "maximum of the ruleset limit and unlocked class capacities", attunement["limit"], character.Term{Label: "Class progression", Value: sheet["classes"]})
	makeExplanation("attunement.count", "Attuned items", "count of attuned inventory instances", attunement["count"], character.Term{Label: "Inventory", Value: input.Play.Inventory})
	for key, value := range object(sheet["senses"]) {
		terms := []character.Term{}
		for _, entry := range evidence {
			if senses := object(entry.Facts["senses"]); senses[key] != nil {
				ref := entry.Reference
				terms = append(terms, character.Term{Label: entry.Name, Value: senses[key], Source: &ref})
			}
			if senses := object(object(entry.Facts["grants"])["senses"]); senses[key] != nil {
				ref := entry.Reference
				terms = append(terms, character.Term{Label: entry.Name, Value: senses[key], Source: &ref})
			}
			for _, activation := range append(objects(entry.Facts["activations"]), objects(object(entry.Facts["grants"])["activations"])...) {
				for _, modifier := range objects(activation["modifiers"]) {
					if text(modifier["target"]) == "sense" && text(modifier["key"]) == key {
						ref := entry.Reference
						status := "inactive"
						if input.Play.ActiveFeatures[entry.Reference.Kind+":"+entry.Reference.ID+":"+text(activation["id"])] {
							status = "applied"
						}
						terms = append(terms, character.Term{Label: entry.Name + ": " + text(activation["condition"]), Value: modifier["value"], Source: &ref, Status: status})
					}
				}
			}
		}
		makeExplanation("senses."+key, guidanceLabel(key), "greatest applicable granted range, followed by explicit DM adjustments", value, terms...)
	}
	for _, resource := range objects(sheet["resources"]) {
		key := text(resource["key"])
		makeExplanation("resources."+key+".max", firstText(resource["name"], key), "capacity from the granting rule's level/proficiency/ability progression", resource["max"], character.Term{Label: "Resource rule", Value: resource})
		makeExplanation("resources."+key+".remaining", firstText(resource["name"], key)+" remaining", "effective capacity − recorded spent uses", resource["remaining"], character.Term{Label: "Capacity", Value: resource["max"]}, character.Term{Label: "Spent", Value: resource["spent"]})
	}
	for index, caster := range objects(object(sheet["spellcasting"])["perClass"]) {
		ability := text(caster["ability"])
		modifier := object(object(sheet["abilities"])[ability])["mod"]
		prefix := fmtIndex("spellcasting.perClass", index)
		makeExplanation(prefix+".saveDC", "Spell save DC", "8 + proficiency bonus + spellcasting ability modifier", caster["saveDC"], character.Term{Label: "Base", Value: 8}, character.Term{Label: "Proficiency bonus", Value: pb}, character.Term{Label: ability + " modifier", Value: modifier})
		makeExplanation(prefix+".spellAttack", "Spell attack", "proficiency bonus + spellcasting ability modifier", caster["spellAttack"], character.Term{Label: "Proficiency bonus", Value: pb}, character.Term{Label: ability + " modifier", Value: modifier})
	}
	for index, weapon := range objects(sheet["weapons"]) {
		proficiency := 0
		if truth(weapon["proficient"]) {
			proficiency = pb
		}
		ability := integer(weapon["attackBonus"], 0) - proficiency
		prefix := fmtIndex("weapons", index)
		makeExplanation(prefix+".attackBonus", "Weapon attack", "eligible weapon ability modifier + proficiency bonus when proficient", weapon["attackBonus"], character.Term{Label: "Weapon ability modifier", Value: ability}, character.Term{Label: "Applicable proficiency", Value: proficiency})
		makeExplanation(prefix+".damage", "Weapon damage", "weapon damage dice + eligible ability modifier", weapon["damage"], character.Term{Label: "Ability modifier", Value: ability}, character.Term{Label: "Damage type", Value: weapon["damageType"]})
		makeExplanation(prefix+".versatileDamage", "Versatile damage", "two-handed weapon dice + eligible ability modifier", weapon["versatileDamage"], character.Term{Label: "Ability modifier", Value: ability})
	}
	maximum := integer(derived["maxHp"], 0)
	zero := 0
	result["play.hp"] = character.Explanation{Label: "Current HP", Formula: "Recorded play value between zero and effective maximum HP.", Value: input.Play.HP, Minimum: &zero, Maximum: &maximum, Terms: []character.Term{{Label: "Effective maximum", Value: maximum}}, Sources: []character.Reference{}}
	result["play.temporaryHp"] = character.Explanation{Label: "Temporary HP", Formula: "Separate non-negative play pool; it does not increase maximum HP.", Value: input.Play.TemporaryHP, Minimum: &zero, Terms: []character.Term{}, Sources: []character.Reference{}}
	for path, explanation := range result {
		if path == "derived.speed" || strings.HasPrefix(path, "senses.") {
			explanation.Unit = "ft"
			result[path] = explanation
		}
	}
	return result
}

func fmtIndex(path string, index int) string { return path + "." + jsonNumber(index) }
