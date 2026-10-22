package rules

import (
	"fmt"
	"math"

	"github.com/pjunak/addon-dnd-engine/character"
)

const maximumItemWeight = 100000

// itemContainer is a record's declared container: capacity in pounds and
// whether its contents count toward the carrier.
func itemContainer(record Object) (capacity float64, weightless, declared bool) {
	container := object(record["container"])
	if container == nil {
		return 0, false, false
	}
	return number(container["capacityLb"], 0), truth(container["weightless"]), true
}

// itemWeight is one unit's weight: the authored weight, else the rules
// record's weightLb, else a magic weapon or armor's base weight. Unknown
// weights report false.
func itemWeight(item character.Item, record, base Object) (float64, bool) {
	if item.WeightLb != nil {
		return *item.WeightLb, true
	}
	if value, ok := record["weightLb"].(float64); ok {
		return value, true
	}
	value, ok := base["weightLb"].(float64)
	return value, ok
}

func pounds(value float64) float64 { return math.Round(value*100) / 100 }

// characterCarrying adds up carried weight against carrying capacity.
// Equipped and carried items count; stored items and the contents of a
// weightless container are reported but not counted. Coins count unless the
// character turns coin weight off. Nothing is enforced.
func characterCarrying(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	containerItems := []character.ContainerItem{}
	itemRecords := map[string]Object{}
	for _, item := range input.Play.Inventory {
		if item.Reference == nil {
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		itemRecords[item.ID] = record
		if capacity, weightless, ok := itemContainer(record); ok {
			containerItems = append(containerItems, character.ContainerItem{ItemID: item.ID, Weightless: weightless, CapacityLb: max(0, capacity)})
		}
	}
	result.Guidance.Storage.ContainerItems = containerItems
	policy := profile.Constants.Carrying
	if policy == nil {
		return
	}
	links := map[string]character.Container{}
	for _, container := range input.Play.Containers {
		links[container.ID] = container
	}
	containerFacts := func(id string) (capacity float64, weightless bool) {
		if link := links[id]; link.ItemID != "" {
			capacity, weightless, _ = itemContainer(itemRecords[link.ItemID])
		}
		return capacity, weightless
	}
	items, loads, members, storedMembers := map[string]character.CarriedItem{}, map[string]float64{}, map[string]int{}, map[string]int{}
	carried, storedLb, weightlessLb := 0.0, 0.0, 0.0
	unknown := []string{}
	for _, item := range input.Play.Inventory {
		each, known := itemWeight(item, itemRecords[item.ID], equipmentRecord(item, records))
		if !known {
			unknown = append(unknown, item.ID)
			continue
		}
		total := each * float64(max(0, item.Quantity))
		items[item.ID] = character.CarriedItem{EachLb: pounds(each), TotalLb: pounds(total), Custom: item.WeightLb != nil}
		if item.ContainerID != "" {
			loads[item.ContainerID] += total
			members[item.ContainerID]++
			if item.Location == "stored" {
				storedMembers[item.ContainerID]++
			}
		}
		_, weightless := containerFacts(item.ContainerID)
		switch {
		case item.Location == "stored":
			storedLb += total
		case weightless:
			weightlessLb += total
		default:
			carried += total
		}
	}
	coins, coinsLb := 0.0, 0.0
	for _, amount := range input.Play.Currency {
		coins += amount
	}
	countCoins := input.Play.CoinWeight == nil || *input.Play.CoinWeight
	if countCoins {
		coinsLb = coins / policy.CoinsPerLb
	}
	containers := map[string]character.CarriedContainer{}
	for _, container := range input.Play.Containers {
		capacity, weightless := containerFacts(container.ID)
		containers[container.ID] = character.CarriedContainer{LoadLb: pounds(loads[container.ID]), Weightless: weightless, Stored: members[container.ID] > 0 && storedMembers[container.ID] == members[container.ID], CapacityLb: max(0, capacity)}
	}
	strength := 0
	if sheet := result.Sheet.Calculation; sheet != nil {
		strength = sheet.Abilities["STR"].Score
	}
	capacity := pounds(float64(strength) * policy.CapacityPerStrength)
	total := pounds(carried + coinsLb)
	result.Sheet.Carrying = &character.Carrying{
		WeightLb: total, CapacityLb: capacity, ItemsLb: pounds(carried), CoinsLb: pounds(coinsLb), CoinWeight: countCoins,
		StoredLb: pounds(storedLb), WeightlessLb: pounds(weightlessLb), Unknown: unknown, Items: items, Containers: containers,
	}
	terms := []character.Term{{Label: "Equipped and carried items", Value: pounds(carried)}}
	if countCoins && coins > 0 {
		terms = append(terms, character.Term{Label: fmt.Sprintf("Coins (%g ÷ %g)", coins, policy.CoinsPerLb), Value: pounds(coinsLb)})
	}
	if storedLb > 0 {
		terms = append(terms, character.Term{Label: "Stored, not counted", Value: pounds(storedLb), Status: "excluded"})
	}
	if weightlessLb > 0 {
		terms = append(terms, character.Term{Label: "Inside weightless containers, not counted", Value: pounds(weightlessLb), Status: "excluded"})
	}
	terms = append(terms, character.Term{Label: fmt.Sprintf("Capacity: Strength %d × %g", strength, policy.CapacityPerStrength), Value: capacity})
	result.Explanations["carrying"] = character.Explanation{Label: "Carrying", Formula: "Weight × quantity of equipped and carried items, plus coins when counted, against the Strength score × the ruleset's capacity per point. Items without a known weight are left out.", Value: total, Terms: terms, Sources: []character.Reference{}}
}

func validateCharacterCarrying(input character.Inputs, result *character.Result) {
	items := map[string]character.Item{}
	for _, item := range input.Play.Inventory {
		items[item.ID] = item
		if weight := item.WeightLb; weight != nil && (*weight < 0 || *weight > maximumItemWeight || math.IsNaN(*weight)) {
			addCharacterIssue(result, "item-weight:"+item.ID, "inventory", "Give the item a weight of 0 or more pounds.", "blocker", nil)
		}
	}
	backed := map[string]bool{}
	for _, container := range input.Play.Containers {
		if container.ItemID == "" {
			continue
		}
		item, exists := items[container.ItemID]
		if !exists || backed[container.ItemID] {
			addCharacterIssue(result, "container-item:"+container.ID, "inventory", "Link the container to an inventory item that no other container uses, or clear the link.", "blocker", nil)
		} else if item.ContainerID == container.ID {
			addCharacterIssue(result, "container-item:"+container.ID, "inventory", "A container's own item cannot be stored inside it.", "blocker", nil)
		}
		backed[container.ItemID] = true
	}
}
