package rules

import (
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"sort"
)

type characterEffectSource struct {
	ID, Name, GrantID string
	Reference         *character.Reference
	Effects           []character.Effect
	Active            bool
}

func characterEffectSources(input character.Inputs, records Records) []characterEffectSource {
	sources := []characterEffectSource{}
	for _, grant := range input.Grants {
		sources = append(sources, characterEffectSource{ID: grant.ID, Name: grant.Name, GrantID: grant.ID, Effects: grant.Effects, Active: characterGrantActive(grant, input)})
	}
	// Identical magic-item properties do not stack. Instances and suppressed
	// contributions remain visible; selection is stable rather than inventory order.
	items := append([]character.Item(nil), input.Play.Inventory...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	applied := map[string]bool{}
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
		key := item.Reference.Kind + ":" + item.Reference.ID
		active := item.Quantity > 0 && item.Location == "equipped" && (!truth(record["attunement"]) || item.Attuned) && !applied[key]
		if active {
			applied[key] = true
		}
		sources = append(sources, characterEffectSource{ID: item.ID, Name: item.Name, Reference: item.Reference, Effects: effects, Active: active})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	return sources
}

func validateCharacterItemMechanics(item character.Item, record Object, input character.Inputs, result *character.Result) {
	if item.Reference.Kind != "magic-item" || item.Quantity < 1 || item.Location != "equipped" && !item.Attuned {
		return
	}
	status := text(record["characterMechanics"])
	if status == "complete" || status == "narrative" {
		if status == "narrative" && len(values(record["characterEffects"])) > 0 {
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

func validCharacterEffect(effect character.Effect) bool {
	if !contains([]string{"set", "add", "minimum", "maximum"}, effect.Mode) || effect.Value < -100000 || effect.Value > 100000 {
		return false
	}
	switch effect.Target {
	case "abilityScore", "abilityCap":
		return contains(Abilities[:], effect.Key)
	case "savingThrow":
		return contains(Abilities[:], effect.Key)
	case "armorClass", "initiative", "speed", "maxHp", "attunementLimit":
		return effect.Key == ""
	case "sense", "resourceMax", "proficiency":
		return effect.Key != ""
	}
	return false
}
