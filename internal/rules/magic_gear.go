package rules

import (
	"slices"
	"sort"

	"github.com/pjunak/addon-dnd-engine/character"
)

// A magic item's baseItem says it is a weapon or armor of a base type. Its
// inventory instance then counts as that base record for attacks, Armor Class,
// slots, hands, weight and placements, and a complete item adds its bonus.

// baseItemDecl is a record's baseItem: the kind of its base records, which
// records those are (fixed ids, or filters over that kind) and its bonuses.
type baseItemDecl struct {
	kind                                                    string
	ids, categories, ranges, properties, armorTypes, except []string
	bonuses                                                 []int
}

// baseItemLimit bounds the record IDs a baseItem lists.
const baseItemLimit = 50

var (
	weaponCategories = []string{"simple", "martial"}
	weaponRanges     = []string{"melee", "ranged"}
	wornArmorTypes   = []string{"light", "medium", "heavy", "shield"}
)

// magicBaseItem reads a record's baseItem all or nothing. A record without one
// returns (nil, true).
func magicBaseItem(record Object) (*baseItemDecl, bool) {
	raw, exists := record["baseItem"]
	if !exists {
		return nil, true
	}
	declared := object(raw)
	if declared == nil {
		return nil, false
	}
	decl := &baseItemDecl{kind: text(declared["kind"])}
	for key, value := range declared {
		valid := true
		switch key {
		case "kind":
			valid = decl.kind == "weapon" || decl.kind == "armor"
		case "ids":
			decl.ids, valid = baseItemIDs(value)
		case "except":
			decl.except, valid = baseItemIDs(value)
		case "categories":
			decl.categories, valid = stringsOf(value), decl.kind == "weapon" && itemOneOf(value, weaponCategories)
		case "ranges":
			decl.ranges, valid = stringsOf(value), decl.kind == "weapon" && itemOneOf(value, weaponRanges)
		case "properties":
			decl.properties, valid = stringsOf(value), decl.kind == "weapon" && itemTexts(value, 100)
		case "armorTypes":
			decl.armorTypes, valid = stringsOf(value), decl.kind == "armor" && itemOneOf(value, wornArmorTypes)
		case "bonus":
			decl.bonuses, valid = baseItemBonuses(value)
		default:
			valid = false
		}
		if !valid {
			return nil, false
		}
	}
	filtered := len(decl.categories)+len(decl.ranges)+len(decl.properties)+len(decl.armorTypes)+len(decl.except) > 0
	if _, hasKind := declared["kind"]; !hasKind || len(decl.ids) > 0 && filtered {
		return nil, false
	}
	return decl, true
}

