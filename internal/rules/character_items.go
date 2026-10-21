package rules

import (
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

type characterEffectSource struct {
	ID, Name, GrantID string
	Reference         *character.Reference
	Effects           []character.Effect
	Active            bool
}

// activeCharacterItems reports which owned item instances apply their record's
// mechanics: present, equipped, not suspended from a hand, attuned when the
// item requires it, and the first such copy of a record by instance ID, because
// identical magic-item properties do not stack. Selection is stable rather than
// inventory order.
func activeCharacterItems(input character.Inputs, records Records) map[string]bool {
	items := append([]character.Item(nil), input.Play.Inventory...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	active, applied := map[string]bool{}, map[character.Reference]bool{}
	for _, item := range items {
		if item.Reference == nil || item.Quantity < 1 || item.Location != "equipped" || characterItemSuspended(input, item.ID) || applied[*item.Reference] {
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		if record == nil || truth(record["attunement"]) && !item.Attuned {
			continue
		}
		applied[*item.Reference] = true
		active[item.ID] = true
	}
	return active
}

func characterEffectSources(input character.Inputs, records Records) []characterEffectSource {
	sources := []characterEffectSource{}
	for _, grant := range input.Grants {
		sources = append(sources, characterEffectSource{ID: grant.ID, Name: grant.Name, GrantID: grant.ID, Effects: grant.Effects, Active: characterGrantActive(grant, input)})
	}
	// Suppressed copies remain visible as inactive contributions.
	active := activeCharacterItems(input, records)
	items := append([]character.Item(nil), input.Play.Inventory...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for _, item := range items {
		if item.Reference == nil {
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		if text(record["characterMechanics"]) != "complete" {
			continue
		}
		var effects []character.Effect
		body, _ := json.Marshal(record["characterEffects"])
		if json.Unmarshal(body, &effects) != nil {
			continue
		}
		sources = append(sources, characterEffectSource{ID: item.ID, Name: item.Name, Reference: item.Reference, Effects: effects, Active: active[item.ID]})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	return sources
}

func characterItemName(item character.Item, record Object) string {
	return firstText(item.Name, record["name"], item.ID)
}

// characterItemGrantSources lists, in inventory order, each owned magic item
// whose complete record declares valid grants, as {source, grants, active}.
// The source is {type: "item", id: <instance>, name, ref}. An inactive item
// keeps only its resources, so its spent uses stay listed and recover on rests.
func characterItemGrantSources(input character.Inputs, records Records) []any {
	active := activeCharacterItems(input, records)
	result := []any{}
	for _, item := range input.Play.Inventory {
		if item.Reference == nil || item.Reference.Kind != "magic-item" {
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		if text(record["characterMechanics"]) != "complete" {
			continue
		}
		grants, valid := characterItemGrants(record, records)
		if !valid || len(grants) == 0 {
			continue
		}
		if !active[item.ID] {
			if grants["resources"] == nil {
				continue
			}
			grants = Object{"resources": grants["resources"]}
		}
		source := Object{"type": "item", "id": item.ID, "name": characterItemName(item, record), "ref": Object{"kind": item.Reference.Kind, "id": item.Reference.ID}}
		result = append(result, Object{"source": source, "grants": grants, "active": active[item.ID]})
	}
	return result
}

// characterActiveItems lists the magic items whose properties apply now, so a
// sheet can show their text without re-deriving the active rule.
func characterActiveItems(input character.Inputs, records Records) []character.ActiveItem {
	active := activeCharacterItems(input, records)
	rows := []character.ActiveItem{}
	for _, item := range input.Play.Inventory {
		if active[item.ID] && item.Reference.Kind == "magic-item" {
			record := recordByID(records, item.Reference.Kind, item.Reference.ID)
			rows = append(rows, character.ActiveItem{ItemID: item.ID, Name: characterItemName(item, record), Ref: *item.Reference})
		}
	}
	return rows
}

// clearCharacterItemState removes the uses, toggles, spell choices and casting
// abilities an item instance owns, once its last unit is gone.
func clearCharacterItemState(input *character.Inputs, id string) {
	owned := func(key string) bool { return itemGrantOwner(key, input.Play.Inventory) == id }
	for key := range input.Play.ResourceUses {
		if owned(key) {
			delete(input.Play.ResourceUses, key)
		}
	}
	for key := range input.Play.ActiveFeatures {
		if owned(key) {
			delete(input.Play.ActiveFeatures, key)
		}
	}
	for key := range input.Build.Spells.GrantChoices {
		if owned(key) {
			delete(input.Build.Spells.GrantChoices, key)
		}
	}
	for key := range input.Build.Spells.CastingAbilities {
		if owned(key) {
			delete(input.Build.Spells.CastingAbilities, key)
		}
	}
}

func validateCharacterItemMechanics(item character.Item, record Object, input character.Inputs, records Records, result *character.Result) {
	if item.Reference.Kind != "magic-item" || item.Quantity < 1 || item.Location != "equipped" && !item.Attuned {
		return
	}
	status := text(record["characterMechanics"])
	if status == "complete" || status == "narrative" {
		if status == "narrative" && (len(values(record["characterEffects"])) > 0 || record["grants"] != nil) {
			addCharacterIssue(result, "item-effects:"+item.ID, "inventory", "A narrative-only item cannot declare mechanical effects.", "blocker", item.Reference)
		}
		if status == "complete" {
			var effects []character.Effect
			body, _ := json.Marshal(record["characterEffects"])
			if json.Unmarshal(body, &effects) != nil {
				addCharacterIssue(result, "item-effects:"+item.ID, "inventory", "The item declares malformed character effects.", "blocker", item.Reference)
			}
			for _, effect := range effects {
				if !validCharacterEffect(effect) {
					addCharacterIssue(result, "item-effects:"+item.ID, "inventory", "This item declares an unsupported effect; its source needs correction.", "blocker", item.Reference)
				}
			}
			if _, valid := characterItemGrants(record, records); !valid {
				addCharacterIssue(result, "item-effects:"+item.ID, "inventory", "This item declares an unsupported effect; its source needs correction.", "blocker", item.Reference)
			}
		}
		return
	}
	for _, grant := range activeCharacterGrants(input) {
		if grant.ItemID == item.ID && len(grant.Effects) > 0 {
			return
		}
	}
	addCharacterIssue(result, "item-mechanics:"+item.ID, "inventory", "This item's character mechanics are not structured. A versioned source definition or explicit typed DM adjustment is required before use.", "blocker", item.Reference)
}

// itemGrantLimit bounds every list an item's grants declare.
const itemGrantLimit = 20

// characterItemGrants reads a magic item's grants through a closed allowlist of
// the feature grant vocabulary, all or nothing. Items become grant sources
// beside features, so anything that matches across sources (extends, while,
// exclusive groups) or spends spell slots is refused, and local keys are
// kebab-case so item-owned keys stay unambiguous. A record without grants
// returns (nil, true).
func characterItemGrants(record Object, records Records) (Object, bool) {
	raw := record["grants"]
	if raw == nil {
		return nil, true
	}
	grants := object(raw)
	if grants == nil {
		return nil, false
	}
	keys := []string{}
	for _, resource := range objects(grants["resources"]) {
		keys = append(keys, text(resource["key"]))
	}
	for field, value := range grants {
		valid := false
		switch field {
		case "advantages":
			valid = itemGrantEntries(value, "", func(entry Object) bool {
				return onlyKeys(entry, "on", "mode", "abilities", "skills", "when") && validItemRoll(entry)
			})
		case "castingAbility":
			valid = validItemCastingAbility(object(value))
		case "resources":
			valid = itemGrantEntries(value, "key", validItemResource)
		case "spells":
			valid = itemGrantEntries(value, "id", func(spell Object) bool { return validItemSpell(spell, keys, records) })
		case "activations":
			valid = itemGrantEntries(value, "id", validItemActivation)
		case "resistances", "damageImmunities", "conditionImmunities":
			valid = itemTexts(value, 100)
		case "conditions":
			valid = itemGrantEntries(value, "id", func(condition Object) bool {
				return onlyKeys(condition, "id", "name", "summary") && itemLocalID(condition["id"]) && itemText(condition["name"], 120) && itemOptionalText(condition, "summary", 2000)
			})
		}
		if !valid {
			return nil, false
		}
	}
	return cloneObjectDeep(grants), true
}

// itemGrantEntries checks a bounded list of valid objects whose identity field,
// when named, is distinct.
func itemGrantEntries(value any, identity string, valid func(Object) bool) bool {
	entries := values(value)
	if entries == nil || len(entries) > itemGrantLimit {
		return false
	}
	seen := map[string]bool{}
	for _, raw := range entries {
		entry := object(raw)
		if entry == nil || !valid(entry) {
			return false
		}
		if identity != "" {
			if id := text(entry[identity]); id != "" {
				if seen[id] {
					return false
				}
				seen[id] = true
			}
		}
	}
	return true
}

func validItemRoll(entry Object) bool {
	if !character.RollKind(text(entry["on"])).Valid() {
		return false
	}
	if mode, exists := entry["mode"]; exists && mode != "advantage" && mode != "disadvantage" {
		return false
	}
	if abilities, exists := entry["abilities"]; exists && !itemOneOf(abilities, Abilities[:]) {
		return false
	}
	if skills, exists := entry["skills"]; exists {
		if !itemTexts(skills, 100) {
			return false
		}
		for _, skill := range stringsOf(skills) {
			if _, known := SkillAbility[canonicalSkill(skill)]; !known {
				return false
			}
		}
	}
	return itemOptionalText(entry, "when", 200)
}

func validItemCastingAbility(declaration Object) bool {
	if _, fixed := declaration["fixed"]; fixed {
		return len(declaration) == 1 && contains(Abilities[:], text(declaration["fixed"]))
	}
	return len(declaration) == 2 && itemLocalID(declaration["id"]) && itemOneOf(declaration["choose"], Abilities[:])
}

func validItemResource(resource Object) bool {
	if recharge, exists := resource["recharge"]; exists && recharge != "short" && recharge != "long" {
		return false
	}
	return onlyKeys(resource, "key", "name", "fixed", "recharge") && itemLocalID(resource["key"]) && itemWhole(resource["fixed"], 1, 100) && itemOptionalText(resource, "name", 120)
}

// validItemSpell accepts fixed spell IDs or one spell choice. A leveled spell
// is cast with one of the item's own resources or at will; spells the Engine
// can read that have neither must be cantrips.
func validItemSpell(spell Object, resources []string, records Records) bool {
	if resource, exists := spell["resource"]; exists && !validItemSpellResource(resource, resources) {
		return false
	}
	if atWill, exists := spell["atWill"]; exists {
		if _, ok := atWill.(bool); !ok {
			return false
		}
	}
	cast := spell["resource"] != nil || truth(spell["atWill"])
	if _, fixed := spell["ids"]; fixed {
		if !onlyKeys(spell, "ids", "resource", "atWill") || len(values(spell["ids"])) == 0 || !itemTexts(spell["ids"], 200) {
			return false
		}
		for _, id := range stringsOf(spell["ids"]) {
			if record := recordByID(records, "spell", id); record != nil && integer(record["level"], 0) > 0 && !cast {
				return false
			}
		}
		return true
	}
	if !onlyKeys(spell, "id", "choose", "spellLevel", "maxSpellLevel", "from", "resource", "atWill") || !itemLocalID(spell["id"]) || !itemWhole(spell["choose"], 1, 10) {
		return false
	}
	level, hasLevel := spell["spellLevel"]
	maximum, hasMaximum := spell["maxSpellLevel"]
	if !hasLevel && !hasMaximum || hasLevel && !itemWhole(level, 0, 9) || hasMaximum && !itemWhole(maximum, 0, 9) {
		return false
	}
	cantrip := hasLevel && integer(level, 0) == 0 || hasMaximum && integer(maximum, 0) == 0
	if !cantrip && !cast {
		return false
	}
	if from, exists := spell["from"]; exists && !validItemSpellFrom(object(from)) {
		return false
	}
	return true
}

// An item spell's resource is one of the item's own keys, or {key, cost}.
func validItemSpellResource(resource any, keys []string) bool {
	if key, ok := resource.(string); ok {
		return contains(keys, key)
	}
	entry := object(resource)
	if cost, exists := entry["cost"]; exists && !itemWhole(cost, 1, 100) {
		return false
	}
	return entry != nil && onlyKeys(entry, "key", "cost") && contains(keys, text(entry["key"]))
}

func validItemSpellFrom(from Object) bool {
	if from == nil {
		return false
	}
	for key, value := range from {
		switch key {
		case "class", "school", "ids":
			if !itemTexts(value, 200) {
				return false
			}
		case "castingTime":
			if !itemText(value, 100) && !itemTexts(value, 100) {
				return false
			}
		case "ritual":
			if _, ok := value.(bool); !ok {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validItemActivation(activation Object) bool {
	if !onlyKeys(activation, "id", "name", "condition", "modifiers") || !itemLocalID(activation["id"]) || !itemOptionalText(activation, "name", 120) || !itemOptionalText(activation, "condition", 200) {
		return false
	}
	if modifiers, exists := activation["modifiers"]; exists {
		return itemGrantEntries(modifiers, "", validItemModifier)
	}
	return true
}

// Item toggles change only rolls, senses, resistances and immunities.
func validItemModifier(modifier Object) bool {
	switch text(modifier["target"]) {
	case "advantage":
		return onlyKeys(modifier, "target", "on", "mode", "abilities", "skills", "when") && validItemRoll(modifier)
	case "sense":
		return onlyKeys(modifier, "target", "key", "value") && itemLocalID(modifier["key"]) && itemWhole(modifier["value"], 1, 1000)
	case "senseBonus":
		return onlyKeys(modifier, "target", "key", "add") && itemLocalID(modifier["key"]) && itemWhole(modifier["add"], 1, 1000)
	case "resistance", "conditionImmunity", "damageImmunity":
		return onlyKeys(modifier, "target", "value") && itemText(modifier["value"], 100)
	}
	return false
}

func onlyKeys(entry Object, allowed ...string) bool {
	for key := range entry {
		if !contains(allowed, key) {
			return false
		}
	}
	return true
}

func itemLocalID(value any) bool {
	id, ok := value.(string)
	return ok && len(id) <= 100 && conditionID.MatchString(id)
}

func itemText(value any, limit int) bool {
	current, ok := value.(string)
	return ok && strings.TrimSpace(current) != "" && utf8.RuneCountInString(current) <= limit
}

func itemOptionalText(entry Object, key string, limit int) bool {
	value, exists := entry[key]
	current, ok := value.(string)
	return !exists || ok && utf8.RuneCountInString(current) <= limit
}

// itemTexts is a bounded list of non-empty strings.
func itemTexts(value any, limit int) bool {
	entries := values(value)
	if entries == nil || len(entries) > itemGrantLimit {
		return false
	}
	for _, entry := range entries {
		if !itemText(entry, limit) {
			return false
		}
	}
	return true
}

func itemOneOf(value any, options []string) bool {
	entries := values(value)
	if len(entries) == 0 || len(entries) > len(options) {
		return false
	}
	for _, entry := range entries {
		if current, ok := entry.(string); !ok || !contains(options, current) {
			return false
		}
	}
	return true
}

func itemWhole(value any, minimum, maximum int) bool {
	var current float64
	switch typed := value.(type) {
	case float64:
		current = typed
	case int:
		current = float64(typed)
	default:
		return false
	}
	return current >= float64(minimum) && current <= float64(maximum) && current == math.Trunc(current)
}
