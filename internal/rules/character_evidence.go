package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	// Evidence depends only on the record and its source, so reads after the
	// first need not rebuild it.
	if _, recorded := records.selected[kind+":"+id]; recorded {
		return body, true
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

// recordObject and recordView record evidence, then lend the evaluation's
// shared, read-only decoded record instead of decoding the body again.
func (records *characterRecords) recordObject(kind, id string) Object {
	return records.recordView(kind, id)
}
func (records *characterRecords) recordView(kind, id string) Object {
	if _, ok := records.Value(kind, id); !ok {
		return nil
	}
	return recordView(records.source, kind, id)
}
func (records *characterRecords) Values(kind string) []json.RawMessage {
	return records.source.Values(kind)
}
func (records *characterRecords) recordCatalog(kind string) []Object {
	return recordCatalog(records.source, kind)
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
		ref := grantSourceReference(object(resource["source"]))
		_, _ = records.Value(ref.Kind, ref.ID)
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

func characterExplanations(input character.Inputs, sheet Object, evidence []character.Evidence, profile Ruleset) map[string]character.Explanation {
	result := map[string]character.Explanation{}
	if size := object(sheet["derived"])["size"]; size != nil {
		source := character.Reference{Kind: "species", ID: input.Build.Species}
		result["derived.size"] = character.Explanation{
			Label: "Size", Formula: "Size from the selected species and its explicit choice, when required.", Value: size,
			Terms: []character.Term{{Label: "Species size", Value: size, Source: &source}}, Sources: []character.Reference{source},
		}
	}
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
			terms := []character.Term{{Label: "Ability modifier", Value: row["mod"]}, {Label: "Proficiency multiplier", Value: multiplier}, {Label: "Proficiency bonus", Value: pb}}
			terms = append(terms, bonusTerms(objects(row["bonuses"]))...)
			makeExplanation(group+"."+key+".total", guidanceLabel(key), "ability modifier + proficiency multiplier × proficiency bonus + feature bonuses", row["total"], terms...)
		}
	}
	hp := object(sheet["hp"])
	hpTerms := []character.Term{{Label: "Level gains", Value: hp["levels"]}}
	for _, bonus := range objects(hp["fixedBonuses"]) {
		ref := grantSourceReference(object(bonus["source"]))
		hpTerms = append(hpTerms, character.Term{Label: text(bonus["name"]), Value: bonus["value"], Source: &ref})
	}
	makeExplanation("derived.maxHp", "Maximum HP", "sum of each level's max(minimum gain, hit die result + Constitution modifier) + per-level grants + fixed grants", derived["maxHp"], hpTerms...)
	ac := object(sheet["ac"])
	terms := []character.Term{{Label: "Available formulas (highest applies)", Value: ac["candidates"]}, {Label: "Chosen formula", Value: ac["base"]}, {Label: "Shield", Value: ac["shield"]}}
	for _, key := range []string{"speciesBonus", "activeBonus", "restrictions"} {
		if value, ok := ac[key]; ok {
			terms = append(terms, character.Term{Label: key, Value: value})
		}
	}
	for _, bonus := range objects(ac["bonuses"]) {
		ref := grantSourceReference(object(bonus["source"]))
		terms = append(terms, character.Term{Label: text(bonus["name"]), Value: Object{"bonus": bonus["value"], "requires": bonus["requires"]},
			Source: &ref, Status: text(bonus["status"])})
	}
	makeExplanation("derived.armorClass", "Armor Class", "highest eligible AC formula + shield + applicable bonuses", derived["armorClass"], terms...)
	dex := object(object(sheet["abilities"])["DEX"])["mod"]
	makeExplanation("derived.initiative", "Initiative", "Dexterity modifier + applicable feature bonuses", derived["initiative"], append([]character.Term{{Label: "Dexterity modifier", Value: dex}, {Label: "Feature bonuses", Value: integer(derived["initiative"], 0) - integer(dex, 0)}}, bonusTerms(objects(sheet["initiativeBonuses"]))...)...)
	speedTerms := []character.Term{}
	for _, entry := range evidence {
		for _, key := range []string{"speeds", "speed", "grants", "modifiers"} {
			if fact, ok := entry.Facts[key]; ok && key != "grants" || key == "grants" && object(entry.Facts[key])["speedBonus"] != nil {
				ref := entry.Reference
				speedTerms = append(speedTerms, character.Term{Label: entry.Name + ": " + key, Value: fact, Source: &ref})
			}
		}
	}
	speedTerms = append(speedTerms, bonusTerms(objects(sheet["speedBonuses"]))...)
	makeExplanation("derived.speed", "Speed", "species movement + applicable grants and active effects", derived["speed"], speedTerms...)
	attunement := object(sheet["attunement"])
	makeExplanation("attunement.limit", "Attunement capacity", "maximum of the ruleset limit and unlocked class capacities", attunement["limit"], character.Term{Label: "Class progression", Value: sheet["classes"]})
	makeExplanation("attunement.count", "Attuned items", "count of attuned inventory instances", attunement["count"], character.Term{Label: "Inventory", Value: input.Play.Inventory})
	for key, value := range object(sheet["senses"]) {
		terms := []character.Term{}
		for _, entry := range evidence {
			ref := entry.Reference
			grants := object(entry.Facts["grants"])
			if senses := object(entry.Facts["senses"]); senses[key] != nil {
				terms = append(terms, character.Term{Label: entry.Name, Value: senses[key], Source: &ref})
			}
			if senses := object(grants["senses"]); senses[key] != nil {
				terms = append(terms, character.Term{Label: entry.Name, Value: senses[key], Source: &ref})
			}
			if bonuses := object(grants["senseBonuses"]); bonuses[key] != nil {
				terms = append(terms, character.Term{Label: entry.Name, Value: Object{"add": bonuses[key]}, Source: &ref})
			}
			// Items toggle only their allowlisted grants, never record-level
			// activations.
			activations := objects(grants["activations"])
			if ref.Kind != "magic-item" {
				activations = append(objects(entry.Facts["activations"]), activations...)
			}
			for _, activation := range activations {
				for _, modifier := range objects(activation["modifiers"]) {
					target := text(modifier["target"])
					if target != "sense" && target != "senseBonus" || text(modifier["key"]) != key {
						continue
					}
					status := "inactive"
					for _, current := range objects(sheet["activations"]) {
						if grantSourceReference(object(current["source"])) == ref && text(current["id"]) == text(activation["id"]) && truth(current["active"]) {
							status = "applied"
						}
					}
					amount := modifier["value"]
					if target == "senseBonus" {
						amount = Object{"add": modifier["add"]}
					}
					terms = append(terms, character.Term{Label: entry.Name + ": " + text(activation["condition"]), Value: amount, Source: &ref, Status: status})
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
		makeExplanation(prefix+".saveDC", "Spell save DC", "base + proficiency bonus + spellcasting ability modifier", caster["saveDC"], character.Term{Label: "Base", Value: profile.Constants.SpellSaveDCBase}, character.Term{Label: "Proficiency bonus", Value: pb}, character.Term{Label: ability + " modifier", Value: modifier})
		makeExplanation(prefix+".spellAttack", "Spell attack", "proficiency bonus + spellcasting ability modifier", caster["spellAttack"], character.Term{Label: "Proficiency bonus", Value: pb}, character.Term{Label: ability + " modifier", Value: modifier})
		makeExplanation(prefix+".preparedLimit", "Prepared spells", "the class table's prepared spells at this class level", caster["preparedLimit"], character.Term{Label: "Class level", Value: caster["level"]}, character.Term{Label: "Prepared", Value: caster["preparedCount"]})
	}
	for index, weapon := range objects(sheet["weapons"]) {
		proficiency := 0
		if truth(weapon["proficient"]) {
			proficiency = pb
		}
		abilityName := text(weapon["ability"])
		ability := integer(object(object(sheet["abilities"])[abilityName])["mod"], 0)
		modifiers := make([]character.Term, 0)
		for _, row := range objects(weapon["modifiers"]) {
			ref := grantSourceReference(object(row["source"]))
			value := cloneObject(row)
			delete(value, "name")
			delete(value, "source")
			modifiers = append(modifiers, character.Term{Label: text(row["name"]), Value: value, Status: "applied", Source: &ref})
		}
		prefix := fmtIndex("weapons", index)
		abilityTerm := character.Term{Label: abilityName + " modifier", Value: ability}
		makeExplanation(prefix+".attackBonus", "Weapon attack", "attack ability modifier + proficiency bonus when proficient + attack bonuses", weapon["attackBonus"], append([]character.Term{abilityTerm, {Label: "Applicable proficiency", Value: proficiency}}, modifiers...)...)
		makeExplanation(prefix+".damage", "Weapon damage", "largest eligible damage die + attack ability modifier + damage bonuses", weapon["damage"], append([]character.Term{abilityTerm, {Label: "Damage type", Value: weapon["damageType"]}}, modifiers...)...)
		makeExplanation(prefix+".versatileDamage", "Versatile damage", "two-handed weapon dice + attack ability modifier + damage bonuses", weapon["versatileDamage"], append([]character.Term{abilityTerm}, modifiers...)...)
		var record *character.Reference
		if ref := text(weapon["ref"]); ref != "" {
			record = &character.Reference{Kind: "weapon", ID: ref}
		}
		if reach, ok := weapon["reach"]; ok && profile.Constants.MeleeReach != nil {
			terms := []character.Term{{Label: "Melee reach", Value: profile.Constants.MeleeReach.Base}}
			if contains(stringsOf(weapon["properties"]), "reach") {
				terms = append(terms, character.Term{Label: "Reach property", Value: profile.Constants.MeleeReach.ReachBonus, Source: record})
			}
			makeExplanation(prefix+".reach", "Reach", "melee reach + the Reach property's bonus", reach, terms...)
		}
		if distance := object(weapon["range"]); distance != nil {
			label := "Range"
			if truth(distance["thrown"]) {
				label = "Thrown range"
			}
			makeExplanation(prefix+".range", label, "normal range / long range from the weapon record",
				fmt.Sprintf("%d/%d", integer(distance["normal"], 0), integer(distance["long"], 0)),
				character.Term{Label: "Normal range", Value: distance["normal"], Source: record},
				character.Term{Label: "Long range", Value: distance["long"], Source: record})
		}
	}
	if worth := object(sheet["currencyWorth"]); worth != nil && profile.Constants.Currency != nil {
		values := profile.Constants.Currency.CoinValues
		unit := float64(values[text(worth["coin"])])
		coins := sortedKeys(input.Play.Currency)
		// Smallest coin first; coins the ruleset gives no value come last.
		sort.SliceStable(coins, func(left, right int) bool {
			leftValue, leftValued := values[coins[left]]
			rightValue, rightValued := values[coins[right]]
			if leftValued != rightValued {
				return leftValued
			}
			return leftValue < rightValue
		})
		terms := []character.Term{}
		for _, coin := range coins {
			amount := input.Play.Currency[coin]
			if value, valued := values[coin]; valued {
				terms = append(terms, character.Term{Label: coin, Value: Object{"coins": amount, "worth": amount * float64(value) / unit}})
			} else if amount != 0 {
				terms = append(terms, character.Term{Label: coin, Value: Object{"coins": amount}, Status: "inactive"})
			}
		}
		makeExplanation("currencyWorth.value", "Worth", "each coin count × its value, divided by the value of one "+text(worth["coin"]), worth["value"], terms...)
	}
	maximum := integer(derived["maxHp"], 0)
	zero := 0
	result["play.hp"] = character.Explanation{Label: "Current HP", Formula: "Recorded play value between zero and effective maximum HP.", Value: input.Play.HP, Minimum: &zero, Maximum: &maximum, Terms: []character.Term{{Label: "Effective maximum", Value: maximum}}, Sources: []character.Reference{}}
	result["play.temporaryHp"] = character.Explanation{Label: "Temporary HP", Formula: "Separate non-negative play pool; it does not increase maximum HP.", Value: input.Play.TemporaryHP, Minimum: &zero, Terms: []character.Term{}, Sources: []character.Reference{}}
	for path, explanation := range result {
		if path == "derived.speed" || strings.HasPrefix(path, "senses.") ||
			strings.HasPrefix(path, "weapons.") && (strings.HasSuffix(path, ".reach") || strings.HasSuffix(path, ".range")) {
			explanation.Unit = "ft"
			result[path] = explanation
		}
	}
	return result
}

func fmtIndex(path string, index int) string { return path + "." + jsonNumber(index) }

func bonusTerms(bonuses []Object) []character.Term {
	terms := make([]character.Term, 0, len(bonuses))
	for _, bonus := range bonuses {
		ref := grantSourceReference(object(bonus["source"]))
		value := bonus["value"]
		if bonus["requires"] != nil {
			value = Object{"bonus": bonus["value"], "requires": bonus["requires"]}
		}
		terms = append(terms, character.Term{Label: text(bonus["name"]), Value: value, Status: text(bonus["status"]), Source: &ref})
	}
	return terms
}
