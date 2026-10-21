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
			addCharacterIssue(result, "quick-use:"+id, "inventory", "Quick-use pins must refer to distinct owned inventory instances.", character.SeverityBlocker, nil)
		}
		seen[id] = true
	}
	if len(input.Play.QuickUse) > 500 {
		addCharacterIssue(result, "quick-use-limit", "inventory", "Quick-use pins exceed the character input limit.", character.SeverityBlocker, nil)
	}
}

func characterItemUseReason(item character.Item) character.ItemReason {
	if item.Quantity < 1 {
		return character.ItemEmpty
	}
	if item.Location != "carried" && item.Location != "equipped" {
		return character.ItemStored
	}
	return character.ItemUsable
}

func characterQuickUse(input character.Inputs, result *character.Result) {
	items := map[string]character.Item{}
	options := map[string]character.ItemUse{}
	for _, item := range input.Play.Inventory {
		items[item.ID] = item
		reason := characterItemUseReason(item)
		if reason == character.ItemUsable && !result.Ready {
			reason = character.ItemBuild
		}
		options[item.ID] = character.ItemUse{CanUse: reason == character.ItemUsable, Reason: reason}
	}
	pins := []character.QuickUsePin{}
	for _, id := range input.Play.QuickUse {
		item, exists := items[id]
		row := character.QuickUsePin{ItemID: id, Reason: character.ItemMissing}
		if exists {
			quantity := item.Quantity
			row.Name, row.Quantity, row.Location = item.Name, &quantity, item.Location
			row.Available, row.Reason = characterItemUseReason(item) == character.ItemUsable, characterItemUseReason(item)
			row.Reference = item.Reference
		}
		pins = append(pins, row)
	}
	result.Sheet.QuickUse = pins
	result.Guidance.QuickUse = options
	result.Explanations["quickUse"] = character.Explanation{
		Label: "Quick use", Formula: "Pins reference owned inventory instances. Using one reduces that instance's quantity once; item effects are resolved separately. Stored or depleted items remain pinned. Rest and recalculation never replenish them.",
		Value: pins, Terms: []character.Term{}, Sources: []character.Reference{},
	}
}

func consumeCharacterItem(input *character.Inputs, id string) error {
	if id == "" {
		return fmt.Errorf("Choose one owned inventory instance to use.")
	}
	for index := range input.Play.Inventory {
		item := &input.Play.Inventory[index]
		if item.ID != id {
			continue
		}
		if reason := characterItemUseReason(*item); reason != character.ItemUsable {
			return fmt.Errorf("The item is empty or stored. Carry an available item before using it.")
		}
		item.Quantity--
		if item.Quantity == 0 {
			clearCharacterItemState(input, id)
			item.BodyPlacement = ""
			item.Attuned = false
			if item.Location == "equipped" {
				item.Location = "carried"
			}
		}
		return nil
	}
	return fmt.Errorf("The selected inventory instance no longer exists.")
}
