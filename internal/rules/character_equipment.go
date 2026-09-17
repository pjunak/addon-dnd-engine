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

func characterEquipmentMechanics(item character.Item, record Object, proposed character.Inputs) bool {
	if item.Reference == nil {
		return characterItemHasGrant(item, proposed)
	}
	check := character.Result{Issues: []character.Issue{}}
	validateCharacterItemMechanics(item, record, proposed, &check)
	return !characterHasBlocker(check)
}

func characterHasBlocker(result character.Result) bool {
	for _, issue := range result.Issues {
		if issue.Severity == "blocker" {
			return true
		}
	}
	return false
}

// Recheck acquisition without the candidate's attunement. Its own effects and
// attunement-conditioned DM grants cannot supply the prerequisite or its waiver.
// This is a detached calculation, never an automatic withdrawal of saved items.
func validateCharacterAttunementPrerequisite(item character.Item, record Object, input character.Inputs, records Records, profile Ruleset, sheet Object, result *character.Result) {
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
		sheet = Hydrate(normalized, records, &profile).Sheet
		applyCharacterEffects(candidate, sheet, &check, records)
	}
	validatePrerequisite(prerequisite, "attunement:"+item.ID, "inventory", candidate, sheet, result, item.Reference)
}

// Equipment controls consume these facts instead of interpreting source rules.
// Reasons are stable codes for translated explanations; eligibility remains here.
func characterEquipmentOptions(input character.Inputs, records Records, profile Ruleset, result *character.Result) Object {
	options := Object{}
	attunement := object(result.Sheet["attunement"])
	for index, item := range input.Play.Inventory {
		var record Object
		if item.Reference != nil {
			record = recordByID(records, item.Reference.Kind, item.Reference.ID)
		}
		entry := Object{"canEquip": false, "canAttune": false, "slot": characterEquipmentSlot(record)}
		options[item.ID] = entry
		if item.Quantity < 1 {
			entry["equipReason"], entry["attuneReason"] = "empty", "empty"
			continue
		}
		if item.Reference != nil && record == nil {
			entry["equipReason"], entry["attuneReason"] = "source", "source"
			continue
		}
		proposed := input
		proposed.Play.Inventory = append([]character.Item(nil), input.Play.Inventory...)
		equipped := item
		equipped.Location = "equipped"
		proposed.Play.Inventory[index] = equipped
		entry["canEquip"] = characterEquipmentMechanics(equipped, record, proposed)
		if !truth(entry["canEquip"]) {
			entry["equipReason"] = "mechanics"
		}

		attuned := item
		attuned.Attuned = true
		proposed.Play.Inventory[index] = attuned
		if item.Reference != nil && !truth(record["attunement"]) {
			entry["attuneReason"] = "not-required"
			continue
		}
		if !characterEquipmentMechanics(attuned, record, proposed) {
			entry["attuneReason"] = "mechanics"
			continue
		}
		if attunement == nil {
			entry["attuneReason"] = "build"
			continue
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
			entry["attuneReason"] = "duplicate"
			continue
		}
		if !item.Attuned && integer(attunement["count"], 0) >= integer(attunement["limit"], 0) {
			entry["attuneReason"] = "capacity"
			continue
		}
		check := character.Result{Issues: []character.Issue{}}
		validateCharacterAttunementPrerequisite(item, record, input, records, profile, Object(result.Sheet), &check)
		if characterHasBlocker(check) {
			entry["attuneReason"] = "prerequisite"
			continue
		}
		entry["canAttune"] = true
	}
	return options
}
