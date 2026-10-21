package rules

import (
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
)

func attunementCapacityWarning(limit int) string {
	return fmt.Sprintf("Attuned to more than %d magic items (limit %d)", limit, limit)
}

// Slots are source-declared facts shared by validation, editor guidance and the
// saved projection. Catalog kind and record IDs are not equipment rules.
func characterEquipmentSlot(record Object) string {
	switch text(record["armorType"]) {
	case "":
		return "worn"
	case "shield":
		return "shield"
	default:
		return "armor"
	}
}

func characterItemHasGrant(item character.Item, input character.Inputs) bool {
	for _, grant := range activeCharacterGrants(input) {
		if grant.ID == item.GrantID && grant.ItemID == item.ID && len(grant.Effects) > 0 {
			return true
		}
	}
	return false
}

func characterEquipmentMechanics(item character.Item, record Object, proposed character.Inputs, records Records) bool {
	if item.Reference == nil {
		return characterItemHasGrant(item, proposed)
	}
	check := character.Result{Issues: []character.Issue{}}
	validateCharacterItemMechanics(item, record, proposed, records, &check)
	return !characterHasBlocker(check)
}

func characterHasBlocker(result character.Result) bool {
	for _, issue := range result.Issues {
		if issue.Severity == character.SeverityBlocker {
			return true
		}
	}
	return false
}

// Recheck acquisition without the candidate's attunement. Its own effects and
// attunement-conditioned DM grants cannot supply the prerequisite or its waiver.
// This is a detached calculation, never an automatic withdrawal of saved items.
func validateCharacterAttunementPrerequisite(item character.Item, record Object, input character.Inputs, records Records, profile Ruleset, sheet *character.Calculation, result *character.Result) {
	prerequisite := record["attunementPrerequisites"]
	if prerequisite == nil || object(prerequisite) != nil && len(object(prerequisite)) == 0 {
		return
	}
	candidate := input
	if item.Attuned {
		candidate = cloneCharacter(input)
		for index := range candidate.Play.Inventory {
			if candidate.Play.Inventory[index].ID == item.ID {
				candidate.Play.Inventory[index].Attuned = false
			}
		}
		check := character.Result{Issues: []character.Issue{}}
		normalized := NormalizeBuilderDecisions(characterDecisions(candidate, records, profile, &check), records, profile)
		applyCharacterAbilityEffects(candidate, normalized, &check, profile, records)
		sheet = Hydrate(normalized, records, profile).Sheet
		applyCharacterEffects(candidate, sheet, &check, records)
	}
	validatePrerequisite(prerequisite, "attunement:"+item.ID, "inventory", candidate, sheet, result, item.Reference)
}

// Equipment controls consume these facts instead of interpreting source rules.
// Reasons are stable codes for translated explanations; eligibility remains here.
func characterEquipmentOptions(input character.Inputs, records Records, profile Ruleset, result *character.Result) map[string]character.EquipmentGuidance {
	options := map[string]character.EquipmentGuidance{}
	sheet := result.Sheet.Calculation
	for index, item := range input.Play.Inventory {
		var record Object
		if item.Reference != nil {
			record = recordByID(records, item.Reference.Kind, item.Reference.ID)
		}
		options[item.ID] = equipmentOption(input, index, item, record, records, profile, sheet)
	}
	return options
}

func equipmentOption(input character.Inputs, index int, item character.Item, record Object, records Records, profile Ruleset, sheet *character.Calculation) character.EquipmentGuidance {
	entry := character.EquipmentGuidance{Slot: characterEquipmentSlot(record), BodyPlacements: characterBodyPlacements(item, record, input)}
	if item.Quantity < 1 {
		entry.EquipReason, entry.AttuneReason = "empty", "empty"
		return entry
	}
	if item.Reference != nil && record == nil {
		entry.EquipReason, entry.AttuneReason = "source", "source"
		return entry
	}
	proposed := input
	proposed.Play.Inventory = append([]character.Item(nil), input.Play.Inventory...)
	equipped := item
	equipped.Location = "equipped"
	proposed.Play.Inventory[index] = equipped
	entry.CanEquip = characterEquipmentMechanics(equipped, record, proposed, records)
	if !entry.CanEquip {
		entry.EquipReason = "mechanics"
	}

	attuned := item
	attuned.Attuned = true
	proposed.Play.Inventory[index] = attuned
	if item.Reference != nil && !truth(record["attunement"]) {
		entry.AttuneReason = "not-required"
		return entry
	}
	if !characterEquipmentMechanics(attuned, record, proposed, records) {
		entry.AttuneReason = "mechanics"
		return entry
	}
	if sheet == nil {
		entry.AttuneReason = "build"
		return entry
	}
	duplicate := false
	if profile.Constants.Character.UniqueAttunement && item.Reference != nil {
		for _, other := range input.Play.Inventory {
			if other.ID != item.ID && other.Quantity > 0 && other.Attuned && other.Reference != nil && *other.Reference == *item.Reference {
				duplicate = true
			}
		}
	}
	if duplicate {
		entry.AttuneReason = "duplicate"
		return entry
	}
	if !item.Attuned && sheet.Attunement.Count >= sheet.Attunement.Limit {
		entry.AttuneReason = "capacity"
		return entry
	}
	check := character.Result{Issues: []character.Issue{}}
	validateCharacterAttunementPrerequisite(item, record, input, records, profile, sheet, &check)
	if characterHasBlocker(check) {
		entry.AttuneReason = "prerequisite"
		return entry
	}
	entry.CanAttune = true
	return entry
}
