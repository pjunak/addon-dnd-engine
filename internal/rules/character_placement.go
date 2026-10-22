package rules

import (
	"github.com/pjunak/addon-dnd-engine/character"
)

// These are display groups, not anatomy rules or mechanical occupancy limits.
var bodyPlacements = []string{"head", "body", "wrists", "legs", "feet", "face", "neck", "shoulders", "waist", "gloves", "other"}

// characterBodyPlacements lists the groups an instance's source declares. A
// magic weapon or armor without its own list uses its base record's.
func characterBodyPlacements(item character.Item, record Object, input character.Inputs, records Records) []string {
	if item.Reference == nil {
		if characterItemHasGrant(item, input) {
			return append([]string{}, bodyPlacements...)
		}
		return []string{}
	}
	if _, declares := record["bodyPlacements"]; !declares {
		if base := equipmentRecord(item, records); base != nil {
			record = base
		}
	}
	declared := stringsOf(record["bodyPlacements"])
	if len(declared) == 0 || len(declared) != len(values(record["bodyPlacements"])) {
		return []string{}
	}
	seen := map[string]bool{}
	for _, placement := range declared {
		if !contains(bodyPlacements, placement) || seen[placement] {
			return []string{}
		}
		seen[placement] = true
	}
	return append([]string{}, declared...)
}

func validateCharacterPlacement(input character.Inputs, records Records, result *character.Result) {
	for _, item := range input.Play.Inventory {
		if item.BodyPlacement == "" {
			continue
		}
		if item.Location != "equipped" || item.Quantity < 1 {
			addCharacterIssue(result, "placement-location:"+item.ID, "inventory", "Only equipped items with a positive quantity can have a body placement. Clear the placement when stowing the item.", "blocker", item.Reference)
		}
		var record Object
		if item.Reference != nil {
			record = recordByID(records, item.Reference.Kind, item.Reference.ID)
		}
		if !contains(characterBodyPlacements(item, record, input, records), item.BodyPlacement) {
			addCharacterIssue(result, "placement-source:"+item.ID, "inventory", "This body placement is not supported by the item's current source or active DM mechanics. Choose an available placement or clear it.", "blocker", item.Reference)
		}
	}
}

func characterPlacement(input character.Inputs, result *character.Result) {
	rows := []character.PlacementRow{}
	terms := []character.Term{}
	sources := []character.Reference{}
	for _, item := range input.Play.Inventory {
		if item.BodyPlacement == "" {
			continue
		}
		rows = append(rows, character.PlacementRow{ItemID: item.ID, Name: item.Name, Placement: item.BodyPlacement})
		terms = append(terms, character.Term{Label: item.Name, Value: item.BodyPlacement, Source: item.Reference, GrantID: item.GrantID})
		if item.Reference != nil {
			sources = append(sources, *item.Reference)
		}
	}
	result.Sheet.BodyPlacement = rows
	result.Explanations["bodyPlacement"] = character.Explanation{Label: "Body placement", Formula: "Authored display organization of equipped inventory instances. Placement grants no bonuses or extra equipment slots and does not limit the number of worn accessories. Armor, shield and attunement rules are evaluated separately.", Value: rows, Terms: terms, Sources: sources}
}
