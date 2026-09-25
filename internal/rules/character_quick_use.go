package rules

import (
	"fmt"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Pins only organize owned instances. They never grant equipment eligibility,
// create another quantity counter, or authorize undeclared item effects.
func validateCharacterQuickUse(input character.Inputs, result *character.Result) {
	owned := map[string]bool{}
	for _, item := range input.Play.Inventory {
		owned[item.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range input.Play.QuickUse {
		if id == "" || !owned[id] || seen[id] {
			addCharacterIssue(result, "quick-use:"+id, "inventory", "Quick-use pins must refer to distinct owned inventory instances.", "blocker", nil)
		}
		seen[id] = true
	}
	if len(input.Play.QuickUse) > 500 {
		addCharacterIssue(result, "quick-use-limit", "inventory", "Quick-use pins exceed the character input limit.", "blocker", nil)
	}
}

func characterItemUseReason(item character.Item) string {
	if item.Quantity < 1 {
		return "empty"
	}
	if item.Location != "carried" && item.Location != "equipped" {
		return "stored"
	}
	return ""
}

func characterQuickUse(input character.Inputs, result *character.Result) {
	items := map[string]character.Item{}
	options := Object{}
	for _, item := range input.Play.Inventory {
		items[item.ID] = item
		reason := characterItemUseReason(item)
		if reason == "" && !result.Ready {
			reason = "build"
		}
		options[item.ID] = Object{"canUse": reason == "", "reason": reason}
	}
	pins := []any{}
	for _, id := range input.Play.QuickUse {
		item, exists := items[id]
		row := Object{"itemId": id, "available": false, "reason": "missing"}
		if exists {
			row["name"], row["quantity"], row["location"] = item.Name, item.Quantity, item.Location
			row["available"], row["reason"] = characterItemUseReason(item) == "", characterItemUseReason(item)
			if item.Reference != nil {
				row["reference"] = *item.Reference
			}
		}
		pins = append(pins, row)
	}
	result.Sheet["quickUse"] = pins
	result.Guidance["quickUse"] = options
	result.Explanations["quickUse"] = character.Explanation{
		Label: "Quick use", Formula: "Pins reference owned inventory instances. Using one reduces that instance's quantity once; item effects are resolved separately. Stored or depleted items remain pinned. Rest and recalculation never replenish them.",
		Value: pins, Terms: []character.Term{}, Sources: []character.Reference{},
	}
}

func consumeCharacterItem(input *character.Inputs, change Object) error {
	id, valid := change["itemId"].(string)
	if !valid || id == "" || len(change) != 2 {
		return fmt.Errorf("Choose one owned inventory instance to use.")
	}
	for index := range input.Play.Inventory {
		item := &input.Play.Inventory[index]
		if item.ID != id {
			continue
		}
		if reason := characterItemUseReason(*item); reason != "" {
			return fmt.Errorf("The item is empty or stored. Carry an available item before using it.")
		}
		item.Quantity--
		if item.Quantity == 0 {
			item.Attuned = false
			if item.Location == "equipped" {
				item.Location = "carried"
			}
		}
		return nil
	}
	return fmt.Errorf("The selected inventory instance no longer exists.")
}
