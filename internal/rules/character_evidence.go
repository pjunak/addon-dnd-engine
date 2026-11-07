package rules

import (
	"bytes"
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
	factBody, _ := json.Marshal(facts)
	identity := SourceIdentity{}
	if source, ok := records.source.(interface {
		Provenance(string, string) SourceIdentity
	}); ok {
		identity = source.Provenance(kind, id)
	}
	records.selected[kind+":"+id] = character.Evidence{Reference: character.Reference{Kind: kind, ID: id}, Name: firstText(record["name"], id), Book: text(record["book"]), Hash: hex.EncodeToString(sum[:]), Summary: summary, Facts: character.RecordFacts(factBody), PackageID: identity.PackageID, PackageGeneration: identity.PackageGeneration, ContentRevision: identity.ContentRevision}
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

func captureCharacterSources(sheet *character.Calculation, records *characterRecords) {
	for _, feature := range sheet.Features {
		// A chosen subclass fills its class's slot row with kind "subclass".
		_, _ = records.Value(firstText(feature.Kind, "feature"), feature.ID)
	}
	for _, resource := range sheet.Resources {
		ref := grantSourceReference(resource.Source)
		_, _ = records.Value(ref.Kind, ref.ID)
	}
}

// factsOf decodes an evidence entry's record facts.
func factsOf(entry character.Evidence) Object {
	facts, _ := DecodeObject(json.RawMessage(entry.Facts))
	return facts
}

// jsonView is a value's JSON form, with whole numbers as int. Explanations
// are keyed by JSON paths, so their generic walk reads the sheet in this form.
func jsonView(value any) Object {
	body, _ := json.Marshal(value)
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var decoded any
	_ = decoder.Decode(&decoded)
	view, _ := jsonViewValue(decoded).(Object)
	return view
}

func jsonViewValue(value any) any {
	switch current := value.(type) {
	case json.Number:
		if whole, err := current.Int64(); err == nil {
			return int(whole)
		}
		number, _ := current.Float64()
		return number
	case map[string]any:
		result := Object{}
		for key, child := range current {
			result[key] = jsonViewValue(child)
		}
		return result
	case []any:
		result := make([]any, len(current))
		for index, child := range current {
			result[index] = jsonViewValue(child)
		}
		return result
	default:
		return current
	}
}

func characterExplanations(input character.Inputs, calculation *character.Calculation, evidence []character.Evidence, profile Ruleset) map[string]character.Explanation {
	sheet := jsonView(calculation)
	result := map[string]character.Explanation{}
	if size := object(sheet["derived"])["size"]; size != nil {
		source := character.Reference{Kind: "species", ID: input.Build.Species}
		speciesSize := term(engineText("Species size"), size)
		speciesSize.Source = &source
		result["derived.size"] = explanation(engineText("Size"), engineText("Size from the selected species and its explicit choice, when required."), size,
			[]character.Term{speciesSize}, []character.Reference{source})
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
			result[path] = explanation(plainText(path), engineText("Result of the selected rule and its applicable progression row."), intValue, terms, []character.Reference{})
		}
	}
	visit(sheet, "", nil)
	makeExplanation := func(path string, label, formula character.Text, value any, terms ...character.Term) {
		result[path] = explanation(label, formula, value, terms, sources)
	}
	keyed := func(label string, value any) character.Term { return term(engineText(label), value) }
	derived := object(sheet["derived"])
	pb := integer(derived["proficiencyBonus"], 0)
	makeExplanation("derived.proficiencyBonus", engineText("Proficiency bonus"), engineText("2 + floor((character level − 1) / 4)"), pb, keyed("Character level", len(input.Build.Levels)))
	for _, ability := range Abilities {
		value := object(object(sheet["abilities"])[ability])
		score := value["score"]
		makeExplanation("abilities."+ability+".score", engineText(ability+" score"), engineText("min(effective ability cap, base score + granted increases)"), score, keyed("Recorded base", input.Build.BaseScores[ability]), keyed("Granted increases", value["bonus"]), keyed("Effective cap", value["cap"]))
		makeExplanation("abilities."+ability+".mod", engineText(ability+" modifier"), engineText("floor((effective score − 10) / 2)"), value["mod"], keyed("Effective score", score))
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
			terms := []character.Term{keyed("Ability modifier", row["mod"]), keyed("Proficiency multiplier", multiplier), keyed("Proficiency bonus", pb)}
			terms = append(terms, bonusTerms(objects(row["bonuses"]))...)
			// Saves are keyed by ability and skills by a fixed name, so both labels are Engine wording.
			label := engineText(guidanceLabel(key))
			if group == "saves" {
				label = engineText(key + " saving throw")
			}
			makeExplanation(group+"."+key+".total", label, engineText("ability modifier + proficiency multiplier × proficiency bonus + feature bonuses"), row["total"], terms...)
		}
	}
	hp := object(sheet["hp"])
	hpTerms := []character.Term{keyed("Level gains", hp["levels"])}
	for _, bonus := range objects(hp["fixedBonuses"]) {
		ref := grantSourceReference(sourceFromObject(object(bonus["source"])))
		hpTerms = append(hpTerms, character.Term{Label: text(bonus["name"]), Value: bonus["value"], Source: &ref})
	}
	makeExplanation("derived.maxHp", engineText("Maximum HP"), engineText("sum of each level's max(minimum gain, hit die result + Constitution modifier) + per-level grants + fixed grants"), derived["maxHp"], hpTerms...)
	ac := object(sheet["ac"])
	terms := []character.Term{keyed("Available formulas (highest applies)", ac["candidates"]), keyed("Chosen formula", ac["base"]), keyed("Shield", ac["shield"])}
	for _, key := range []string{"speciesBonus", "activeBonus", "restrictions"} {
		if value, ok := ac[key]; ok {
			terms = append(terms, character.Term{Label: key, Value: value})
		}
	}
	for _, bonus := range objects(ac["bonuses"]) {
		ref := grantSourceReference(sourceFromObject(object(bonus["source"])))
		terms = append(terms, character.Term{Label: text(bonus["name"]), Value: Object{"bonus": bonus["value"], "requires": bonus["requires"]},
			Source: &ref, Status: text(bonus["status"])})
	}
	makeExplanation("derived.armorClass", engineText("Armor Class"), engineText("highest eligible AC formula + shield + applicable bonuses"), derived["armorClass"], terms...)
	dex := object(object(sheet["abilities"])["DEX"])["mod"]
	makeExplanation("derived.initiative", engineText("Initiative"), engineText("Dexterity modifier + applicable feature bonuses"), derived["initiative"], append([]character.Term{keyed("Dexterity modifier", dex), keyed("Feature bonuses", integer(derived["initiative"], 0)-integer(dex, 0))}, bonusTerms(objects(sheet["initiativeBonuses"]))...)...)
	speedTerms := []character.Term{}
	for _, entry := range evidence {
		for _, key := range []string{"speeds", "speed", "grants", "modifiers"} {
			facts := factsOf(entry)
			if fact, ok := facts[key]; ok && key != "grants" || key == "grants" && object(facts[key])["speedBonus"] != nil {
				ref := entry.Reference
				speedTerms = append(speedTerms, character.Term{Label: entry.Name + ": " + key, Value: fact, Source: &ref})
			}
		}
	}
	speedTerms = append(speedTerms, bonusTerms(objects(sheet["speedBonuses"]))...)
	makeExplanation("derived.speed", engineText("Speed"), engineText("species movement + applicable grants and active effects"), derived["speed"], speedTerms...)
	attunement := object(sheet["attunement"])
	makeExplanation("attunement.limit", engineText("Attunement capacity"), engineText("maximum of the ruleset limit and unlocked class capacities"), attunement["limit"], keyed("Class progression", sheet["classes"]))
	makeExplanation("attunement.count", engineText("Attuned items"), engineText("count of attuned inventory instances"), attunement["count"], keyed("Inventory", input.Play.Inventory))
	for key, value := range object(sheet["senses"]) {
		terms := []character.Term{}
		for _, entry := range evidence {
			ref := entry.Reference
			facts := factsOf(entry)
			grants := object(facts["grants"])
			if senses := object(facts["senses"]); senses[key] != nil {
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
				activations = append(objects(facts["activations"]), activations...)
			}
			for _, activation := range activations {
				for _, modifier := range objects(activation["modifiers"]) {
					target := text(modifier["target"])
					if target != "sense" && target != "senseBonus" || text(modifier["key"]) != key {
						continue
					}
					status := "inactive"
					for _, current := range calculation.Activations {
						if grantSourceReference(current.Source) == ref && current.ID == text(activation["id"]) && current.Active {
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
		makeExplanation("senses."+key, engineText(guidanceLabel(key)), engineText("greatest applicable granted range, followed by explicit DM adjustments"), value, terms...)
	}
	for _, resource := range objects(sheet["resources"]) {
		key := text(resource["key"])
		name := firstText(resource["name"], key)
		makeExplanation("resources."+key+".max", plainText(name), engineText("capacity from the granting rule's level/proficiency/ability progression"), resource["max"], keyed("Resource rule", resource))
		makeExplanation("resources."+key+".remaining", engineText("{0} remaining", name), engineText("effective capacity − recorded spent uses"), resource["remaining"], keyed("Capacity", resource["max"]), keyed("Spent", resource["spent"]))
	}
	for index, caster := range objects(object(sheet["spellcasting"])["perClass"]) {
		ability := text(caster["ability"])
		modifier := object(object(sheet["abilities"])[ability])["mod"]
		prefix := fmtIndex("spellcasting.perClass", index)
		makeExplanation(prefix+".saveDC", engineText("Spell save DC"), engineText("base + proficiency bonus + spellcasting ability modifier"), caster["saveDC"], keyed("Base", profile.Constants.SpellSaveDCBase), keyed("Proficiency bonus", pb), keyed(ability+" modifier", modifier))
		makeExplanation(prefix+".spellAttack", engineText("Spell attack"), engineText("proficiency bonus + spellcasting ability modifier"), caster["spellAttack"], keyed("Proficiency bonus", pb), keyed(ability+" modifier", modifier))
		makeExplanation(prefix+".preparedLimit", engineText("Prepared spells"), engineText("the class table's prepared spells at this class level"), caster["preparedLimit"], keyed("Class level", caster["level"]), keyed("Prepared", caster["preparedCount"]))
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
			ref := grantSourceReference(sourceFromObject(object(row["source"])))
			value := cloneObject(row)
			delete(value, "name")
			delete(value, "source")
			modifiers = append(modifiers, character.Term{Label: text(row["name"]), Value: value, Status: "applied", Source: &ref})
		}
		prefix := fmtIndex("weapons", index)
		abilityTerm := keyed(abilityName+" modifier", ability)
		makeExplanation(prefix+".attackBonus", engineText("Weapon attack"), engineText("attack ability modifier + proficiency bonus when proficient + attack bonuses"), weapon["attackBonus"], append([]character.Term{abilityTerm, keyed("Applicable proficiency", proficiency)}, modifiers...)...)
		makeExplanation(prefix+".damage", engineText("Weapon damage"), engineText("largest eligible damage die + attack ability modifier + damage bonuses"), weapon["damage"], append([]character.Term{abilityTerm, keyed("Damage type", weapon["damageType"])}, modifiers...)...)
		makeExplanation(prefix+".versatileDamage", engineText("Versatile damage"), engineText("two-handed weapon dice + attack ability modifier + damage bonuses"), weapon["versatileDamage"], append([]character.Term{abilityTerm}, modifiers...)...)
		var record *character.Reference
		if ref := text(weapon["ref"]); ref != "" {
			record = &character.Reference{Kind: "weapon", ID: ref}
		}
		if reach, ok := weapon["reach"]; ok && profile.Constants.MeleeReach != nil {
			terms := []character.Term{keyed("Melee reach", profile.Constants.MeleeReach.Base)}
			if contains(stringsOf(weapon["properties"]), "reach") {
				property := keyed("Reach property", profile.Constants.MeleeReach.ReachBonus)
				property.Source = record
				terms = append(terms, property)
			}
			makeExplanation(prefix+".reach", engineText("Reach"), engineText("melee reach + the Reach property's bonus"), reach, terms...)
		}
		if distance := object(weapon["range"]); distance != nil {
			label := "Range"
			if truth(distance["thrown"]) {
				label = "Thrown range"
			}
			normal, long := keyed("Normal range", distance["normal"]), keyed("Long range", distance["long"])
			normal.Source, long.Source = record, record
			makeExplanation(prefix+".range", engineText(label), engineText("normal range / long range from the weapon record"),
				fmt.Sprintf("%d/%d", integer(distance["normal"], 0), integer(distance["long"], 0)), normal, long)
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
		makeExplanation("currencyWorth.value", engineText("Worth"), engineText("each coin count × its value, divided by the value of one {0}", text(worth["coin"])), worth["value"], terms...)
	}
	maximum := integer(derived["maxHp"], 0)
	zero := 0
	current := explanation(engineText("Current HP"), engineText("Recorded play value between zero and effective maximum HP."), input.Play.HP, []character.Term{keyed("Effective maximum", maximum)}, []character.Reference{})
	current.Minimum, current.Maximum = &zero, &maximum
	result["play.hp"] = current
	temporary := explanation(engineText("Temporary HP"), engineText("Separate non-negative play pool; it does not increase maximum HP."), input.Play.TemporaryHP, []character.Term{}, []character.Reference{})
	temporary.Minimum = &zero
	result["play.temporaryHp"] = temporary
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
		ref := grantSourceReference(sourceFromObject(object(bonus["source"])))
		value := bonus["value"]
		if bonus["requires"] != nil {
			value = Object{"bonus": bonus["value"], "requires": bonus["requires"]}
		}
		terms = append(terms, character.Term{Label: text(bonus["name"]), Value: value, Status: text(bonus["status"]), Source: &ref})
	}
	return terms
}
