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
	carrying := object(result.Sheet["carrying"])
	want := Object{"weightLb": 17.0, "capacityLb": 225.0, "itemsLb": 15.0, "coinsLb": 2.0, "coinWeight": true, "storedLb": 50.0, "weightlessLb": 20.0}
	for key, value := range want {
		if carrying[key] != value {
			t.Errorf("%s = %v, want %v", key, carrying[key], value)
		}
	}
	if !reflect.DeepEqual(carrying["unknown"], []any{"mystery"}) {
		t.Error("unknown weights", carrying["unknown"])
	}
	containers := object(carrying["containers"])
	if !reflect.DeepEqual(containers["in-pack"], Object{"loadLb": 5.0, "capacityLb": 30.0, "weightless": false, "stored": false}) ||
		!reflect.DeepEqual(containers["in-bag"], Object{"loadLb": 20.0, "capacityLb": 500.0, "weightless": true, "stored": false}) ||
		!reflect.DeepEqual(containers["camp"], Object{"loadLb": 50.0, "weightless": false, "stored": true}) {
		t.Error("containers", containers)
	}
	if !reflect.DeepEqual(object(carrying["items"])["anvil"], Object{"eachLb": 50.0, "totalLb": 50.0, "custom": true}) {
		t.Error("custom item weight", object(carrying["items"])["anvil"])
	}
	if offered := object(result.Guidance["storage"])["containerItems"]; !reflect.DeepEqual(offered, []any{Object{"itemId": "pack", "weightless": false, "capacityLb": 30.0}, Object{"itemId": "bag", "weightless": true, "capacityLb": 500.0}}) {
		t.Error("container items", offered)
	}
	off := false
	input.Play.CoinWeight = &off
	if carrying := object(EvaluateCharacter(input, records, profile).Sheet["carrying"]); carrying["weightLb"] != 15.0 || carrying["coinWeight"] != false {
		t.Error("coin weight off", carrying)
	}
	profile.Constants.Carrying = nil
	if EvaluateCharacter(input, records, profile).Sheet["carrying"] != nil {
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
