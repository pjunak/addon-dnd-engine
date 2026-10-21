package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func carryingFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	profile.Constants.Carrying = &CarryingPolicy{CapacityPerStrength: 15, CoinsPerLb: 50}
	records.byKind["gear"] = map[string]json.RawMessage{
		"pack":    mustJSON(Object{"kind": "gear", "id": "pack", "name": "Pack", "weightLb": 5, "container": Object{"capacityLb": 30}}),
		"bag":     mustJSON(Object{"kind": "gear", "id": "bag", "name": "Bag", "weightLb": 5, "container": Object{"capacityLb": 500, "weightless": true}}),
		"rope":    mustJSON(Object{"kind": "gear", "id": "rope", "name": "Rope", "weightLb": 5}),
		"tent":    mustJSON(Object{"kind": "gear", "id": "tent", "name": "Tent", "weightLb": 20}),
		"mystery": mustJSON(Object{"kind": "gear", "id": "mystery", "name": "Mystery"}),
	}
	ref := func(id string) *character.Reference { return &character.Reference{Kind: "gear", ID: id} }
	anvil := 50.0
	input.Play.Inventory = []character.Item{
		{ID: "pack", Name: "Pack", Reference: ref("pack"), Quantity: 1, Location: "carried"},
		{ID: "bag", Name: "Bag", Reference: ref("bag"), Quantity: 1, Location: "carried"},
		{ID: "rope", Name: "Rope", Reference: ref("rope"), Quantity: 1, Location: "carried", ContainerID: "in-pack"},
		{ID: "tent", Name: "Tent", Reference: ref("tent"), Quantity: 1, Location: "carried", ContainerID: "in-bag"},
		{ID: "anvil", Name: "Anvil", Quantity: 1, Location: "stored", ContainerID: "camp", WeightLb: &anvil},
		{ID: "mystery", Name: "Mystery", Reference: ref("mystery"), Quantity: 2, Location: "carried"},
	}
	input.Play.Containers = []character.Container{{ID: "in-pack", Name: "Pack", ItemID: "pack"}, {ID: "in-bag", Name: "Bag", ItemID: "bag"}, {ID: "camp", Name: "Camp"}}
	input.Play.Currency = map[string]float64{"gp": 90, "sp": 10}
	return input, records, profile
}

func TestCarryingCountsCarriedWeightAgainstStrength(t *testing.T) {
	input, records, profile := carryingFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	carrying := result.Sheet.Carrying
	if carrying == nil || carrying.WeightLb != 17 || carrying.CapacityLb != 225 || carrying.ItemsLb != 15 || carrying.CoinsLb != 2 || !carrying.CoinWeight || carrying.StoredLb != 50 || carrying.WeightlessLb != 20 {
		t.Fatalf("carrying = %+v", carrying)
	}
	if !reflect.DeepEqual(carrying.Unknown, []string{"mystery"}) {
		t.Error("unknown weights", carrying.Unknown)
	}
	containers := carrying.Containers
	if containers["in-pack"] != (character.CarriedContainer{LoadLb: 5, CapacityLb: 30}) ||
		containers["in-bag"] != (character.CarriedContainer{LoadLb: 20, CapacityLb: 500, Weightless: true}) ||
		containers["camp"] != (character.CarriedContainer{LoadLb: 50, Stored: true}) {
		t.Error("containers", containers)
	}
	if carrying.Items["anvil"] != (character.CarriedItem{EachLb: 50, TotalLb: 50, Custom: true}) {
		t.Error("custom item weight", carrying.Items["anvil"])
	}
	if offered := result.Guidance.Storage.ContainerItems; !reflect.DeepEqual(offered, []character.ContainerItem{{ItemID: "pack", CapacityLb: 30}, {ItemID: "bag", Weightless: true, CapacityLb: 500}}) {
		t.Error("container items", offered)
	}
	off := false
	input.Play.CoinWeight = &off
	if carrying := EvaluateCharacter(input, records, profile).Sheet.Carrying; carrying.WeightLb != 15 || carrying.CoinWeight {
		t.Error("coin weight off", carrying)
	}
	profile.Constants.Carrying = nil
	if EvaluateCharacter(input, records, profile).Sheet.Carrying != nil {
		t.Error("carrying without a ruleset policy")
	}
}

func TestCarryingRejectsInvalidWeightsAndLinks(t *testing.T) {
	for name, change := range map[string]func(*character.Inputs){
		"negative weight": func(input *character.Inputs) { weight := -1.0; input.Play.Inventory[0].WeightLb = &weight },
		"missing item":    func(input *character.Inputs) { input.Play.Containers[0].ItemID = "gone" },
		"shared item":     func(input *character.Inputs) { input.Play.Containers[1].ItemID = "pack" },
		"inside itself":   func(input *character.Inputs) { input.Play.Inventory[0].ContainerID = "in-pack" },
	} {
		input, records, profile := carryingFixture(t)
		change(&input)
		if EvaluateCharacter(input, records, profile).Ready {
			t.Errorf("%s was accepted", name)
		}
	}
	ruleset := syntheticRuleset(t)
	ruleset.Constants.Carrying = &CarryingPolicy{CapacityPerStrength: 0, CoinsPerLb: 50}
	if ruleset.Validate() == nil {
		t.Error("zero capacity accepted")
	}
}