// baseItemIDs is a bounded list of distinct record IDs.
func baseItemIDs(value any) ([]string, bool) {
	entries := values(value)
	if len(entries) == 0 || len(entries) > baseItemLimit {
		return nil, false
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		id, ok := entry.(string)
		if !ok || !itemText(id, 200) || contains(ids, id) {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

// baseItemBonuses reads one bonus from 1 to 10, or a list of distinct bonuses
// for the owner to pick from, in ascending order.
func baseItemBonuses(value any) ([]int, bool) {
	entries := values(value)
	if entries == nil {
		entries = []any{value}
	}
	if len(entries) == 0 || len(entries) > 10 {
		return nil, false
	}
	bonuses := make([]int, 0, len(entries))
	for _, entry := range entries {
		if !itemWhole(entry, 1, 10) || slices.Contains(bonuses, integer(entry, 0)) {
			return nil, false
		}
		bonuses = append(bonuses, integer(entry, 0))
	}
	slices.Sort(bonuses)
	return bonuses, true
}

// matches reports whether a record of the declared kind can be the base.
// Armor needs a worn armor type for its slot and Armor Class.
func (decl *baseItemDecl) matches(record Object) bool {
	id := text(record["id"])
	if decl.kind == "armor" && !contains(wornArmorTypes, text(record["armorType"])) {
		return false
	}
	if len(decl.ids) > 0 {
		return contains(decl.ids, id)
	}
	properties := stringsOf(record["properties"])
	for _, property := range decl.properties {
		if !contains(properties, property) {
			return false
		}
	}
	return !contains(decl.except, id) &&
		(len(decl.categories) == 0 || contains(decl.categories, text(record["category"]))) &&
		(len(decl.ranges) == 0 || contains(decl.ranges, text(record["range"]))) &&
		(len(decl.armorTypes) == 0 || contains(decl.armorTypes, text(record["armorType"])))
}

// options lists the records the item may be, by name. Catalog scans do not
// make the candidates evidence; only the base a calculation reads is.
func (decl *baseItemDecl) options(records Records) []Object {
	result := []Object{}
	for _, candidate := range recordCatalog(records, decl.kind) {
		if decl.matches(candidate) {
			result = append(result, candidate)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return text(result[i]["name"]) < text(result[j]["name"]) })
	return result
}

// formIssue is why an item's base or bonus is not known.
type formIssue string

const (
	formKnown   formIssue = ""
	formChoose  formIssue = "choose"  // several options and no pick
	formInvalid formIssue = "invalid" // a pick the record does not offer
	formMissing formIssue = "missing" // no rules record matches
)

// itemForm is what one inventory instance of a magic weapon or armor is: its
// record's baseItem resolved with the instance's picks. The records are shared
// and read-only.
type itemForm struct {
	record  Object
	decl    *baseItemDecl
	options []Object
	// base and bonus are set once each is fixed by the record or picked.
	base                  Object
	bonus                 int
	baseIssue, bonusIssue formIssue
}

// characterItemForm resolves an instance whose magic-item record declares a
// valid baseItem. Any other instance returns nil.
func characterItemForm(item character.Item, records Records) *itemForm {
	if item.Reference == nil || item.Reference.Kind != "magic-item" {
		return nil
	}
	record := recordByID(records, item.Reference.Kind, item.Reference.ID)
	decl, valid := magicBaseItem(record)
	if decl == nil || !valid {
		return nil
	}
	form := &itemForm{record: record, decl: decl, options: decl.options(records)}
	switch {
	case item.BaseItemID != "":
		form.baseIssue = formInvalid
		for _, option := range form.options {
			if text(option["id"]) == item.BaseItemID {
				form.base, form.baseIssue = option, formKnown
			}
		}
	case len(form.options) == 1:
		form.base = form.options[0]
	case len(form.options) == 0:
		form.baseIssue = formMissing
	default:
		form.baseIssue = formChoose
	}
	switch {
	case item.Bonus != 0:
		form.bonusIssue = formInvalid
		if slices.Contains(decl.bonuses, item.Bonus) {
			form.bonus, form.bonusIssue = item.Bonus, formKnown
		}
	case len(decl.bonuses) == 1:
		form.bonus = decl.bonuses[0]
	case len(decl.bonuses) > 1:
		form.bonusIssue = formChoose
	}
	return form
}

// appliedBonus is the bonus the instance adds: a complete item's known bonus,
// while it is attuned if its record requires attunement. Callers add it only
// where they use the base, so an unequipped or suspended item adds nothing.
// Copies do not suppress each other: each bonus belongs to its own weapon.
func (form *itemForm) appliedBonus(item character.Item) int {
	if form.base == nil || text(form.record["characterMechanics"]) != "complete" || truth(form.record["attunement"]) && !item.Attuned {
		return 0
	}
	return form.bonus
}

// equipmentRecord is the record whose equipment facts an instance has: the
// base of a magic weapon or armor once it is known, else its own record.
func equipmentRecord(item character.Item, records Records) Object {
	if item.Reference == nil {
		return nil
	}
	if form := characterItemForm(item, records); form != nil && form.base != nil {
		return form.base
	}
	return recordByID(records, item.Reference.Kind, item.Reference.ID)
}

// characterItemSlot is an instance's equipment slot. A magic armor whose base
// is still to be picked takes the slot all of its options share.
func characterItemSlot(item character.Item, records Records) string {
	if form := characterItemForm(item, records); form != nil && form.base == nil && len(form.options) > 0 {
		slot := characterEquipmentSlot(form.options[0])
		for _, option := range form.options[1:] {
			if characterEquipmentSlot(option) != slot {
				return "worn"
			}
		}
		return slot
	}
	return characterEquipmentSlot(equipmentRecord(item, records))
}

// baseItemGuidance lists what an instance may be for its equipment controls.
func baseItemGuidance(item character.Item, records Records, entry *character.EquipmentGuidance) {
	form := characterItemForm(item, records)
	if form == nil {
		return
	}
	for _, option := range form.options {
		entry.BaseItems = append(entry.BaseItems, character.Option{ID: text(option["id"]), Label: firstText(option["name"], option["id"])})
	}
	entry.Bonuses = append([]int(nil), form.decl.bonuses...)
}

// validateCharacterItemForm reports a magic weapon or armor's missing picks as
// warnings, so equipping it never stops play, and picks its record does not
// offer as blockers. Callers check only equipped or attuned items.
func validateCharacterItemForm(item character.Item, records Records, result *character.Result) {
	form := characterItemForm(item, records)
	if form == nil {
		if item.BaseItemID != "" || item.Bonus != 0 {
			addCharacterIssue(result, "item-base:"+item.ID, "inventory", item.Name+" has no base weapon or armor to choose; clear its base and bonus.", character.SeverityBlocker, item.Reference)
		}
		return
	}
	name, kind := characterItemName(item, form.record), form.decl.kind
	switch form.baseIssue {
	case formKnown:
	case formChoose:
		addCharacterIssue(result, "item-base:"+item.ID, "inventory", "Choose which "+kind+" "+name+" is.", character.SeverityWarning, item.Reference)
	case formMissing:
		addCharacterIssue(result, "item-base:"+item.ID, "inventory", "No "+kind+" in the rules data can be "+name+".", character.SeverityWarning, item.Reference)
	case formInvalid:
		addCharacterIssue(result, "item-base:"+item.ID, "inventory", name+" cannot be that "+kind+"; choose one of its options.", character.SeverityBlocker, item.Reference)
	}
	switch form.bonusIssue {
	case formKnown, formMissing:
	case formChoose:
		addCharacterIssue(result, "item-bonus:"+item.ID, "inventory", "Choose the bonus of "+name+".", character.SeverityWarning, item.Reference)
	case formInvalid:
		addCharacterIssue(result, "item-bonus:"+item.ID, "inventory", name+" does not offer that bonus; choose one of its options.", character.SeverityBlocker, item.Reference)
	}
}

// inventorySource names an inventory entry as the source of its own bonus.
func inventorySource(item Object) character.Source {
	return character.Source{Type: character.SourceItem, ID: text(item["id"]), Name: text(item["name"]),
		Ref: &character.Reference{Kind: text(item["kind"]), ID: text(item["ref"])}}
}
